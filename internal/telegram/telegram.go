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
	wakeChan       chan struct{}
	forceCheckChan chan int // item ID to check (0 = all)

	// Conversational state machine
	awaitingState  string
	pendingItemID  int // item being edited
}

// New creates a new Telegram Bot instance.
func New(cfg *config.Config, store *config.ItemStore, wakeChan chan struct{}, forceCheckChan chan int) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.TelegramToken())
	if err != nil {
		return nil, fmt.Errorf("init telegram bot: %w", err)
	}

	api.Debug = false

	commands := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "start", Description: "Show welcome message"},
		tgbotapi.BotCommand{Command: "help", Description: "Show available commands"},
		tgbotapi.BotCommand{Command: "list", Description: "Show all tracked items"},
		tgbotapi.BotCommand{Command: "add", Description: "Add a new item to track"},
		tgbotapi.BotCommand{Command: "check", Description: "Force check all enabled items"},
		tgbotapi.BotCommand{Command: "status", Description: "Show global settings"},
		tgbotapi.BotCommand{Command: "setinterval", Description: "Set poll interval in seconds"},
		tgbotapi.BotCommand{Command: "smart", Description: "Toggle smart notifications"},
		tgbotapi.BotCommand{Command: "cancel", Description: "Cancel current prompt"},
	)
	if _, err := api.Request(commands); err != nil {
		log.Printf("[WARN] Failed to register telegram commands: %v", err)
	}

	return &Bot{
		api:            api,
		cfg:            cfg,
		store:          store,
		wakeChan:       wakeChan,
		forceCheckChan: forceCheckChan,
	}, nil
}

// ── Public API ──────────────────────────────────────────────────────────

