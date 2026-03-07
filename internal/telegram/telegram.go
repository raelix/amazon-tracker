package telegram

import (
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	tls_client "github.com/bogdanfinn/tls-client"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api/v5"

	"amazon-tracker/internal/config"
)

// Bot manages Telegram interactions.
type Bot struct {
	api            *tgbotapi.BotAPI
	cfg            *config.Config
	wakeChan       chan struct{}
	forceCheckChan chan struct{}
	awaitingState  string // State machine for conversational prompts
}

// New creates a new Telegram Bot instance.
func New(httpClient tls_client.HttpClient, cfg *config.Config, wakeChan chan struct{}, forceCheckChan chan struct{}) (*Bot, error) {
	api, err := tgbotapi.NewBotAPI(cfg.TelegramToken())
	if err != nil {
		return nil, fmt.Errorf("init telegram bot: %w", err)
	}

	api.Debug = false

	// Register commands for Telegram UI
	commands := tgbotapi.NewSetMyCommands(
		tgbotapi.BotCommand{Command: "start", Description: "Show welcome message and commands"},
		tgbotapi.BotCommand{Command: "help", Description: "Show available commands"},
		tgbotapi.BotCommand{Command: "status", Description: "Show current configuration and state"},
		tgbotapi.BotCommand{Command: "check", Description: "Force an immediate scrape to report prices"},
		tgbotapi.BotCommand{Command: "track", Description: "Pause or resume scraping (start/stop)"},
		tgbotapi.BotCommand{Command: "seturl", Description: "Change the Amazon product URL"},
		tgbotapi.BotCommand{Command: "setprice", Description: "Set target price alert"},
		tgbotapi.BotCommand{Command: "setinterval", Description: "Set poll interval in seconds"},
		tgbotapi.BotCommand{Command: "smart", Description: "Toggle smart notifications (on/off)"},
		tgbotapi.BotCommand{Command: "cancel", Description: "Cancel the current interactive prompt"},
	)
	if _, err := api.Request(commands); err != nil {
		log.Printf("[WARN] Failed to register telegram commands: %v", err)
	}

	return &Bot{
		api:            api,
		cfg:            cfg,
		wakeChan:       wakeChan,
		forceCheckChan: forceCheckChan,
		awaitingState:  "",
	}, nil
}

// SendAlert sends a formatted product alert message.
func (b *Bot) SendAlert(price float64, productURL string) error {
	text := fmt.Sprintf(
		"🚨 <b>AMAZON SNIPER ALERT!</b>\n\n"+
			"💰 Prezzo: <b>%.2f EUR</b>\n"+
			"🔗 <a href=\"%s\">Acquista su Amazon</a>",
		price, productURL,
	)

	msg := tgbotapi.NewMessage(b.cfg.ChatID(), text)
	msg.ParseMode = tgbotapi.ModeHTML
	msg.DisableWebPagePreview = false

	_, err := b.api.Send(msg)
	return err
}

// SendRaw sends a raw message string.
func (b *Bot) SendRaw(text string) error {
	msg := tgbotapi.NewMessage(b.cfg.ChatID(), text)
	msg.ParseMode = tgbotapi.ModeHTML
	_, err := b.api.Send(msg)
	if err != nil {
		log.Printf("[ERROR] Telegram send failed: %v | Text: %s", err, text)
	}
	return err
}

// StartReceiver begins listening to incoming Telegram updates in the background.
func (b *Bot) StartReceiver() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60

	updates := b.api.GetUpdatesChan(u)

	go func() {
		log.Println("[INFO] Telegram command receiver started.")
		for update := range updates {
			if update.Message == nil {
				continue
			}

			// Only allow commands from our authorized ChatID
			if update.Message.Chat.ID != b.cfg.ChatID() {
				log.Printf("[WARN] Unauthorized message from chat %d", update.Message.Chat.ID)
				continue
			}

			if update.Message.IsCommand() {
				cmd := update.Message.Command()
				args := update.Message.CommandArguments()
				b.handleCommand(cmd, args)
			} else if b.awaitingState != "" {
				// Use the unformatted message text as the argument for the pending state
				text := strings.TrimSpace(update.Message.Text)
				b.handleCommand(b.awaitingState, text)
			}
		}
	}()
}

