# Amazon Sniper Bot 🎯

A high-performance stealth monitoring bot in Go that tracks the **Google Pixel 10 Pro 256GB** on Amazon.it. Sends instant Telegram alerts when the product becomes available below your target price.

## Features

- **TLS Fingerprint Spoofing** — Impersonates Chrome 124 at the TLS/JA3 level via `bogdanfinn/tls-client`
- **HTTP/2 Consistency** — Correct pseudo-header order, SETTINGS frame, and connection flow
- **Exact Chrome Header Order** — Client Hints, Fetch Metadata, and all headers in real wire order
- **Session Priming** — Visits Amazon homepage first to acquire legitimate session cookies
- **User-Agent Rotation** — Rotates through a pool of 5 Chrome UAs (Win/Mac/Linux)
- **Escalating Back-off** — 5min → 10min → 20min cooldown on anti-bot blocks, resets on success
- **Randomized Polling** — 60s ± 5s jitter to avoid mechanical patterns
- **Graceful Shutdown** — Handles SIGINT/SIGTERM cleanly

## Quick Start

### 1. Prerequisites
- Go 1.21+
- A Telegram bot token (from [@BotFather](https://t.me/BotFather))
- Your Telegram chat ID (from [@userinfobot](https://t.me/userinfobot))

### 2. Setup
```bash
# Clone and enter the project
cd amazon-tracker

# Copy and edit the environment file
cp .env.example .env
# Edit .env with your Telegram credentials

# Download dependencies
go mod download
```

### 3. Build & Run
```bash
# Build
go build -o sniper ./cmd/sniper

# Run
./sniper
```

Or run directly:
```bash
go run ./cmd/sniper
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `TELEGRAM_TOKEN` | *(required)* | Bot token from @BotFather |
| `CHAT_ID` | *(required)* | Your Telegram user ID |
| `TARGET_URL` | `https://www.amazon.it/dp/B0FHL3385S` | Amazon product URL |
| `ASIN` | `B0FHL3385S` | Amazon product ASIN |
| `TARGET_PRICE` | `900.00` | Alert threshold in EUR |

## Project Structure

```
amazon-tracker/
├── cmd/sniper/main.go              # Entry point & monitoring loop
├── internal/
│   ├── config/config.go            # .env loading & Config struct
│   ├── client/client.go            # Stealth TLS client factory
│   ├── scraper/scraper.go          # Page fetching & DOM parsing
│   └── notifier/notifier.go        # Telegram Bot API integration
├── .env.example
├── go.mod
└── README.md
```

## License

Personal use only. Not affiliated with Amazon.