// SendAlert sends a formatted price alert for a specific item.
func (b *Bot) SendAlert(item *config.Item, price float64) error {
	text := fmt.Sprintf(
		"🚨 <b>AMAZON SNIPER ALERT!</b>\n\n"+
			"📦 Item: <b>%s</b> (#%d)\n"+
			"💰 Prezzo: <b>%.2f EUR</b>\n"+
			"🔗 <a href=\"%s\">Acquista su Amazon</a>",
		item.Label(), item.ID, price, item.URL,
	)

	msg := tgbotapi.NewMessage(b.cfg.ChatID(), text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = false
	_, err := b.api.Send(msg)
	return err
}

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
func (b *Bot) SendItemCheckResult(item *config.Item, available bool, price, usedPrice float64, condition string, isManual bool) {
	header := "🔍 <b>Check Result</b>"
	if !isManual {
		header = "🔄 <b>Auto-check Result</b>"
	}

	var body string
	if available && price > 0 {
		emoji := "✅"
		if price > item.TargetPrice {
			emoji = "⚠️"
		}
		body = fmt.Sprintf(
			"%s %s available (NEW) at <b>%.2f EUR</b>\n🎯 Target: &lt; %.2f EUR",
			emoji, item.Label(), price, item.TargetPrice,
		)
	} else if usedPrice > 0 {
		body = fmt.Sprintf(
			"❌ %s out of stock (new)\n📦 Used offer: <b>%.2f EUR</b>",
			item.Label(), usedPrice,
		)
	} else {
		body = fmt.Sprintf("❌ %s out of stock. No offers found.", item.Label())
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
			// Handle callback queries (inline button taps)
			if update.CallbackQuery != nil {
				if update.CallbackQuery.Message != nil && update.CallbackQuery.Message.Chat.ID != b.cfg.ChatID() {
					continue
				}
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
		_ = b.SendRaw("🔗 <b>Send me the Amazon product URL:</b>\n<i>(Or type /cancel)</i>")

	case "check":
		b.resetState()
		_ = b.SendRaw("🔍 <b>Forcing check on all enabled items...</b>")
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
			_ = b.SendRaw("⏱ <b>Send me the new check interval in seconds (e.g. 60):</b>\n<i>(Or type /cancel)</i>")
		} else {
			b.applySetInterval(args)
		}

	case "smart":
		b.resetState()
		if args == "" {
			b.awaitingState = "smart"
			_ = b.SendRaw("🧠 <b>Enable Smart Notifications? (on/off):</b>\n<i>(Or type /cancel)</i>")
		} else {
			b.applySmartToggle(args)
		}

	case "cancel":
		b.resetState()
		_ = b.SendRaw("🚫 Prompt cancelled.")

	default:
		b.resetState()
		_ = b.SendRaw("❓ Unknown command. Type <code>/help</code>.")
	}
}

// ── Conversational State Machine ────────────────────────────────────────

func (b *Bot) handleConversation(text string) {
	switch b.awaitingState {
	case "add_url":
		text = strings.TrimSpace(text)
		if !strings.Contains(text, "amazon") {
			_ = b.SendRaw("❌ That doesn't look like an Amazon URL. Try again or /cancel")
			return
		}
		b.pendingItemID = 0 // reuse as temp storage flag
		b.awaitingState = "add_price"
		// Store URL temporarily in awaitingState metadata
		b.awaitingState = "add_price:" + text
		_ = b.SendRaw("💰 <b>Now send me the target price (e.g. 900.00):</b>\n<i>(Or type /cancel)</i>")

	case "setinterval":
		b.applySetInterval(text)

	case "smart":
		b.applySmartToggle(text)

	case "editurl":
		item := b.store.Get(b.pendingItemID)
		if item == nil {
			_ = b.SendRaw("❌ Item not found.")
			b.resetState()
			return
		}
		text = strings.TrimSpace(text)
		if !strings.Contains(text, "amazon") {
			_ = b.SendRaw("❌ That doesn't look like an Amazon URL. Try again or /cancel")
			return
		}
		if err := b.store.Update(b.pendingItemID, text, 0); err != nil {
			_ = b.SendRaw(fmt.Sprintf("⚠️ Failed to save: %v", err))
		} else {
			_ = b.SendRaw("✅ URL updated.")
			b.wake()
		}
		b.resetState()

	case "editprice":
		item := b.store.Get(b.pendingItemID)
		if item == nil {
			_ = b.SendRaw("❌ Item not found.")
			b.resetState()
			return
		}
		p, err := strconv.ParseFloat(strings.ReplaceAll(text, ",", "."), 64)
		if err != nil || p <= 0 {
			_ = b.SendRaw("❌ Invalid price. Try again (e.g. 850.00) or /cancel")
			return
		}
		if err := b.store.Update(b.pendingItemID, "", p); err != nil {
			_ = b.SendRaw(fmt.Sprintf("⚠️ Failed to save: %v", err))
		} else {
			_ = b.SendRaw(fmt.Sprintf("✅ Target price updated to %.2f EUR.", p))
			b.wake()
		}
		b.resetState()

	default:
		// Handle "add_price:<url>"
		if strings.HasPrefix(b.awaitingState, "add_price:") {
			url := strings.TrimPrefix(b.awaitingState, "add_price:")
			p, err := strconv.ParseFloat(strings.ReplaceAll(text, ",", "."), 64)
			if err != nil || p <= 0 {
				_ = b.SendRaw("❌ Invalid price. Try again (e.g. 900.00) or /cancel")
				return
			}
			item, err := b.store.Add(url, p)
			if err != nil {
				_ = b.SendRaw(fmt.Sprintf("⚠️ Failed to add item: %v", err))
			} else {
				_ = b.SendRaw(fmt.Sprintf("✅ Item #%d added!\n📦 %s\n💰 Target: %.2f EUR", item.ID, item.Label(), item.TargetPrice))
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

	// Always acknowledge the callback
	callback := tgbotapi.NewCallback(cq.ID, "")
	b.api.Request(callback)

	msgID := cq.Message.MessageID
	chatID := cq.Message.Chat.ID

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
		_ = b.SendRaw(fmt.Sprintf("🔍 <b>Checking %s...</b>", item.Label()))
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
		b.pendingItemID = id
		b.awaitingState = "editurl"
		_ = b.SendRaw("🔗 <b>Send me the new Amazon URL:</b>\n<i>(Or type /cancel)</i>")

	case strings.HasPrefix(data, "editprice:"):
		id := b.parseID(data, "editprice:")
		b.pendingItemID = id
		b.awaitingState = "editprice"
		_ = b.SendRaw("💰 <b>Send me the new target price (e.g. 850.00):</b>\n<i>(Or type /cancel)</i>")

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
			_ = b.SendRaw(fmt.Sprintf("⚠️ Failed to remove: %v", err))
		} else {
			_ = b.SendRaw(fmt.Sprintf("🗑 Item #%d (%s) removed.", id, label))
			b.wake()
		}
		// Update the original message to show the list
		b.editMessageToItemList(chatID, msgID)

	case data == "cancelremove":
		b.resetState()
		// The callback came from a confirm-remove message on a specific item.
		// We don't know which item, so just go back to list.
		b.editMessageToItemList(chatID, msgID)
	}
}

// ── Message Builders ────────────────────────────────────────────────────

func (b *Bot) sendItemList() {
	items := b.store.All()
	if len(items) == 0 {
		_ = b.SendRaw("📭 No items tracked yet.\nUse /add to add one!")
		return
	}

	text := "📋 <b>Tracked Items</b>\n\nTap an item to manage it:"
	keyboard := b.buildItemListKeyboard(items)

	msg := tgbotapi.NewMessage(b.cfg.ChatID(), text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.ReplyMarkup = keyboard
	b.api.Send(msg)
}

func (b *Bot) editMessageToItemList(chatID int64, msgID int) {
	items := b.store.All()
	text := "📋 <b>Tracked Items</b>\n\nTap an item to manage it:"
	if len(items) == 0 {
		text = "📭 No items tracked yet.\nUse /add to add one!"
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
	item := b.store.Get(id)
	if item == nil {
		return
	}

	status := "▶️ Active"
	if !item.Enabled {
		status = "⏸ Paused"
	}

	lastChecked := "Never"
	if !item.LastCheckedTime.IsZero() {
		lastChecked = item.LastCheckedTime.Format("15:04:05")
	}

	text := fmt.Sprintf(
		"📦 <b>Item #%d</b>\n\n"+
			"🔗 <a href=\"%s\">%s</a>\n"+
			"💰 Target: <b>%.2f EUR</b>\n"+
			"⚙️ Status: <b>%s</b>\n"+
			"🕒 Last check: <b>%s</b>",
		item.ID, item.URL, item.Label(), item.TargetPrice, status, lastChecked,
	)

	// Build action buttons
	var toggleBtn tgbotapi.InlineKeyboardButton
	if item.Enabled {
		toggleBtn = tgbotapi.NewInlineKeyboardButtonData("⏸ Pause", fmt.Sprintf("pause:%d", id))
	} else {
		toggleBtn = tgbotapi.NewInlineKeyboardButtonData("▶️ Resume", fmt.Sprintf("resume:%d", id))
	}

	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✏️ Edit", fmt.Sprintf("edit:%d", id)),
			tgbotapi.NewInlineKeyboardButtonData("🔍 Check", fmt.Sprintf("check:%d", id)),
		),
		tgbotapi.NewInlineKeyboardRow(
			toggleBtn,
			tgbotapi.NewInlineKeyboardButtonData("🗑 Remove", fmt.Sprintf("remove:%d", id)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔙 Back to list", "back"),
		),
	)

	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, msgID, text, keyboard)
	edit.ParseMode = tgbotapi.ModeHTML
	edit.DisableWebPagePreview = true
	b.api.Send(edit)
}

func (b *Bot) editMessageToEditMenu(chatID int64, msgID int, id int) {
	item := b.store.Get(id)
	if item == nil {
		return
	}

	text := fmt.Sprintf("✏️ <b>Edit Item #%d</b> (%s)\n\nWhat do you want to change?", id, item.Label())
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔗 URL", fmt.Sprintf("editurl:%d", id)),
			tgbotapi.NewInlineKeyboardButtonData("💰 Price", fmt.Sprintf("editprice:%d", id)),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("🔙 Back", fmt.Sprintf("item:%d", id)),
		),
	)

	edit := tgbotapi.NewEditMessageTextAndMarkup(chatID, msgID, text, keyboard)
	edit.ParseMode = tgbotapi.ModeHTML
	b.api.Send(edit)
}

