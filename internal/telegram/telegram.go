package telegram

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"amazon-tracker/internal/config"
)

// Bot manages Telegram interactions with inline-keyboard-driven UX.
type Bot struct {
	api            *tgbotapi.BotAPI
	cfg            *config.Config
	store          *config.ItemStore
	lang           *Lang
	wakeChan       chan struct{}
	forceCheckChan chan int // item ID to check (0 = all)

	// Conversational state machine
	awaitingState string
	pendingItemID int // item being edited
}

// New creates a new Telegram Bot instance.
func New(cfg *config.Config, store *config.ItemStore, wakeChan chan struct{}, forceCheckChan chan int) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.TelegramToken())
	if err != nil {
		return nil, fmt.Errorf("init telegram bot: %w", err)
	}

	api.Debug = false

	b := &Bot{
		api:            api,
		cfg:            cfg,
		store:          store,
		lang:           GetLang(cfg.Language()),
		wakeChan:       wakeChan,
		forceCheckChan: forceCheckChan,
	}

	b.registerCommands()

	return b, nil
}

// ── Public API ──────────────────────────────────────────────────────────

// SendAlert sends a formatted price alert for a specific item.
func (b *Bot) SendAlert(item *config.Item, price float64) error {
	L := b.lang
	text := fmt.Sprintf(
		"%s\n\n"+
			"%s\n"+
			"%s\n"+
			"%s",
		L.AlertTitle,
		fmt.Sprintf(L.AlertItem, item.Label(), item.ID),
		fmt.Sprintf(L.AlertPrice, price),
		fmt.Sprintf(L.AlertBuy, item.URL),
	)

	msg := tgbotapi.NewMessage(b.cfg.ChatID(), text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = false
	_, err := b.api.Send(msg)
	return err
}

// Lang returns the active language string table.
func (b *Bot) Lang() *Lang { return b.lang }

// SendRaw sends a plain HTML message.
func (b *Bot) SendRaw(text string) error {
	msg := tgbotapi.NewMessage(b.cfg.ChatID(), text)
	msg.ParseMode = tgbotapi.ModeHTML
	_, err := b.api.Send(msg)
	if err != nil {
		log.Printf("[ERROR] Telegram send failed: %v | Text: %s", err, text)
	}
	return err
}

// SendItemCheckResult sends the result of a scrape for a specific item.
// It reads from the item's cached Last* fields, so it is always consistent
// with the "Last Check" button display.
func (b *Bot) SendItemCheckResult(item *config.Item, isManual bool) {
	L := b.lang
	header := L.CheckResult
	if !isManual {
		header = L.AutoCheckResult
	}

	var body string
	if item.LastAvailable && item.LastPrice > 0 {
		emoji := "✅"
		if item.LastPrice > item.TargetPrice {
			emoji = "⚠️"
		}
		body = fmt.Sprintf(L.AvailableNew, emoji, item.Label(), item.LastPrice, item.TargetPrice)
	} else if item.LastUsedPrice > 0 {
		body = fmt.Sprintf(L.OutOfStockUsed, item.Label(), item.LastUsedPrice)
	} else {
		body = fmt.Sprintf(L.OutOfStock, item.Label())
	}

	text := fmt.Sprintf("%s — #%d\n\n%s\n🔗 <a href=\"%s\">Link</a>", header, item.ID, body, item.URL)
	_ = b.SendRaw(text)
}

// ── Update Receiver ─────────────────────────────────────────────────────

// StartReceiver begins listening to Telegram updates (commands + callbacks).
func (b *Bot) StartReceiver() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	go func() {
		log.Println("[INFO] Telegram command receiver started.")
		for update := range updates {
			if update.CallbackQuery != nil {
				if update.CallbackQuery.Message != nil && update.CallbackQuery.Message.Chat.ID != b.cfg.ChatID() {
					continue
				}
				b.sendTyping()
				b.handleCallback(update.CallbackQuery)
				continue
			}

			if update.Message == nil {
				continue
			}

			if update.Message.Chat.ID != b.cfg.ChatID() {
				log.Printf("[WARN] Unauthorized message from chat %d", update.Message.Chat.ID)
				continue
			}

			b.sendTyping()
			if update.Message.IsCommand() {
				b.handleCommand(update.Message.Command(), update.Message.CommandArguments())
			} else if b.awaitingState != "" {
				text := strings.TrimSpace(update.Message.Text)
				b.handleConversation(text)
			}
		}
	}()
}

