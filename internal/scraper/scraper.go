// Package scraper handles fetching and parsing Amazon product pages.
package scraper

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	tls_client "github.com/bogdanfinn/tls-client"

	"amazon-tracker/internal/client"
)

// Result holds the parsed data from the Amazon product page.
type Result struct {
	Price     float64
	UsedPrice float64 // Added field for second-hand prices
	Available bool
	Condition string // "new" or "used"
}

// Scraper handles the DOM parsing of an Amazon product page.
type Scraper struct {
	httpClient tls_client.HttpClient
	url        string
}

// BlockedError is returned when Amazon blocks the request (403/503).
type BlockedError struct {
	StatusCode int
}

func (e *BlockedError) Error() string {
	return fmt.Errorf("amazon blocked request (HTTP %d)", e.StatusCode).Error()
}

// New initializes a Scraper for a given URL.
func New(httpClient tls_client.HttpClient, url string) *Scraper {
	return &Scraper{
		httpClient: httpClient,
		url:        url,
	}
}

// PrimeSession fetches the Amazon homepage once to establish cookies (session-id)
// and bypass initial captchas before hitting the actual product URL.
func (s *Scraper) PrimeSession() error {
	req, err := client.BuildRequest("https://www.amazon.it")
	if err != nil {
		return err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == 503 || resp.StatusCode == 403 {
		return &BlockedError{StatusCode: resp.StatusCode}
	}

	// Drain the body to allow connection reuse.
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// Check performs a single scrape of the product URL.
func (s *Scraper) Check() (*Result, error) {
	req, err := client.BuildRequest(s.url)
	if err != nil {
		return nil, err
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 503 || resp.StatusCode == 403 {
		_, _ = io.Copy(io.Discard, resp.Body) // Drain body
		return nil, &BlockedError{StatusCode: resp.StatusCode}
	}

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body) // Drain body
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("parse html: %w", err)
	}

	return s.parse(doc.Selection)
}

func (s *Scraper) parse(doc *goquery.Selection) (*Result, error) {
	res := &Result{
		Available: false,
		Condition: "unknown",
	}

	// 1. Check if the product is completely out of stock
	outOfStockText := doc.Find("#availability .a-color-state").Text()
	if strings.Contains(outOfStockText, "Attualmente non disponibile") {
		return res, nil
	}
	
	hasUsedOnlyBuybox := doc.Find("#usedOnlyBuybox").Length() > 0
	if hasUsedOnlyBuybox {
		res.Condition = "used"
		priceStr := doc.Find("#corePrice_feature_div .a-price-whole").First().Text()
		if priceStr == "" {
			// Fallback: If it's a completely used-only page, the price is often locked in the 'other sellers' ingress
			priceStr = doc.Find("#aod-ingress-link .a-price-whole").First().Text()
		}
		priceStr = cleanPriceString(priceStr)
		if priceStr != "" {
			if p, err := strconv.ParseFloat(priceStr, 64); err == nil {
				res.UsedPrice = p
			}
		}
	}

	// 2. Check the main BuyBox (often used if "new" is hidden behind "See All Buying Options")
	isUsed := s.checkBuyBox(doc, res)

	// Try parsing prices from the Accordion design if present
	s.parseAccordionPrices(doc, res)

	// If accordion parse didn't find a new price but the buybox is new, grab the buybox price
	if res.Price == 0 && !isUsed && !hasUsedOnlyBuybox {
		priceStr := doc.Find("#corePrice_feature_div .a-price-whole").First().Text()
		priceStr = cleanPriceString(priceStr)
		if priceStr != "" {
			if p, err := strconv.ParseFloat(priceStr, 64); err == nil {
				res.Price = p
				res.Available = true
				res.Condition = "new"
			}
		}
	}

	// 3. One last fallback to ensure we catch a UsedPrice if the page defaults to it
	if res.UsedPrice == 0 {
		priceStr := doc.Find("#aod-ingress-link .a-price-whole").First().Text()
		priceStr = cleanPriceString(priceStr)
		if priceStr != "" {
			if p, err := strconv.ParseFloat(priceStr, 64); err == nil {
				res.UsedPrice = p
			}
		}
	}

	// Special check for "buy-now" button availability (sometimes items show a price but aren't actually shippable)
	hasNewAddToCart := doc.Find("#add-to-cart-button").Length() > 0
	if !hasNewAddToCart && !isUsed && !hasUsedOnlyBuybox {
		// Just to be safe, if we can't legitimately add it to cart without going to "other sellers", it's tough
		// But accordion often handles this. We will trust the price parsing for now.
	}

	// If we only found a used condition in the buybox, mark it as unavailable for our NEW sniping purposes
	if res.Condition == "used" {
		res.Available = false
	} else if res.Price > 0 {
		res.Available = true
		res.Condition = "new"
	}

	return res, nil
}

func (s *Scraper) checkBuyBox(doc *goquery.Selection, res *Result) bool {
	conditionText := doc.Find("#merchant-info").Text()
	buyboxStr := doc.Find("#buyBoxAccordion").Text()

	if strings.Contains(strings.ToLower(conditionText), "usat") || strings.Contains(strings.ToLower(buyboxStr), "usat") {
		res.Condition = "used"
		priceStr := doc.Find("#corePrice_feature_div .a-price-whole").First().Text()
		priceStr = cleanPriceString(priceStr)
		if priceStr != "" {
			if p, err := strconv.ParseFloat(priceStr, 64); err == nil {
				res.UsedPrice = p
			}
		}
		return true
	}
	return false
}

func (s *Scraper) parseAccordionPrices(doc *goquery.Selection, res *Result) {
	newRow := doc.Find("div[id^='newAccordionRow']")
	if newRow.Length() > 0 {
		priceStr := newRow.Find(".a-price-whole").First().Text()
		priceStr = cleanPriceString(priceStr)
		if priceStr != "" {
			if p, err := strconv.ParseFloat(priceStr, 64); err == nil {
				res.Price = p
				res.Condition = "new"
				res.Available = true
			}
		}
	}

	usedRow := doc.Find("div[id^='usedAccordionRow']")
	if usedRow.Length() > 0 {
		priceStr := usedRow.Find(".a-price-whole").First().Text()
		priceStr = cleanPriceString(priceStr)
		if priceStr != "" {
			if p, err := strconv.ParseFloat(priceStr, 64); err == nil {
				res.UsedPrice = p
			}
		}
	}
}

// cleanPriceString sanitizes the Amazon format (e.g., "1.049,00" -> "1049.00")
func cleanPriceString(s string) string {
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	s = strings.TrimSpace(s)
	return s
}
