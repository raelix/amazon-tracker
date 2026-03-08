// Package config loads environment variables and provides thread-safe Config
// and ItemStore types. Config holds global settings (from .env), ItemStore
// manages the list of tracked items (persisted to items.json).
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

// ── Global Config ───────────────────────────────────────────────────────

// Config holds global runtime settings loaded from environment variables.
type Config struct {
	mu sync.RWMutex

	// Telegram auth
	telegramToken string
	chatID        int64

	// Timing & Behavior
	checkInterval      time.Duration
	jitterRange        time.Duration
	coolOffBase        time.Duration
	smartNotifications bool

	// Language ("en" or "it")
	language string
}

// Load reads the .env file (if present) and populates a Config from
// environment variables or sensible defaults.
func Load() (*Config, error) {
	_ = godotenv.Load()

	chatIDStr := os.Getenv("CHAT_ID")
	chatID, _ := strconv.ParseInt(chatIDStr, 10, 64)

	cfg := &Config{
		telegramToken: os.Getenv("TELEGRAM_TOKEN"),
		chatID:        chatID,

		checkInterval: 60 * time.Second,
		jitterRange:   5 * time.Second,
		coolOffBase:   5 * time.Minute,

		smartNotifications: true,
		language:           "en",
	}

	if v := os.Getenv("CHECK_INTERVAL"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			cfg.checkInterval = time.Duration(secs) * time.Second
		}
	}

	if v := os.Getenv("SMART_NOTIFICATIONS"); v == "false" || v == "0" {
		cfg.smartNotifications = false
	}

	if v := os.Getenv("LANGUAGE"); v == "it" || v == "en" {
		cfg.language = v
	}

	if cfg.telegramToken == "" || cfg.chatID == 0 {
		return nil, fmt.Errorf("TELEGRAM_TOKEN and CHAT_ID must be set and valid")
	}

	return cfg, nil
}

// ── Config Getters ──────────────────────────────────────────────────────

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

func (c *Config) Language() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.language
}

// ── Config Setters ──────────────────────────────────────────────────────

func (c *Config) SetCheckInterval(d time.Duration) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checkInterval = d
	return c.saveEnv()
}

func (c *Config) SetSmartNotifications(enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.smartNotifications = enabled
	return c.saveEnv()
}

func (c *Config) SetLanguage(lang string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.language = lang
	return c.saveEnv()
}

// saveEnv writes global settings back to .env, preserving secrets.
func (c *Config) saveEnv() error {
	envMap, err := godotenv.Read()
	if err != nil {
		envMap = make(map[string]string)
	}

	envMap["TELEGRAM_TOKEN"] = c.telegramToken
	envMap["CHAT_ID"] = fmt.Sprintf("%d", c.chatID)
	envMap["CHECK_INTERVAL"] = fmt.Sprintf("%.0f", c.checkInterval.Seconds())
	envMap["LANGUAGE"] = c.language
	if c.smartNotifications {
		envMap["SMART_NOTIFICATIONS"] = "true"
	} else {
		envMap["SMART_NOTIFICATIONS"] = "false"
	}

	return godotenv.Write(envMap, ".env")
}

// ── Item ────────────────────────────────────────────────────────────────

// Item represents a single tracked Amazon product.
type Item struct {
	ID          int     `json:"id"`
	URL         string  `json:"url"`
	TargetPrice float64 `json:"target_price"`
	Enabled     bool    `json:"enabled"`

	// Runtime state — not persisted to JSON.
	LastCheckedTime   time.Time `json:"-"`
	LastNotifiedTime  time.Time `json:"-"`
	LastNotifiedPrice float64   `json:"-"`

	// Last check results — cached from the most recent scrape.
	LastPrice     float64 `json:"-"`
	LastUsedPrice float64 `json:"-"`
	LastCondition string  `json:"-"`
	LastAvailable bool    `json:"-"`
}

