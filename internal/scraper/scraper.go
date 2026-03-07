// Package scraper fetches an Amazon product page and extracts price/availability.
package scraper

import (
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"

	"math/rand"

	"github.com/PuerkitoBio/goquery"

	tls_client "github.com/bogdanfinn/tls-client"

	"amazon-tracker/internal/client"
)

// Result contains the parsed data from a product page scrape.
type Result struct {
	Price     float64
	Available bool
	Condition string // "new", "used", or "none"
}

// Scraper manages fetching and parsing an Amazon product page.
type Scraper struct {
	client     tls_client.HttpClient
	productURL string
	baseURL    string
}

// New creates a Scraper for the given product URL.
func New(c tls_client.HttpClient, productURL string) *Scraper {
	return &Scraper{
		client:     c,
		productURL: productURL,
		baseURL:    "https://www.amazon.it",
	}
}

// PrimeSession performs a GET to the Amazon homepage to acquire session cookies
// (session-id, ubid-acbit, i18n-prefs, sp-cdn) before scraping the product page.
// This mimics a real user who browses from the homepage.
func (s *Scraper) PrimeSession() error {
	req, err := client.BuildRequest(s.baseURL)
	if err != nil {
		return fmt.Errorf("prime session: build request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("prime session: request failed: %w", err)
	}
	defer resp.Body.Close()

	// Drain the body to allow connection reuse.
	_, _ = io.Copy(io.Discard, resp.Body)

	// Random pause to simulate a human reading the homepage (2-4s).
	pause := 2*time.Second + time.Duration(rand.Intn(2000))*time.Millisecond
	time.Sleep(pause)

	return nil
}

// Check fetches the product page and returns a Result with price and availability.
// Returns an error with the HTTP status code embedded for the caller to handle
// rate-limiting (403/503).
func (s *Scraper) Check() (*Result, error) {
	req, err := client.BuildRequest(s.productURL)
	if err != nil {
		return nil, fmt.Errorf("check: build request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("check: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 403 || resp.StatusCode == 503 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, &BlockedError{StatusCode: resp.StatusCode}
	}

	if resp.StatusCode != 200 {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil, fmt.Errorf("check: unexpected status %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("check: parse HTML: %w", err)
	}

	return parseProduct(doc), nil
}

// parseProduct extracts the NEW product price and availability from the page.
//
// Amazon product pages have several price locations:
//   - Recommendation carousels (.cerberus / .lpo widgets) — WRONG, other products
//   - #usedOnlyBuybox — used/renewed offers only
//   - #newBuyBox or #corePrice_feature_div — new item price
//   - .priceToPay inside the main buybox — primary display price
//   - #aod-ingress-link — "other sellers" link price
//
// We specifically target the NEW offer price and ignore used/carousel prices.
func parseProduct(doc *goquery.Document) *Result {
	result := &Result{}

	// ── Detect buybox type ──────────────────────────────────────────────
	// If #usedOnlyBuybox exists AND there's no #newBuyBox / #corePrice_feature_div,
	// then only used/renewed offers are available — no new product for sale.
	hasUsedOnlyBuybox := doc.Find("#usedOnlyBuybox").Length() > 0
	hasNewBuyBox := doc.Find("#newBuyBox").Length() > 0
	hasCorePrice := doc.Find("#corePrice_feature_div").Length() > 0
	hasNewAddToCart := doc.Find("#add-to-cart-button").Length() > 0

	// Also check the accordion-based buybox for new offers
	hasNewAccordion := false
	doc.Find("#buyBoxAccordion .a-accordion-row, .dp-accordion-row").Each(func(i int, s *goquery.Selection) {
		txt := strings.ToLower(s.Text())
		if strings.Contains(txt, "nuovo") || strings.Contains(txt, "compra nuovo") {
			hasNewAccordion = true
		}
	})

	isNewAvailable := hasNewBuyBox || hasCorePrice || hasNewAddToCart || hasNewAccordion

	if hasUsedOnlyBuybox && !isNewAvailable {
		// Page only shows used/renewed offers — no new product.
		result.Condition = "used"
		result.Available = false
		result.Price = 0

		// Still log the used price for informational purposes.
		usedPrice := extractPriceFromContainer(doc, "#usedOnlyBuybox")
		if usedPrice > 0 {
			log.Printf("[scraper] Used-only buybox detected. Lowest used price: %.2f EUR (ignoring, want new)", usedPrice)
		} else {
			log.Println("[scraper] Used-only buybox detected, no new offer available")
		}
		return result
	}

	// ── Extract NEW price ───────────────────────────────────────────────
	// Try several selectors in order of reliability for the NEW product price.
	var price float64

	// 1. Accordion buybox: look for the ID starting with "newAccordionRow" (e.g., #newAccordionRow_0)
	//    This is the most robust way to find the NEW price on multi-offer pages.
	if price == 0 && hasNewAccordion {
		doc.Find("[id^='newAccordionRow'] .a-price-whole").Each(func(i int, s *goquery.Selection) {
			if price > 0 {
				return
			}
			price = extractPriceFromElement(s)
		})

		// Fallback for accordion text search if ID isn't there
		if price == 0 {
			doc.Find("#buyBoxAccordion .a-accordion-row, .dp-accordion-row").Each(func(i int, s *goquery.Selection) {
				if price > 0 {
					return
				}
				txt := strings.ToLower(s.Text())
				if strings.Contains(txt, "nuovo") || strings.Contains(txt, "compra nuovo") {
					whole := s.Find(".a-price-whole").First()
					price = extractPriceFromElement(whole)
				}
			})
		}
	}

	// 2. #newBuyBox is used on some product pages for the new offer.
	if price == 0 && hasNewBuyBox {
		price = extractPriceFromContainer(doc, "#newBuyBox")
	}

	// 3. #corePrice_feature_div contains the price for new items on standard pages.
	// We only use this if we didn't find an accordion, because corePrice can contain both new and used prices!
	if price == 0 && hasCorePrice && !hasNewAccordion {
		price = extractPriceFromContainer(doc, "#corePrice_feature_div")
	}

	// 4. The .priceToPay element inside the buybox (NOT inside carousels/widgets).
	//    This is shown as the main price on the product page when the new offer is active.
	if price == 0 && !hasNewAccordion {
		doc.Find("#desktop_buybox .priceToPay .a-price-whole, #buybox .priceToPay .a-price-whole").Each(func(i int, s *goquery.Selection) {
			if price > 0 {
				return // already found
			}
			price = extractPriceFromElement(s)
		})
	}

	// 5. Fallback: the .apex-core-price-identifier container (used on some layouts).
	if price == 0 && !hasNewAccordion {
		price = extractPriceFromContainer(doc, ".apex-core-price-identifier")
	}

	result.Price = price

	// ── Availability ────────────────────────────────────────────────────
	// For a new item, the #add-to-cart-button must exist (not #add-to-cart-button-ubb).
	// Also check the availability text.
	availText := strings.TrimSpace(doc.Find("#availability").Text())
	isUnavailable := strings.Contains(availText, "Attualmente non disponibile") ||
		strings.Contains(availText, "Non disponibile")

	if hasNewAddToCart {
		result.Available = true
		result.Condition = "new"
	} else if isNewAvailable && !isUnavailable && price > 0 {
		result.Available = true
		result.Condition = "new"
	} else {
		result.Available = false
		if isNewAvailable {
			result.Condition = "new"
		} else {
			result.Condition = "none"
		}
	}

	return result
}

// extractPriceFromContainer finds the first .a-price-whole inside the given
// CSS selector container and returns the parsed price.
func extractPriceFromContainer(doc *goquery.Document, containerSelector string) float64 {
	container := doc.Find(containerSelector)
	if container.Length() == 0 {
		return 0
	}
	whole := container.Find(".a-price-whole").First()
	return extractPriceFromElement(whole)
}

// extractPriceFromElement parses an .a-price-whole element and its sibling
// .a-price-fraction into a float64.
// Amazon Italy uses the format "1.049,00" (dot = thousands, comma = decimal).
func extractPriceFromElement(whole *goquery.Selection) float64 {
	if whole == nil || whole.Length() == 0 {
		return 0
	}

	wholeText := strings.TrimSpace(whole.Text())
	if wholeText == "" {
		return 0
	}

	// Get the fraction from the sibling element.
	fractionText := strings.TrimSpace(whole.Parent().Find(".a-price-fraction").First().Text())

	// Remove trailing comma/dot separators from the whole part.
	wholeText = strings.TrimRight(wholeText, ",.")
	// Remove thousand separators (dots).
	wholeText = strings.ReplaceAll(wholeText, ".", "")

	if fractionText == "" {
		fractionText = "00"
	}

	priceStr := wholeText + "." + fractionText
	p, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		return 0
	}
	return p
}

// BlockedError signals that Amazon returned an anti-bot response.
type BlockedError struct {
	StatusCode int
}

func (e *BlockedError) Error() string {
	return fmt.Sprintf("blocked by Amazon (HTTP %d)", e.StatusCode)
}
