// Package scraper handles fetching and parsing Amazon product pages.
package scraper

import (
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
	fhttp "github.com/bogdanfinn/fhttp"
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

// LocationError is returned when Amazon is not delivering to an Italian
// postcode, so availability and prices on the page are not the Italian ones
// (e.g. an IP geolocated abroad shows every item as "Non disponibile").
type LocationError struct {
	Location string
}

func (e *LocationError) Error() string {
	return fmt.Sprintf("delivery location is %q, not an Italian postcode", e.Location)
}

var (
	italianZip      = regexp.MustCompile(`\b\d{5}\b`)
	modalTokenRe    = regexp.MustCompile(`"anti-csrftoken-a2z":"([^"]+)"`)
	glowCSRFTokenRe = regexp.MustCompile(`CSRF_TOKEN\s*:\s*"([^"]+)"`)
)

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

// SetDeliveryZip sets the session's delivery location to an Italian postcode,
// replaying the "Seleziona la tua posizione" flow: the product page carries a
// token for the location modal, the modal carries the CSRF token for the
// address-change call. The scraper URL must be an amazon.it product page.
func (s *Scraper) SetDeliveryZip(zip string) error {
	page, err := s.fetch(s.url)
	if err != nil {
		return err
	}
	m := modalTokenRe.FindStringSubmatch(html.UnescapeString(page))
	if m == nil {
		return fmt.Errorf("location modal token not found")
	}

	modalURL := "https://www.amazon.it/portal-migration/hz/glow/get-rendered-address-selections?deviceType=desktop&pageType=Detail&storeContext=NoStoreName&actionSource=desktop-modal"
	modal, err := s.xhr(http.MethodGet, modalURL, m[1], "")
	if err != nil {
		return fmt.Errorf("location modal: %w", err)
	}
	m = glowCSRFTokenRe.FindStringSubmatch(modal)
	if m == nil {
		return fmt.Errorf("location modal has no CSRF token (no postcode input offered)")
	}

	body := fmt.Sprintf(`{"locationType":"LOCATION_INPUT","zipCode":%q,"deviceType":"web","storeContext":"generic","pageType":"Detail","actionSource":"glow"}`, zip)
	out, err := s.xhr(http.MethodPost, "https://www.amazon.it/portal-migration/hz/glow/address-change?actionSource=glow", m[1], body)
	if err != nil {
		return fmt.Errorf("address change: %w", err)
	}
	var res struct {
		Successful int `json:"successful"`
		Address    struct {
			CountryCode string `json:"countryCode"`
		} `json:"address"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil || res.Successful != 1 || res.Address.CountryCode != "IT" {
		return fmt.Errorf("address change rejected: %.200q", out)
	}
	return nil
}

func (s *Scraper) fetch(url string) (string, error) {
	req, err := client.BuildRequest(url)
	if err != nil {
		return "", err
	}
	return s.do(req)
}

func (s *Scraper) xhr(method, url, token, body string) (string, error) {
	req, err := client.BuildXHR(method, url, s.url, token, body)
	if err != nil {
		return "", err
	}
	return s.do(req)
}

func (s *Scraper) do(req *fhttp.Request) (string, error) {
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("http do: %w", err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode == 503 || resp.StatusCode == 403 {
		return "", &BlockedError{StatusCode: resp.StatusCode}
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}
	return string(b), nil
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

	// 0. Prices and availability are only meaningful for an Italian address.
	if loc := strings.TrimSpace(doc.Find("#glow-ingress-line2").Text()); loc != "" && !italianZip.MatchString(loc) {
		return nil, &LocationError{Location: loc}
	}

	// 1. Check if the product is completely out of stock. "Attualmente non
	// disponibile" items that can still be ordered (shipped when back in
	// stock) keep an add-to-cart button and a price, so parse those.
	outOfStockText := doc.Find("#availability .a-color-state").Text()
	if strings.Contains(outOfStockText, "Attualmente non disponibile") && doc.Find("#add-to-cart-button").Length() == 0 {
		return res, nil
	}

	hasUsedOnlyBuybox := doc.Find("#usedOnlyBuybox").Length() > 0
	if hasUsedOnlyBuybox {
		res.Condition = "used"
		p, ok := parsePrice(doc.Find("#corePrice_feature_div"))
		if !ok {
			// Fallback: If it's a completely used-only page, the price is often locked in the 'other sellers' ingress
			p, ok = parsePrice(doc.Find("#aod-ingress-link"))
		}
		if ok {
			res.UsedPrice = p
		}
	}

	// 2. Check the main BuyBox (often used if "new" is hidden behind "See All Buying Options")
	isUsed := s.checkBuyBox(doc, res)

	// Try parsing prices from the Accordion design if present
	s.parseAccordionPrices(doc, res)

	// If accordion parse didn't find a new price but the buybox is new, grab the buybox price
	if res.Price == 0 && !isUsed && !hasUsedOnlyBuybox {
		if p, ok := parsePrice(doc.Find("#corePrice_feature_div")); ok {
			res.Price = p
			res.Available = true
			res.Condition = "new"
		}
	}

	// 3. One last fallback to ensure we catch a UsedPrice if the page defaults to it
	if res.UsedPrice == 0 {
		if p, ok := parsePrice(doc.Find("#aod-ingress-link")); ok {
			res.UsedPrice = p
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
		if p, ok := parsePrice(doc.Find("#corePrice_feature_div")); ok {
			res.UsedPrice = p
		}
		return true
	}
	return false
}

func (s *Scraper) parseAccordionPrices(doc *goquery.Selection, res *Result) {
	newRow := doc.Find("div[id^='newAccordionRow']")
	if newRow.Length() > 0 {
		if p, ok := parsePrice(newRow); ok {
			res.Price = p
			res.Condition = "new"
			res.Available = true
		}
	}

	usedRow := doc.Find("div[id^='usedAccordionRow']")
	if usedRow.Length() > 0 {
		if p, ok := parsePrice(usedRow); ok {
			res.UsedPrice = p
		}
	}
}

// parsePrice reads the first Amazon price inside sel, combining the whole
// part (e.g. "1.049,") with the cents in .a-price-fraction (e.g. "99").
func parsePrice(sel *goquery.Selection) (float64, bool) {
	whole := sel.Find(".a-price-whole").First()
	priceStr := strings.TrimSuffix(cleanPriceString(whole.Text()), ".")
	if priceStr == "" {
		return 0, false
	}
	if fraction := strings.TrimSpace(whole.NextAllFiltered(".a-price-fraction").First().Text()); fraction != "" {
		priceStr += "." + fraction
	}
	p, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		return 0, false
	}
	return p, true
}

// cleanPriceString sanitizes the Amazon format (e.g., "1.049,00" -> "1049.00")
func cleanPriceString(s string) string {
	s = strings.ReplaceAll(s, ".", "")
	s = strings.ReplaceAll(s, ",", ".")
	s = strings.TrimSpace(s)
	return s
}