func (b *Bot) editMessageToConfirmRemove(chatID int64, msgID int, id int) {
	item := b.store.Get(id)
	if item == nil {
		return
	}

	text := fmt.Sprintf("⚠️ <b>Remove Item #%d?</b>\n\n%s — %.2f EUR\n\nThis cannot be undone.", id, item.Label(), item.TargetPrice)
	keyboard := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ Yes, remove", fmt.Sprintf("confirmremove:%d", id)),
			tgbotapi.NewInlineKeyboardButtonData("❌ Cancel", "cancelremove"),
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

func (b *Bot) parseID(data, prefix string) int {
	s := strings.TrimPrefix(data, prefix)
	id, _ := strconv.Atoi(s)
	return id
}

func (b *Bot) helpText() string {
	return `🤖 <b>Amazon Sniper Bot</b>

<b>Item Management</b>
/list - Show all tracked items (tap to manage)
/add - Add a new item to track

<b>Monitoring</b>
/check - Force check all enabled items
/status - Show global settings

<b>Settings</b>
/setinterval - Set poll interval (seconds)
/smart - Toggle smart notifications (on/off)

<b>Other</b>
/cancel - Cancel current prompt
/help - Show this message`
}

func (b *Bot) statusText() string {
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
		"📊 <b>GLOBAL STATUS</b>\n\n"+
			"📦 Items: <b>%d total</b> (%d active)\n"+
			"⏱ Interval: <b>%.0fs</b>\n"+
			"🧠 Smart Notify: <b>%s</b>",
		len(items), enabled,
		b.cfg.CheckInterval().Seconds(),
		smart,
	)
}