// ── Command Handlers ────────────────────────────────────────────────────

func (b *Bot) handleCommand(cmd string, args string) {
	log.Printf("[INFO] Telegram command: /%s %s", cmd, args)
	L := b.lang

	switch cmd {
	case "start", "help":
		b.resetState()
		_ = b.SendRaw(b.helpText())

	case "list":
		b.resetState()
		b.sendItemList()

	case "add":
		b.resetState()
		b.awaitingState = "add_url"
		_ = b.SendRaw(L.AddPromptURL + "\n" + L.OrCancel)

	case "check":
		b.resetState()
		_ = b.SendRaw(L.CheckingAll)
		select {
		case b.forceCheckChan <- 0:
		default:
		}

	case "status":
		b.resetState()
		_ = b.SendRaw(b.statusText())

	case "setinterval":
		b.resetState()
		if args == "" {
			b.awaitingState = "setinterval"
			_ = b.SendRaw(fmt.Sprintf(L.IntervalPrompt, b.cfg.CheckInterval().Seconds()) + "\n" + L.OrCancel)
		} else {
			b.applySetInterval(args)
		}

	case "smart":
		b.resetState()
		if args == "" {
			b.awaitingState = "smart"
			_ = b.SendRaw(L.SmartPrompt + "\n" + L.OrCancel)
		} else {
			b.applySmartToggle(args)
		}

	case "lang":
		b.resetState()
		b.sendLangPicker()

	case "cancel":
		b.resetState()
		_ = b.SendRaw(L.PromptCancelled)

	default:
		b.resetState()
		_ = b.SendRaw(L.UnknownCommand)
	}
}

// ── Conversational State Machine ────────────────────────────────────────

func (b *Bot) handleConversation(text string) {
	L := b.lang

	switch b.awaitingState {
	case "add_url":
		text = strings.TrimSpace(text)
		if !strings.Contains(text, "amazon") {
			_ = b.SendRaw(L.InvalidURL)
			return
		}
		b.pendingItemID = 0
		b.awaitingState = "add_price:" + text
		_ = b.SendRaw(L.AddPromptPrice + "\n" + L.OrCancel)

	case "setinterval":
		b.applySetInterval(text)

	case "smart":
		b.applySmartToggle(text)

	case "editurl":
		item := b.store.Get(b.pendingItemID)
		if item == nil {
			_ = b.SendRaw(L.ItemNotFound)
			b.resetState()
			return
		}
		text = strings.TrimSpace(text)
		if !strings.Contains(text, "amazon") {
			_ = b.SendRaw(L.InvalidURL)
			return
		}
		if err := b.store.Update(b.pendingItemID, text, 0); err != nil {
			_ = b.SendRaw(fmt.Sprintf(L.SaveFailed, err))
		} else {
			_ = b.SendRaw(L.URLUpdated)
			b.wake()
		}
		b.resetState()

	case "editprice":
		item := b.store.Get(b.pendingItemID)
		if item == nil {
			_ = b.SendRaw(L.ItemNotFound)
			b.resetState()
			return
		}
		p, err := strconv.ParseFloat(strings.ReplaceAll(text, ",", "."), 64)
		if err != nil || p <= 0 {
			_ = b.SendRaw(L.InvalidPrice)
			return
		}
		if err := b.store.Update(b.pendingItemID, "", p); err != nil {
			_ = b.SendRaw(fmt.Sprintf(L.SaveFailed, err))
		} else {
			_ = b.SendRaw(fmt.Sprintf(L.PriceUpdated, p))
			b.wake()
		}
		b.resetState()

	default:
		if strings.HasPrefix(b.awaitingState, "add_price:") {
			url := strings.TrimPrefix(b.awaitingState, "add_price:")
			p, err := strconv.ParseFloat(strings.ReplaceAll(text, ",", "."), 64)
			if err != nil || p <= 0 {
				_ = b.SendRaw(L.InvalidPrice)
				return
			}
			item, err := b.store.Add(url, p)
			if err != nil {
				_ = b.SendRaw(fmt.Sprintf(L.AddFailed, err))
			} else {
				_ = b.SendRaw(fmt.Sprintf(L.AddSuccess, item.ID, item.Label(), item.TargetPrice))
				b.wake()
			}
			b.resetState()
			return
		}
		b.resetState()
	}
}