// Label returns a short human-readable label for Telegram display.
// It extracts the ASIN from the URL if possible, otherwise truncates.
func (it *Item) Label() string {
	// Try to extract ASIN (typically /dp/XXXXXXXXXX or /gp/product/XXXXXXXXXX)
	url := it.URL
	for _, prefix := range []string{"/dp/", "/gp/product/"} {
		if idx := indexOf(url, prefix); idx >= 0 {
			start := idx + len(prefix)
			end := start + 10
			if end > len(url) {
				end = len(url)
			}
			// Trim trailing slash or query
			asin := url[start:end]
			for i, ch := range asin {
				if ch == '/' || ch == '?' {
					asin = asin[:i]
					break
				}
			}
			if asin != "" {
				return asin
			}
		}
	}
	// Fallback: truncated URL
	if len(url) > 40 {
		return url[:40] + "…"
	}
	return url
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// ── ItemStore ───────────────────────────────────────────────────────────

// ItemStore manages the tracked items list with thread-safe CRUD and
// JSON file persistence.
type ItemStore struct {
	mu       sync.RWMutex
	items    []*Item
	nextID   int
	filePath string
}

// LoadItems reads items from the given JSON file. If the file does not
// exist, an empty store is returned (the file will be created on first Add).
func LoadItems(filePath string) (*ItemStore, error) {
	store := &ItemStore{
		filePath: filePath,
		items:    make([]*Item, 0),
		nextID:   1,
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return nil, fmt.Errorf("read items file: %w", err)
	}

	if len(data) == 0 {
		return store, nil
	}

	var items []*Item
	if err := json.Unmarshal(data, &items); err != nil {
		return nil, fmt.Errorf("parse items file: %w", err)
	}

	store.items = items

	// Compute nextID as max(existing IDs) + 1
	for _, it := range items {
		if it.ID >= store.nextID {
			store.nextID = it.ID + 1
		}
	}

	return store, nil
}

// All returns a copy of all items.
func (s *ItemStore) All() []*Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]*Item, len(s.items))
	copy(out, s.items)
	return out
}

// EnabledItems returns items where Enabled == true.
func (s *ItemStore) EnabledItems() []*Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var out []*Item
	for _, it := range s.items {
		if it.Enabled {
			out = append(out, it)
		}
	}
	return out
}

// Get returns the item with the given ID, or nil if not found.
func (s *ItemStore) Get(id int) *Item {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, it := range s.items {
		if it.ID == id {
			return it
		}
	}
	return nil
}

// Add creates a new item and persists to disk.
func (s *ItemStore) Add(url string, targetPrice float64) (*Item, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := &Item{
		ID:          s.nextID,
		URL:         url,
		TargetPrice: targetPrice,
		Enabled:     true,
	}
	s.nextID++
	s.items = append(s.items, item)

	if err := s.save(); err != nil {
		// Rollback
		s.items = s.items[:len(s.items)-1]
		s.nextID--
		return nil, err
	}
	return item, nil
}

// Remove deletes the item with the given ID and persists to disk.
func (s *ItemStore) Remove(id int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := -1
	for i, it := range s.items {
		if it.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		return fmt.Errorf("item %d not found", id)
	}

	removed := s.items[idx]
	s.items = append(s.items[:idx], s.items[idx+1:]...)

	if err := s.save(); err != nil {
		// Rollback
		s.items = append(s.items[:idx], append([]*Item{removed}, s.items[idx:]...)...)
		return err
	}
	return nil
}

// Update modifies the URL and/or target price of an item.
// Pass empty url or zero price to leave unchanged.
func (s *ItemStore) Update(id int, url string, targetPrice float64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.getUnlocked(id)
	if item == nil {
		return fmt.Errorf("item %d not found", id)
	}

	oldURL, oldPrice := item.URL, item.TargetPrice
	if url != "" {
		item.URL = url
	}
	if targetPrice > 0 {
		item.TargetPrice = targetPrice
	}

	if err := s.save(); err != nil {
		item.URL, item.TargetPrice = oldURL, oldPrice
		return err
	}
	return nil
}

// SetEnabled sets the enabled state of an item.
func (s *ItemStore) SetEnabled(id int, enabled bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	item := s.getUnlocked(id)
	if item == nil {
		return fmt.Errorf("item %d not found", id)
	}

	item.Enabled = enabled
	return s.save()
}

// getUnlocked returns an item by ID without acquiring the lock.
// Caller must hold the lock.
func (s *ItemStore) getUnlocked(id int) *Item {
	for _, it := range s.items {
		if it.ID == id {
			return it
		}
	}
	return nil
}

// save writes the items slice to the JSON file.
// Caller must hold the lock.
func (s *ItemStore) save() error {
	data, err := json.MarshalIndent(s.items, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal items: %w", err)
	}
	return os.WriteFile(s.filePath, data, 0644)
}
