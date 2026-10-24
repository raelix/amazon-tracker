// Command sniper monitors tracked Amazon items and sends Telegram alerts
// when a product drops below its target price.
package main

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	tls_client "github.com/bogdanfinn/tls-client"

	"amazon-tracker/internal/client"
	"amazon-tracker/internal/config"
	"amazon-tracker/internal/scraper"
	"amazon-tracker/internal/telegram"
)

const itemsFile = "items.json"

// maxCoolOffSteps caps the exponential back-off (base * 2^steps).
const maxCoolOffSteps = 4

type monitor struct {
	cfg        *config.Config
	store      *config.ItemStore
	bot        *telegram.Bot
	httpClient tls_client.HttpClient
	primed     bool
	blocks     int  // consecutive anti-bot blocks
	wrongLoc   bool // last check hit a non-Italian delivery location
}

func main() {
	log.SetFlags(log.Ldate | log.Ltime)

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("[FATAL] Config: %v", err)
	}

	store, err := config.LoadItems(itemsFile)
	if err != nil {
		log.Fatalf("[FATAL] Items: %v", err)
	}

	httpClient, err := client.New()
	if err != nil {
		log.Fatalf("[FATAL] TLS client: %v", err)
	}

	wakeChan := make(chan struct{}, 1)
	forceCheckChan := make(chan int, 1)

	bot, err := telegram.New(cfg, store, wakeChan, forceCheckChan)
	if err != nil {
		log.Fatalf("[FATAL] Telegram: %v", err)
	}
	bot.StartReceiver()
	_ = bot.SendRaw(bot.Lang().BotOnline)

	m := &monitor{cfg: cfg, store: store, bot: bot, httpClient: httpClient}

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	log.Printf("[INFO] Sniper started: %d items (%d enabled), interval %v",
		len(store.All()), len(store.EnabledItems()), cfg.CheckInterval())

	for {
		coolOff := m.runCycle(store.EnabledItems(), false)

		wait := coolOff
		if wait == 0 {
			wait = m.nextInterval()
		}
		log.Printf("[INFO] Sleeping %v...", wait.Round(time.Second))

		timer := time.NewTimer(wait)
	waitLoop:
		for {
			select {
			case <-timer.C:
				break waitLoop
			case <-wakeChan:
				// Settings or items changed: restart the wait with fresh values.
				timer.Stop()
				break waitLoop
			case id := <-forceCheckChan:
				m.forceCheck(id)
			case sig := <-sigChan:
				timer.Stop()
				log.Printf("[INFO] Received %v, shutting down.", sig)
				return
			}
		}
	}
}

// nextInterval returns the configured interval with random jitter applied.
func (m *monitor) nextInterval() time.Duration {
	base := m.cfg.CheckInterval()
	jitter := m.cfg.JitterRange()
	if jitter <= 0 {
		return base
	}
	d := base + time.Duration(rand.Int63n(int64(2*jitter))) - jitter
	if d < time.Second {
		d = time.Second
	}
	return d
}

// forceCheck handles a manual check request from Telegram (0 = all items).
func (m *monitor) forceCheck(id int) {
	if id == 0 {
		items := m.store.EnabledItems()
		if len(items) == 0 {
			_ = m.bot.SendRaw(m.bot.Lang().NoItemsToCheck)
			return
		}
		m.runCycle(items, true)
		return
	}
	item := m.store.Get(id)
	if item == nil {
		_ = m.bot.SendRaw(m.bot.Lang().ItemNotFound)
		return
	}
	m.runCycle([]*config.Item{item}, true)
}