// ── Callback Query Handlers ─────────────────────────────────────────────

func (b *Bot) handleCallback(cq *tgbotapi.CallbackQuery) {
	data := cq.Data
	log.Printf("[INFO] Telegram callback: %s", data)

	callback := tgbotapi.NewCallback(cq.ID, "")
	b.api.Request(callback)

	msgID := cq.Message.MessageID
	chatID := cq.Message.Chat.ID
	L := b.lang

	switch {
	case data == "back":
		b.resetState()
		b.editMessageToItemList(chatID, msgID)

	case strings.HasPrefix(data, "item:"):
		b.resetState()
		id := b.parseID(data, "item:")
		b.editMessageToItemDetail(chatID, msgID, id)

	case strings.HasPrefix(data, "check:"):
		b.resetState()
		id := b.parseID(data, "check:")
		item := b.store.Get(id)
		if item == nil {
			return
		}
		_ = b.SendRaw(fmt.Sprintf(L.CheckingItem, item.Label()))
		select {
		case b.forceCheckChan <- id:
		default:
		}

	case strings.HasPrefix(data, "pause:"):
		b.resetState()
		id := b.parseID(data, "pause:")
		if err := b.store.SetEnabled(id, false); err == nil {
			b.editMessageToItemDetail(chatID, msgID, id)
			b.wake()
		}

	case strings.HasPrefix(data, "resume:"):
		b.resetState()
		id := b.parseID(data, "resume:")
		if err := b.store.SetEnabled(id, true); err == nil {
			b.editMessageToItemDetail(chatID, msgID, id)
			b.wake()
		}

	case strings.HasPrefix(data, "edit:"):
		b.resetState()
		id := b.parseID(data, "edit:")
		b.editMessageToEditMenu(chatID, msgID, id)

	case strings.HasPrefix(data, "editurl:"):
		id := b.parseID(data, "editurl:")
		item := b.store.Get(id)
		b.pendingItemID = id
		b.awaitingState = "editurl"
		if item != nil {
			_ = b.SendRaw(fmt.Sprintf(L.EditURLPrompt, item.URL) + "\n" + L.OrCancel)
		}

	case strings.HasPrefix(data, "editprice:"):
		id := b.parseID(data, "editprice:")
		item := b.store.Get(id)
		b.pendingItemID = id
		b.awaitingState = "editprice"
		if item != nil {
			_ = b.SendRaw(fmt.Sprintf(L.EditPricePrompt, item.TargetPrice) + "\n" + L.OrCancel)
		}

	case strings.HasPrefix(data, "remove:"):
		id := b.parseID(data, "remove:")
		b.editMessageToConfirmRemove(chatID, msgID, id)

	case strings.HasPrefix(data, "confirmremove:"):
		b.resetState()
		id := b.parseID(data, "confirmremove:")
		item := b.store.Get(id)
		label := "unknown"
		if item != nil {
			label = item.Label()
		}
		if err := b.store.Remove(id); err != nil {
			_ = b.SendRaw(fmt.Sprintf(L.RemoveFailed, err))
		} else {
			_ = b.SendRaw(fmt.Sprintf(L.RemoveSuccess, id, label))
			b.wake()
		}
		b.editMessageToItemList(chatID, msgID)

	case data == "cancelremove":
		b.resetState()
		b.editMessageToItemList(chatID, msgID)

	case strings.HasPrefix(data, "lastcheck:"):
		b.resetState()
		id := b.parseID(data, "lastcheck:")
		item := b.store.Get(id)
		if item == nil {
			return
		}

		var text string
		if item.LastCheckedTime.IsZero() {
			text = fmt.Sprintf(L.LastCheckTitle, item.ID, item.Label()) + "\n\n" + L.LastCheckNone
		} else {
			var body string
			if item.LastAvailable && item.LastPrice > 0 {
				emoji := "✅"
				if item.LastPrice > item.TargetPrice {
					emoji = "⚠️"
				}
				body = fmt.Sprintf(L.AvailableNew, emoji, item.Label(), item.LastPrice, item.TargetPrice)
			} else if item.LastUsedPrice > 0 {
				body = fmt.Sprintf(L.OutOfStockUsed, item.Label(), item.LastUsedPrice)
			} else {
				body = fmt.Sprintf(L.OutOfStock, item.Label())
			}

			text = fmt.Sprintf(L.LastCheckTitle, item.ID, item.Label()) + "\n\n" +
				fmt.Sprintf(L.LastCheckTime, item.LastCheckedTime.Format("15:04:05")) + "\n\n" +
				body + "\n" +
				fmt.Sprintf("🔗 <a href=\"%s\">Link</a>", item.URL)
		}

		keyboard := tgbotapi.NewInlineKeyboardMarkup(
			tgbotapi.NewInlineKeyboardRow(
				tgbotapi.NewInlineKeyboardButtonData(L.BtnBack, fmt.Sprintf("item:%d", id)),
			),
		)
		edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, msgID, text, keyboard)
		edit.ParseMode = tgbotapi.ModeHTML
		edit.DisableWebPagePreview = true
		b.api.Send(edit)

	case strings.HasPrefix(data, "lang:"):
		b.resetState()
		code := strings.TrimPrefix(data, "lang:")
		if _, ok := LangMap[code]; !ok {
			return
		}
		if err := b.cfg.SetLanguage(code); err != nil {
			log.Printf("[WARN] Failed to save language: %v", err)
		}
		b.lang = GetLang(code)
		b.registerCommands()
		name := "English"
		if code == "it" {
			name = "Italiano"
		}
		edit := tgbotapi.NewEditMessageText(chatID, msgID, fmt.Sprintf(b.lang.LangChanged, name))
		edit.ParseMode = tgbotapi.ModeHTML
		b.api.Send(edit)
	}
}