func (b *Bot) handleCommand(cmd string, args string) {
	log.Printf("[INFO] Telegram action: %s | Args: %s", cmd, args)
	replyText := ""

	switch cmd {
	case "start", "help":
		b.awaitingState = ""
		replyText = b.helpText()
	case "status":
		b.awaitingState = ""
		replyText = b.statusText()
	case "check":
		b.awaitingState = ""
		replyText = "🔍 <b>Forcing an immediate check...</b>"
		
		// Send non-blocking signal to trigger a check
		select {
		case b.forceCheckChan <- struct{}{}:
		default:
		}
	case "cancel":
		b.awaitingState = ""
		replyText = "🚫 Prompt cancelled."
	case "seturl":
		if args == "" {
			b.awaitingState = "seturl"
			replyText = "🔗 <b>Send me the new Amazon URL:</b>\n<i>(Or type /cancel)</i>"
		} else {
			if err := b.cfg.SetTargetURL(strings.TrimSpace(args)); err != nil {
				replyText = "⚠️ Config updated in memory, but failed to save to .env"
			} else {
				replyText = "✅ Target URL updated and saved."
			}
			b.awaitingState = ""
			b.wake()
		}
	case "setprice":
		if args == "" {
			b.awaitingState = "setprice"
			replyText = "💰 <b>Send me the new target price (e.g. 850.00):</b>\n<i>(Or type /cancel)</i>"
		} else {
			if p, err := strconv.ParseFloat(strings.ReplaceAll(args, ",", "."), 64); err == nil && p > 0 {
				if err := b.cfg.SetTargetPrice(p); err != nil {
					replyText = fmt.Sprintf("⚠️ Config updated to %.2f EUR, but failed to save to .env", p)
				} else {
					replyText = fmt.Sprintf("✅ Target price updated to %.2f EUR and saved.", p)
				}
				b.awaitingState = ""
				b.wake()
			} else {
				replyText = "❌ Invalid price. Try again (e.g. 800.00) or /cancel"
			}
		}
	case "setinterval":
		if args == "" {
			b.awaitingState = "setinterval"
			replyText = "⏱ <b>Send me the new check interval in seconds (e.g. 60):</b>\n<i>(Or type /cancel)</i>"
		} else {
			if sec, err := strconv.Atoi(args); err == nil && sec > 0 {
				if err := b.cfg.SetCheckInterval(time.Duration(sec) * time.Second); err != nil {
					replyText = fmt.Sprintf("⚠️ Check interval updated to %d sec, but failed to save.", sec)
				} else {
					replyText = fmt.Sprintf("✅ Check interval updated to %d seconds and saved.", sec)
				}
				b.awaitingState = ""
				b.wake()
			} else {
				replyText = "❌ Invalid interval. Try again (e.g. 60) or /cancel"
			}
		}
	case "smart":
		if args == "" {
			b.awaitingState = "smart"
			replyText = "🧠 <b>Enable Smart Notifications? (on/off):</b>\n<i>(Or type /cancel)</i>"
		} else {
			argsLower := strings.ToLower(args)
			if argsLower == "on" || argsLower == "true" {
				if err := b.cfg.SetSmartNotifications(true); err != nil {
					replyText = "⚠️ Smart Notifications enabled (not saved to .env)."
				} else {
					replyText = "✅ Smart Notifications enabled and saved."
				}
				b.awaitingState = ""
			} else if argsLower == "off" || argsLower == "false" {
				if err := b.cfg.SetSmartNotifications(false); err != nil {
					replyText = "⚠️ Smart Notifications disabled (not saved to .env)."
				} else {
					replyText = "✅ Smart Notifications disabled and saved."
				}
				b.awaitingState = ""
			} else {
				replyText = "❌ Please reply with exactly `on` or `off`, or /cancel"
			}
		}
	case "track":
		if args == "" {
			b.awaitingState = "track"
			replyText = "▶️ <b>Start or stop tracking? (start/stop):</b>\n<i>(Or type /cancel)</i>"
		} else {
			argsLower := strings.ToLower(args)
			if argsLower == "start" || argsLower == "on" {
				_ = b.cfg.SetIsTracking(true)
				replyText = "▶️ Tracking resumed."
				b.awaitingState = ""
				b.wake()
			} else if argsLower == "stop" || argsLower == "off" || argsLower == "pause" {
				_ = b.cfg.SetIsTracking(false)
				replyText = "⏸ Tracking paused."
				b.awaitingState = ""
				b.wake()
			} else {
				replyText = "❌ Please reply with exactly `start` or `stop`, or /cancel"
			}
		}
	default:
		b.awaitingState = ""
		replyText = "❓ Unknown command. Type <code>/help</code>."
	}

	_ = b.SendRaw(replyText)
}

func (b *Bot) wake() {
	// Non-blocking send to wake up the main loop
	select {
	case b.wakeChan <- struct{}{}:
	default:
	}
}

func (b *Bot) helpText() string {
	return `🤖 <b>Amazon Sniper Bot Commands</b>

/status - Show current configuration
/check - Force an immediate scrape and show results
/track &lt;start|stop&gt; - Pause/resume scraping
/seturl &lt;amazon_link&gt; - Change product URL
/setprice &lt;number&gt; - Set target price alert
/setinterval &lt;seconds&gt; - Set poll interval
/smart &lt;on|off&gt; - Toggle smart notifications`
}

func (b *Bot) statusText() string {
	state := "⏸ PAUSED"
	if b.cfg.IsTracking() {
		state = "▶️ RUNNING"
	}

	smart := "OFF"
	if b.cfg.SmartNotifications() {
		smart = "ON"
	}

	return fmt.Sprintf(
		"📊 <b>STATUS</b>\n"+
			"State: %s\n"+
			"Interval: %.0fs\n"+
			"Smart Notify: %s\n\n"+
			"💰 <b>TARGET</b>\n"+
			"Price: &lt; %.2f EUR\n\n"+
			"🔗 <b>URL</b>\n<a href=\"%s\">Product Link</a>",
		state,
		b.cfg.CheckInterval().Seconds(),
		smart,
		b.cfg.TargetPrice(),
		b.cfg.TargetURL(),
	)
}
