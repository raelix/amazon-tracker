// Package notifier sends Telegram alerts via the Bot API.
package notifier

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	tls_client "github.com/bogdanfinn/tls-client"

	"amazon-tracker/internal/client"
)

// Notifier sends messages via the Telegram Bot API.
type Notifier struct {
	httpClient tls_client.HttpClient
	token      string
	chatID     string
}

// New creates a Notifier with the given Telegram token and chat ID.
// It reuses the same TLS client to avoid fingerprint leaks on the
// Telegram API calls.
func New(c tls_client.HttpClient, token, chatID string) *Notifier {
	return &Notifier{
		httpClient: c,
		token:      token,
		chatID:     chatID,
	}
}

// Send sends a Telegram notification about product availability.
func (n *Notifier) Send(price float64, productURL string) error {
	text := fmt.Sprintf(
		"*PIXEL 10 PRO DISPONIBILE!*\n\n"+
			"Prezzo: *%.2f EUR*\n"+
			"[Acquista su Amazon](%s)",
		price, productURL,
	)

	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", n.token)

	form := url.Values{}
	form.Set("chat_id", n.chatID)
	form.Set("text", text)
	form.Set("parse_mode", "Markdown")
	form.Set("disable_web_page_preview", "false")

	req, err := client.BuildRequest(apiURL)
	if err != nil {
		return fmt.Errorf("notify: build request: %w", err)
	}

	// Override to POST with form data.
	req.Method = "POST"
	req.Body = io.NopCloser(strings.NewReader(form.Encode()))
	req.Header.Set("content-type", "application/x-www-form-urlencoded")

	resp, err := n.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("notify: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("notify: Telegram API error %d: %s", resp.StatusCode, string(body))
	}

	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}