// ── Message Builders ────────────────────────────────────────────────────

func (b *Bot) sendItemList() {
	L := b.lang
	items := b.store.All()
	if len(items) == 0 {
		_ = b.SendRaw(L.NoItemsYet)
		return
	}

	text := L.TrackedItems + "\n\n" + L.TapToManage
	keyboard := b.buildItemListKeyboard(items)

	msg := tgbotapi.NewMessage(b.cfg.ChatID(), text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

func (b *Bot) editMessageToItemList(chatID int64, msgID int) {
	L := b.lang
	items := b.store.All()
	text := L.TrackedItems + "\n\n" + L.TapToManage
	if len(items) == 0 {
		text = L.NoItemsYet
	}

	keyboard := b.buildItemListKeyboard(items)
	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, msgID, text, keyboard)
	edit.ParseMode = tgbotapi.ModeHTML
	b.api.Send(edit)
}

func (b *Bot) buildItemListKeyboard(items []*config.Item) tgbotapi.InlineKeyboardMarkup {
	var rows [][]tgbotapi.InlineKeyboardButton
	for _, it := range items {
		status := "▶️"
		if !it.Enabled {
			status = "⏸"
		}
		label := fmt.Sprintf("%s %s — %.0f€", status, it.Label(), it.TargetPrice)
		row := tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, fmt.Sprintf("item:%d", it.ID)),
		)
		rows = append(rows, row)
	}
	return tgbotapi.NewInlineKeyboardMarkup(rows...)
}

