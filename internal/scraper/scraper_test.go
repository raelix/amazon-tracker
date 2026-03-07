package scraper_test

import (
	"log"
	"testing"

	"amazon-tracker/internal/client"
	"amazon-tracker/internal/scraper"
)

// TestIntegration_AmazonScraper tests the scraper against live Amazon.it pages.
// We use two known ASINs:
//  - B0FHL3385S: Used-only offers (should return Available=false, Condition=used)
//  - B0FHL38J26: New product in stock (should return Available=true, Condition=new, Price~999)
//
// Note: This relies on live Amazon data. If prices or availability change significantly,
// the assertions might need updating. It also depends on Amazon not immediately blocking
// the request.
func TestIntegration_AmazonScraper(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode.")
	}

	httpClient, err := client.New()
	if err != nil {
		t.Fatalf("Failed to initialize TLS client: %v", err)
	}

	// Prime session to mimic real browser flow
	primer := scraper.New(httpClient, "https://www.amazon.it")
	if err := primer.PrimeSession(); err != nil {
		t.Logf("Warning: Session prime failed (continuing anyway): %v", err)
	}

	t.Run("Used-Only Product (B0FHL3385S)", func(t *testing.T) {
		t.Parallel()
		sc := scraper.New(httpClient, "https://www.amazon.it/dp/B0FHL3385S")
		res, err := sc.Check()
		
		if err != nil {
			t.Fatalf("Scraper.Check() returned error: %v", err)
		}

		if res.Condition != "used" {
			t.Errorf("Expected Condition='used', got '%s'", res.Condition)
		}
		if res.Available {
			t.Errorf("Expected Available=false, got true")
		}
		if res.Price != 0 {
			t.Errorf("Expected Price=0 for used-only, got %.2f", res.Price)
		}
	})

	t.Run("New Product (B0FHL38J26)", func(t *testing.T) {
		t.Parallel()
		sc := scraper.New(httpClient, "https://www.amazon.it/dp/B0FHL38J26")
		res, err := sc.Check()

		if err != nil {
			t.Fatalf("Scraper.Check() returned error: %v", err)
		}

		if res.Condition != "new" {
			t.Errorf("Expected Condition='new', got '%s'", res.Condition)
		}
		if !res.Available {
			t.Errorf("Expected Available=true, got false")
		}

		log.Printf("Live new price found: %.2f EUR", res.Price)
		// Assuming price is around 999 EUR
		if res.Price < 800 || res.Price > 1200 {
			t.Errorf("Expected price roughly between 800 and 1200, got %.2f", res.Price)
		}
	})
}
