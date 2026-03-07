// Package config loads environment variables and provides a thread-safe Config struct
// that can be modified concurrently by the Telegram bot and read by the scraper loop.
package config

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

// Config holds all runtime configuration for the sniper bot.
// It is protected by a mutex to allow concurrent updates from Telegram commands.
type Config struct {
	mu sync.RWMutex

	// Telegram auth
	telegramToken string
	chatID        int64

	// Product
	targetURL   string
	targetPrice float64

	// Timing & Behavior
	checkInterval      time.Duration
	jitterRange        time.Duration
	coolOffBase        time.Duration
	smartNotifications bool
	isTracking         bool
	lastCheckedTime    time.Time
	lastNotifiedTime   time.Time
}

// Load reads the .env file (if present) and populates a Config with
// environment variables or sensible defaults.
func Load() (*Config, error) {
	// Best-effort load; missing .env is not fatal.
	_ = godotenv.Load()

	chatIDStr := os.Getenv("CHAT_ID")
	chatID, _ := strconv.ParseInt(chatIDStr, 10, 64)

	cfg := &Config{
		telegramToken: os.Getenv("TELEGRAM_TOKEN"),
		chatID:        chatID,

		targetURL:   "https://www.amazon.it/dp/B0FHL3385S",
		targetPrice: 900.00,

		// Default timing
		checkInterval: 60 * time.Second,
		jitterRange:   5 * time.Second,
		coolOffBase:   5 * time.Minute,

		smartNotifications: true,
		isTracking:         true,
	}

	intervalStr := os.Getenv("CHECK_INTERVAL")
	if intervalStr != "" {
		if intervalSecs, err := strconv.Atoi(intervalStr); err == nil && intervalSecs > 0 {
			cfg.checkInterval = time.Duration(intervalSecs) * time.Second
		}
	}

	smartNotifStr := os.Getenv("SMART_NOTIFICATIONS")
	if smartNotifStr == "false" || smartNotifStr == "0" {
		cfg.smartNotifications = false
	}

	if v := os.Getenv("TARGET_URL"); v != "" {
		cfg.targetURL = v
	}
	if v := os.Getenv("TARGET_PRICE"); v != "" {
		if p, err := strconv.ParseFloat(v, 64); err == nil {
			cfg.targetPrice = p
		}
	}

	if cfg.telegramToken == "" || cfg.chatID == 0 {
		return nil, fmt.Errorf("TELEGRAM_TOKEN and CHAT_ID must be set and valid")
	}

	return cfg, nil
}

// ── Getters ─────────────────────────────────────────────────────────────

func (c *Config) TelegramToken() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.telegramToken
}

func (c *Config) ChatID() int64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.chatID
}

func (c *Config) TargetURL() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.targetURL
}

func (c *Config) TargetPrice() float64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.targetPrice
}

func (c *Config) CheckInterval() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.checkInterval
}

func (c *Config) JitterRange() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.jitterRange
}

func (c *Config) CoolOffBase() time.Duration {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.coolOffBase
}

func (c *Config) SmartNotifications() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.smartNotifications
}

func (c *Config) IsTracking() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.isTracking
}

func (c *Config) LastCheckedTime() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastCheckedTime
}

func (c *Config) LastNotifiedTime() time.Time {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.lastNotifiedTime
}

// ── Setters ─────────────────────────────────────────────────────────────

func (c *Config) SetTargetURL(url string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.targetURL = url
	return c.save()
}

func (c *Config) SetTargetPrice(price float64) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.targetPrice = price
	return c.save()
}

func (c *Config) SetCheckInterval(d time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checkInterval = d
	return c.save()
}

func (c *Config) SetSmartNotifications(enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.smartNotifications = enabled
	return c.save()
}

func (c *Config) SetIsTracking(tracking bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.isTracking = tracking
	// We don't necessarily need to persist "IsTracking" pausing across restarts,
	// but it doesn't hurt. We'll leave it in memory for now unless requested.
	return nil 
}

func (c *Config) SetLastCheckedTime(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastCheckedTime = t
}

func (c *Config) SetLastNotifiedTime(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lastNotifiedTime = t
}

// save writes the current configuration back to the .env file.
// It assumes the caller already holds the lock.
func (c *Config) save() error {
	// Read existing to preserve other vars like TELEGRAM_TOKEN
	envMap, err := godotenv.Read()
	if err != nil {
		envMap = make(map[string]string)
	}

	envMap["TARGET_URL"] = c.targetURL
	envMap["TARGET_PRICE"] = fmt.Sprintf("%.2f", c.targetPrice)
	envMap["CHECK_INTERVAL"] = fmt.Sprintf("%.0f", c.checkInterval.Seconds())
	if c.smartNotifications {
		envMap["SMART_NOTIFICATIONS"] = "true"
	} else {
		envMap["SMART_NOTIFICATIONS"] = "false"
	}

	// Make sure we keep the critical bot tokens intact
	envMap["TELEGRAM_TOKEN"] = c.telegramToken
	envMap["CHAT_ID"] = fmt.Sprintf("%d", c.chatID)

	return godotenv.Write(envMap, ".env")
}