func (b *Bot) editMessageToItemDetail(chatID int64, msgID int, id int) {
	L := b.lang
	item := b.store.Get(id)
	if item == nil {
		return
	}

	status := L.Active
	if !item.Enabled {
		status = L.Paused
	}

	lastChecked := L.Never
	if !item.LastCheckedTime.IsZero() {
		lastChecked = item.LastCheckedTime.Format("15:04:05")
	}

	text := fmt.Sprintf(
		"%s\n\n"+
			"🔗 <a href=\"%s\">%s</a>\n"+
			"💰 %s: <b>%.2f EUR</b>\n"+
			"⚙️ %s: <b>%s</b>\n"+
			"🕒 %s: <b>%s</b>",
		fmt.Sprintf(L.ItemDetail, item.ID),
		item.URL, item.Label(),
		L.Target, item.TargetPrice,
		L.Status, status,
		L.LastCheck, lastChecked,
	)

	var toggleBtn tgbotapi.InlineKeyboardButton
	if item.Enabled {
		toggleBtn = tgbotapi.NewInlineKeyboardButtonData(L.BtnPause, fmt.Sprintf("pause:%d", id))
	} else {
		toggleBtn = tgbotapi.NewInlineKeyboardButtonData(L.BtnResume, fmt.Sprintf("resume:%d", id))
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(L.BtnEdit, fmt.Sprintf("edit:%d", id)),
			tgbotapi.NewInlineKeyboardButtonData(L.BtnCheck, fmt.Sprintf("check:%d", id)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(L.BtnLastCheck, fmt.Sprintf("lastcheck:%d", id)),
		),
		tgbotapi.NewInlineKeyboardRow(
			toggleBtn,
			tgbotapi.NewInlineKeyboardButtonData(L.BtnRemove, fmt.Sprintf("remove:%d", id)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(L.BackToList, "back"),
		),
	)

	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, msgID, text, keyboard)
	edit.ParseMode = tgbotapi.ModeHTML
	edit.DisableWebPagePreview = true
	b.api.Send(edit)
}

func (b *Bot) editMessageToEditMenu(chatID int64, msgID int, id int) {
	L := b.lang
	item := b.store.Get(id)
	if item == nil {
		return
	}

	text := fmt.Sprintf(L.EditTitle, id, item.Label()) + "\n\n" + L.EditWhatChange
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(L.BtnURL, fmt.Sprintf("editurl:%d", id)),
			tgbotapi.NewInlineKeyboardButtonData(L.BtnPrice, fmt.Sprintf("editprice:%d", id)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(L.BtnBack, fmt.Sprintf("item:%d", id)),
		),
	)

	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, msgID, text, keyboard)
	edit.ParseMode = tgbotapi.ModeHTML
	b.api.Send(edit)
}

func (b *Bot) editMessageToConfirmRemove(chatID int64, msgID int, id int) {
	L := b.lang
	item := b.store.Get(id)
	if item == nil {
		return
	}

	text := fmt.Sprintf(L.RemoveConfirm, id) + "\n\n" +
		fmt.Sprintf("%s — %.2f EUR\n\n%s", item.Label(), item.TargetPrice, L.RemoveWarning)
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(L.RemoveYes, fmt.Sprintf("confirmremove:%d", id)),
			tgbotapi.NewInlineKeyboardButtonData(L.RemoveNo, "cancelremove"),
		),
	)

	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, msgID, text, keyboard)
	edit.ParseMode = tgbotapi.ModeHTML
	b.api.Send(edit)
}

// ── Helpers ─────────────────────────────────────────────────────────────

func (b *Bot) resetState() {
	b.awaitingState = ""
	b.pendingItemID = 0
}

func (b *Bot) wake() {
	select {
	case b.wakeChan <- struct{}{}:
	default:
	}
}

