# Amazon Sniper Bot 🎯

A high-performance stealth monitoring bot in Go that tracks **multiple Amazon products** and sends instant Telegram alerts when any product drops below your target price. Manage everything via an interactive inline-keyboard Telegram interface.

## Features

- **Multi-Item Tracking** — Monitor multiple Amazon products simultaneously
- **Inline Keyboard UI** — Tap-to-manage interface: add, edit, pause, remove items via buttons
- **Per-Item Alerts** — Individual target prices and notification state per product
- **TLS Fingerprint Spoofing** — Impersonates Chrome 124 at the TLS/JA3 level
- **HTTP/2 Consistency** — Correct pseudo-header order, SETTINGS frame, and connection flow
- **Session Priming** — Visits Amazon homepage first to acquire legitimate session cookies
- **User-Agent Rotation** — Rotates through a pool of Chrome UAs
- **Escalating Back-off** — Progressive cooldown on anti-bot blocks, resets on success
- **Randomized Polling** — Configurable interval with jitter to avoid mechanical patterns
- **Graceful Shutdown** — Handles SIGINT/SIGTERM cleanly

## Quick Start

### 1. Prerequisites
- Go 1.21+
- A Telegram bot token (from [@BotFather](https://t.me/BotFather))
- Your Telegram chat ID (from [@userinfobot](https://t.me/userinfobot))

### 2. Setup
```bash
cd amazon-tracker

# Copy and edit the environment file (secrets only)
cp .env.example .env
# Edit .env with your Telegram credentials

# Optionally pre-populate items (or use /add in Telegram)
cp items.json.example items.json

# Download dependencies
go mod download
```

### 3. Build & Run
```bash
go build -o sniper ./cmd/sniper
./sniper
```

Or run directly:
```bash
go run ./cmd/sniper
```

## Configuration

### Environment Variables (`.env`)

| Variable | Default | Description |
|----------|---------|-------------|
| `TELEGRAM_TOKEN` | *(required)* | Bot token from @BotFather |
| `CHAT_ID` | *(required)* | Your Telegram user ID |
| `CHECK_INTERVAL` | `60` | Seconds between check cycles |
| `SMART_NOTIFICATIONS` | `true` | Only alert on price changes |

### Items File (`items.json`)

Tracked items are stored in `items.json`. You can edit this file directly or manage items via Telegram commands.

```json
[
  {
    "id": 1,
    "url": "https://www.amazon.it/dp/B0FHL3385S",
    "target_price": 900.00,
    "enabled": true
  },
  {
    "id": 2,
    "url": "https://www.amazon.it/dp/B0FHL38J26",
    "target_price": 1000.00,
    "enabled": true
  }
]
```

## Telegram Commands

| Command | Description |
|---------|-------------|
| `/list` | Show all items as tappable buttons |
| `/add` | Add a new item (interactive prompts) |
| `/check` | Force check all enabled items |
| `/status` | Show global settings |
| `/setinterval` | Set check interval in seconds |
| `/smart` | Toggle smart notifications |
| `/help` | Show command list |

### Inline Item Actions

Tap any item from `/list` to see its detail card with action buttons:

- **✏️ Edit** → Change URL or target price
- **🔍 Check** → Force check this item
- **⏸ Pause / ▶️ Resume** → Toggle tracking
- **🗑 Remove** → Delete item (with confirmation)
- **🔙 Back** → Return to item list

## Project Structure

```
amazon-tracker/
├── cmd/sniper/main.go              # Entry point & multi-item monitoring loop
├── internal/
│   ├── config/config.go            # Global config (.env) + ItemStore (items.json)
│   ├── config/config_test.go       # Unit tests for ItemStore CRUD
│   ├── client/client.go            # Stealth TLS client factory
│   ├── scraper/scraper.go          # Page fetching & DOM parsing
│   └── telegram/telegram.go        # Telegram bot with inline keyboards
├── .env.example
├── items.json.example
├── go.mod
└── README.md
```

## License

Personal use only. Not affiliated with Amazon.