// runCycle checks the given items. It returns a non-zero cool-off duration
// if Amazon blocked a request, in which case the cycle is aborted.
func (m *monitor) runCycle(items []*config.Item, isManual bool) time.Duration {
	if !m.primed && len(items) > 0 {
		m.prime(items[0].URL)
	}

	for i, item := range items {
		if i > 0 {
			// Small random pause between items to avoid burst patterns.
			time.Sleep(time.Duration(1500+rand.Intn(2500)) * time.Millisecond)
		}

		res, err := scraper.New(m.httpClient, item.URL).Check()
		if err != nil {
			var blocked *scraper.BlockedError
			if errors.As(err, &blocked) {
				return m.handleBlocked(item, blocked)
			}
			var wrongLoc *scraper.LocationError
			if errors.As(err, &wrongLoc) {
				m.handleWrongLocation(item, wrongLoc)
				return 0
			}
			log.Printf("[ERROR] #%d (%s): %v", item.ID, item.Label(), err)
			_ = m.bot.SendRaw(fmt.Sprintf(m.bot.Lang().CheckError, item.ID, item.Label(), err))
			continue
		}

		m.blocks = 0
		if m.wrongLoc {
			m.wrongLoc = false
			log.Printf("[INFO] Delivery location is Italian again.")
		}
		item.LastCheckedTime = time.Now()
		item.LastPrice = res.Price
		item.LastUsedPrice = res.UsedPrice
		item.LastCondition = res.Condition
		item.LastAvailable = res.Available

		log.Printf("[INFO] #%d (%s): available=%v price=%.2f used=%.2f target=%.2f",
			item.ID, item.Label(), res.Available, res.Price, res.UsedPrice, item.TargetPrice)

		if isManual {
			m.bot.SendItemCheckResult(item, true)
		}
		m.maybeAlert(item, res)
	}
	return 0
}

// prime warms up the session and sets the Italian delivery location, using
// productURL as the page the location modal is opened from.
func (m *monitor) prime(productURL string) {
	if err := scraper.New(m.httpClient, "").PrimeSession(); err != nil {
		log.Printf("[WARN] Session priming failed: %v", err)
		return
	}
	if err := scraper.New(m.httpClient, productURL).SetDeliveryZip(m.cfg.DeliveryZip()); err != nil {
		log.Printf("[WARN] Setting delivery postcode %s failed: %v", m.cfg.DeliveryZip(), err)
		return
	}
	log.Printf("[INFO] Session primed, delivery postcode %s.", m.cfg.DeliveryZip())
	m.primed = true
}

// handleWrongLocation re-primes the session next cycle. It notifies on
// Telegram only when the problem starts, not on every cycle.
func (m *monitor) handleWrongLocation(item *config.Item, err *scraper.LocationError) {
	m.primed = false
	log.Printf("[WARN] #%d (%s): %v. Re-priming next cycle.", item.ID, item.Label(), err)
	if !m.wrongLoc {
		m.wrongLoc = true
		_ = m.bot.SendRaw(fmt.Sprintf(m.bot.Lang().CheckError, item.ID, item.Label(), err))
	}
}

// maybeAlert sends a price alert when the new-condition price is at or below
// target. With smart notifications, it alerts only when the price changes.
func (m *monitor) maybeAlert(item *config.Item, res *scraper.Result) {
	if !res.Available || res.Price <= 0 || res.Price > item.TargetPrice {
		item.LastNotifiedPrice = 0
		return
	}
	if m.cfg.SmartNotifications() && item.LastNotifiedPrice == res.Price {
		return
	}
	if err := m.bot.SendAlert(item, res.Price); err != nil {
		log.Printf("[ERROR] Alert send failed for #%d: %v", item.ID, err)
		return
	}
	item.LastNotifiedPrice = res.Price
	item.LastNotifiedTime = time.Now()
	log.Printf("[INFO] Alert sent! #%d (%s) at %.2f EUR (target %.2f)", item.ID, item.Label(), res.Price, item.TargetPrice)
}

// handleBlocked escalates the cool-off and re-primes the session next cycle.
func (m *monitor) handleBlocked(item *config.Item, blocked *scraper.BlockedError) time.Duration {
	steps := m.blocks
	if steps > maxCoolOffSteps {
		steps = maxCoolOffSteps
	}
	m.blocks++
	m.primed = false
	coolOff := m.cfg.CoolOffBase() * time.Duration(1<<steps)

	log.Printf("[WARN] Blocked (HTTP %d) on #%d (%s). Cooling off %v.",
		blocked.StatusCode, item.ID, item.Label(), coolOff)
	_ = m.bot.SendRaw(fmt.Sprintf(m.bot.Lang().BlockedMsg, blocked.StatusCode, item.ID, item.Label(), coolOff))
	return coolOff
}