func (b *Bot) registerCommands() {
	L := b.lang
	commands := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "start", Description: L.CmdDescStart},
		tgbotapi.BotCommand{Command: "help", Description: L.CmdDescHelp},
		tgbotapi.BotCommand{Command: "list", Description: L.CmdDescList},
		tgbotapi.BotCommand{Command: "add", Description: L.CmdDescAdd},
		tgbotapi.BotCommand{Command: "check", Description: L.CmdDescCheck},
		tgbotapi.BotCommand{Command: "status", Description: L.CmdDescStatus},
		tgbotapi.BotCommand{Command: "setinterval", Description: L.CmdDescSetInterval},
		tgbotapi.BotCommand{Command: "smart", Description: L.CmdDescSmart},
		tgbotapi.BotCommand{Command: "lang", Description: L.CmdDescLang},
		tgbotapi.BotCommand{Command: "cancel", Description: L.CmdDescCancel},
	)
	if _, err := b.api.Request(commands); err != nil {
		log.Printf("[WARN] Failed to register telegram commands: %v", err)
	}
}

func (b *Bot) sendTyping() {
	action := tgbotapi.NewChatAction(b.cfg.ChatID(), tgbotapi.ChatTyping)
	b.api.Send(action)
}

func (b *Bot) parseID(data, prefix string) int {
	s := strings.TrimPrefix(data, prefix)
	id, _ := strconv.Atoi(s)
	return id
}

func (b *Bot) sendLangPicker() {
	L := b.lang
	current := b.cfg.Language()
	enLabel := "🇬🇧 English"
	itLabel := "🇮🇹 Italiano"
	if current == "en" {
		enLabel = "✅ " + enLabel
	} else {
		itLabel = "✅ " + itLabel
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(enLabel, "lang:en"),
			tgbotapi.NewInlineKeyboardButtonData(itLabel, "lang:it"),
		),
	)

	msg := tgbotapi.NewMessage(b.cfg.ChatID(), L.LangPrompt)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

func (b *Bot) helpText() string {
	L := b.lang
	return fmt.Sprintf(
		"%s\n\n%s\n%s\n%s\n\n%s\n%s\n%s\n\n%s\n%s\n%s\n%s\n\n%s\n%s\n%s",
		L.HelpTitle,
		L.HelpItemMgmt, L.HelpList, L.HelpAdd,
		L.HelpMonitoring, L.HelpCheck, L.HelpStatus,
		L.HelpSettings, L.HelpSetInterval, L.HelpSmart, L.HelpLang,
		L.HelpOther, L.HelpCancel, L.HelpHelp,
	)
}

func (b *Bot) statusText() string {
	L := b.lang
	smart := "OFF"
	if b.cfg.SmartNotifications() {
		smart = "ON"
	}

	items := b.store.All()
	enabled := 0
	for _, it := range items {
		if it.Enabled {
			enabled++
		}
	}

	return fmt.Sprintf(
		"%s\n\n%s\n%s\n%s\n%s",
		L.GlobalStatus,
		fmt.Sprintf(L.StatusItems, len(items), enabled),
		fmt.Sprintf(L.StatusInterval, b.cfg.CheckInterval().Seconds()),
		fmt.Sprintf(L.StatusSmart, smart),
		fmt.Sprintf(L.StatusLang, b.cfg.Language()),
	)
}

func (b *Bot) applySetInterval(text string) {
	L := b.lang
	sec, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || sec <= 0 {
		_ = b.SendRaw(L.InvalidInterval)
		return
	}
	if err := b.cfg.SetCheckInterval(time.Duration(sec) * time.Second); err != nil {
		_ = b.SendRaw(fmt.Sprintf(L.IntervalFailed, sec))
	} else {
		_ = b.SendRaw(fmt.Sprintf(L.IntervalUpdated, sec))
	}
	b.resetState()
	b.wake()
}

func (b *Bot) applySmartToggle(text string) {
	L := b.lang
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "on", "true":
		if err := b.cfg.SetSmartNotifications(true); err != nil {
			_ = b.SendRaw(L.SmartFailed)
		} else {
			_ = b.SendRaw(L.SmartEnabled)
		}
	case "off", "false":
		if err := b.cfg.SetSmartNotifications(false); err != nil {
			_ = b.SendRaw(L.SmartFailed)
		} else {
			_ = b.SendRaw(L.SmartDisabled)
		}
	default:
		_ = b.SendRaw(L.InvalidSmartVal)
		return
	}
	b.resetState()
}