func (b *Bot) applySetInterval(text string) {
	sec, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil || sec <= 0 {
		_ = b.SendRaw("❌ Invalid interval. Try again (e.g. 60) or /cancel")
		return
	}
	if err := b.cfg.SetCheckInterval(time.Duration(sec) * time.Second); err != nil {
		_ = b.SendRaw(fmt.Sprintf("⚠️ Interval updated to %ds, but failed to save.", sec))
	} else {
		_ = b.SendRaw(fmt.Sprintf("✅ Check interval updated to %d seconds.", sec))
	}
	b.resetState()
	b.wake()
}

func (b *Bot) applySmartToggle(text string) {
	switch strings.ToLower(strings.TrimSpace(text)) {
	case "on", "true":
		if err := b.cfg.SetSmartNotifications(true); err != nil {
			_ = b.SendRaw("⚠️ Smart Notifications enabled (not saved).")
		} else {
			_ = b.SendRaw("✅ Smart Notifications enabled.")
		}
	case "off", "false":
		if err := b.cfg.SetSmartNotifications(false); err != nil {
			_ = b.SendRaw("⚠️ Smart Notifications disabled (not saved).")
		} else {
			_ = b.SendRaw("✅ Smart Notifications disabled.")
		}
	default:
		_ = b.SendRaw("❌ Please reply with <code>on</code> or <code>off</code>, or /cancel")
		return
	}
	b.resetState()
}
