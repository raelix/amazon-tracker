// Package config loads environment variables and provides a typed Config struct.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration for the sniper bot.
type Config struct {
	// Telegram
	TelegramToken string
	ChatID        string

	// Product
	TargetURL   string
	ASIN        string
	TargetPrice float64

	// Timing & Behavior
	CheckInterval      time.Duration
	JitterRange        time.Duration
	CoolOffBase        time.Duration
	SmartNotifications bool
}

// Load reads the .env file (if present) and populates a Config with
// environment variables or sensible defaults.
func Load() (*Config, error) {
	// Best-effort load; missing .env is not fatal.
	_ = godotenv.Load()

	cfg := &Config{
		TelegramToken: os.Getenv("TELEGRAM_TOKEN"),
		ChatID:        os.Getenv("CHAT_ID"),

		TargetURL:   "https://www.amazon.it/dp/B0FHL3385S",
		ASIN:        "B0FHL3385S",
		TargetPrice: 900.00,

		// CheckInterval will be set after parsing
		JitterRange: 5 * time.Second,
		CoolOffBase: 5 * time.Minute,
	}

	intervalStr := os.Getenv("CHECK_INTERVAL")
	if intervalStr == "" {
		intervalStr = "30"
	}
	intervalSecs, err := strconv.Atoi(intervalStr)
	if err != nil || intervalSecs <= 0 {
		intervalSecs = 60
	}
	cfg.CheckInterval = time.Duration(intervalSecs) * time.Second

	smartNotifStr := os.Getenv("SMART_NOTIFICATIONS")
	if smartNotifStr == "false" || smartNotifStr == "0" {
		cfg.SmartNotifications = false
	} else {
		cfg.SmartNotifications = true // Default to true
	}

	// Allow overriding via env vars.
	if v := os.Getenv("TARGET_URL"); v != "" {
		cfg.TargetURL = v
	}
	if v := os.Getenv("ASIN"); v != "" {
		cfg.ASIN = v
	}
	if v := os.Getenv("TARGET_PRICE"); v != "" {
		p, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid TARGET_PRICE: %w", err)
		}
		cfg.TargetPrice = p
	}

	if cfg.TelegramToken == "" || cfg.ChatID == "" {
		return nil, fmt.Errorf("TELEGRAM_TOKEN and CHAT_ID must be set (via .env or environment)")
	}

	return cfg, nil
}
