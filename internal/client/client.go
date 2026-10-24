// Package client provides a stealth HTTP client that impersonates Chrome 124
// at the TLS, HTTP/2, and header layers to avoid Amazon bot detection.
package client

import (
	"io"
	"math/rand"
	"strings"

	http "github.com/bogdanfinn/fhttp"
	"github.com/bogdanfinn/fhttp/cookiejar"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
)

// userAgents is a pool of realistic Chrome User-Agent strings.
// Rotated per request to avoid fingerprint locking.
var userAgents = []string{
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
	"Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36",
}

// RandomUA returns a randomly selected user agent from the pool.
func RandomUA() string {
	return userAgents[rand.Intn(len(userAgents))]
}

// New creates a new tls-client HttpClient configured to impersonate Chrome 124.
// The profile automatically sets:
//   - TLS ClientHello (JA3 fingerprint) matching Chrome 124
//   - HTTP/2 SETTINGS frame values (header table size, initial window, etc.)
//   - HTTP/2 pseudo-header order (:method, :authority, :scheme, :path)
//   - Connection flow matching Chrome 124
func New() (tls_client.HttpClient, error) {
	// Use the RFC 6265 jar rather than tls_client.NewCookieJar: the latter keys
	// cookies by name only and ignores Expires, so Amazon's expired
	// ".www.amazon.it" deletion cookies overwrite the live ".amazon.it" session
	// (session-id becomes "-") and no session state, like the delivery
	// location, survives between requests.
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}

	options := []tls_client.HttpClientOption{
		tls_client.WithTimeoutSeconds(30),
		tls_client.WithClientProfile(profiles.Chrome_124),
		tls_client.WithRandomTLSExtensionOrder(),
		tls_client.WithCookieJar(jar),
		tls_client.WithNotFollowRedirects(),
	}

	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(), options...)
	if err != nil {
		return nil, err
	}

	// Allow following redirects for Amazon (302 is common).
	client.SetFollowRedirect(true)

	return client, nil
}

// BuildRequest creates an *http.Request with Chrome 124's exact header set
// in the precise wire order that a real browser sends them.
// This covers:
//   - Client Hints (Sec-CH-UA, Sec-CH-UA-Mobile, Sec-CH-UA-Platform)
//   - Fetch Metadata (Sec-Fetch-Dest, Sec-Fetch-Mode, Sec-Fetch-Site, Sec-Fetch-User)
//   - Standard navigation headers (Accept, Accept-Encoding, Accept-Language, Referer)
//   - HTTP/2 pseudo-header order via PHeaderOrderKey
func BuildRequest(targetURL string) (*http.Request, error) {
	req, err := http.NewRequest(http.MethodGet, targetURL, nil)
	if err != nil {
		return nil, err
	}

	ua := RandomUA()

	// Set headers — note: the order they appear in Header-Order: controls
	// the wire order for HTTP/2, NOT the order they are set here.
	req.Header = http.Header{
		"sec-ch-ua":                 {`"Chromium";v="124", "Google Chrome";v="124", "Not-A.Brand";v="99"`},
		"sec-ch-ua-mobile":          {"?0"},
		"sec-ch-ua-platform":        {`"Windows"`},
		"upgrade-insecure-requests": {"1"},
		"user-agent":                {ua},
		"accept":                    {"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7"},
		"sec-fetch-site":            {"none"},
		"sec-fetch-mode":            {"navigate"},
		"sec-fetch-user":            {"?1"},
		"sec-fetch-dest":            {"document"},
		"accept-encoding":           {"gzip, deflate, br, zstd"},
		"accept-language":           {"it-IT,it;q=0.9,en-US;q=0.8,en;q=0.7"},
		"referer":                   {"https://www.google.it/"},
		"priority":                  {"u=0, i"},

		// Magic keys that control HTTP/2 header wire order.
		// This MUST match Chrome's exact ordering.
		http.HeaderOrderKey: {
			"sec-ch-ua",
			"sec-ch-ua-mobile",
			"sec-ch-ua-platform",
			"upgrade-insecure-requests",
			"user-agent",
			"accept",
			"sec-fetch-site",
			"sec-fetch-mode",
			"sec-fetch-user",
			"sec-fetch-dest",
			"accept-encoding",
			"accept-language",
			"referer",
			"priority",
		},

		// HTTP/2 pseudo-header order (Chrome sends them in this exact sequence).
		http.PHeaderOrderKey: {
			":method",
			":authority",
			":scheme",
			":path",
		},
	}

	return req, nil
}

// BuildXHR creates a same-origin XMLHttpRequest-style request, as Amazon's
// page scripts send to their AJAX endpoints. token is sent as the
// anti-csrftoken-a2z header; a non-empty body is sent as JSON.
func BuildXHR(method, targetURL, referer, token, body string) (*http.Request, error) {
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, targetURL, r)
	if err != nil {
		return nil, err
	}

	req.Header = http.Header{
		"sec-ch-ua":          {`"Chromium";v="124", "Google Chrome";v="124", "Not-A.Brand";v="99"`},
		"sec-ch-ua-mobile":   {"?0"},
		"sec-ch-ua-platform": {`"Windows"`},
		"anti-csrftoken-a2z": {token},
		"x-requested-with":   {"XMLHttpRequest"},
		"user-agent":         {RandomUA()},
		"accept":             {"text/html,*/*"},
		"origin":             {"https://www.amazon.it"},
		"sec-fetch-site":     {"same-origin"},
		"sec-fetch-mode":     {"cors"},
		"sec-fetch-dest":     {"empty"},
		"referer":            {referer},
		"accept-encoding":    {"gzip, deflate, br, zstd"},
		"accept-language":    {"it-IT,it;q=0.9,en-US;q=0.8,en;q=0.7"},
		http.PHeaderOrderKey: {":method", ":authority", ":scheme", ":path"},
	}
	if body != "" {
		req.Header.Set("content-type", "application/json")
	}
	return req, nil
}
