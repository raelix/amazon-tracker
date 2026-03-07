// Deep DOM inspection of the #corePrice_feature_div and buybox accordion
// on the new product page (B0FHL38J26) to understand price container hierarchy.
package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/PuerkitoBio/goquery"

	"amazon-tracker/internal/client"
)

func main() {
	log.SetFlags(log.Ldate | log.Ltime)

	httpClient, err := client.New()
	if err != nil {
		log.Fatalf("TLS client error: %v", err)
	}

	// Prime
	log.Println("Priming...")
	req, _ := client.BuildRequest("https://www.amazon.it")
	resp, _ := httpClient.Do(req)
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// Fetch
	log.Println("Fetching B0FHL38J26...")
	req, _ = client.BuildRequest("https://www.amazon.it/dp/B0FHL38J26")
	resp, err = httpClient.Do(req)
	if err != nil {
		log.Fatalf("Fetch failed: %v", err)
	}
	defer resp.Body.Close()

	doc, _ := goquery.NewDocumentFromReader(resp.Body)

	// Save full HTML
	fullHTML, _ := doc.Html()
	os.WriteFile("/tmp/amazon_new_product.html", []byte(fullHTML), 0644)

	// ── #corePrice_feature_div internal structure ────────────────────────
	fmt.Println("\n=== #corePrice_feature_div structure ===")
	corePrice := doc.Find("#corePrice_feature_div")
	if corePrice.Length() > 0 {
		// Show all children divs with their IDs and classes
		corePrice.Find("div, span").Each(func(i int, s *goquery.Selection) {
			id, _ := s.Attr("id")
			class, _ := s.Attr("class")
			if id != "" || (class != "" && strings.Contains(class, "price")) {
				text := strings.TrimSpace(s.Contents().Not("*").Text())
				if len(text) > 100 {
					text = text[:100]
				}
				fmt.Printf("  id='%s' class='%s' text='%s'\n", id, class, text)
			}
		})

		// Find all .a-price elements and their context
		fmt.Println("\n  .a-price elements inside #corePrice:")
		corePrice.Find(".a-price").Each(func(i int, s *goquery.Selection) {
			offscreen := strings.TrimSpace(s.Find(".a-offscreen").Text())
			whole := strings.TrimSpace(s.Find(".a-price-whole").Text())
			class, _ := s.Attr("class")
			parentClass, _ := s.Parent().Attr("class")
			parentID, _ := s.Parent().Attr("id")
			fmt.Printf("    [%d] offscreen='%s' whole='%s'\n", i, offscreen, whole)
			fmt.Printf("        class='%s'\n", class)
			fmt.Printf("        parent: id='%s' class='%s'\n", parentID, parentClass)
		})
	}

	// ── Accordion structure ─────────────────────────────────────────────
	fmt.Println("\n=== Accordion / buybox sections ===")
	doc.Find("#buyBoxAccordion, .dp-accordion-row, .rbbHeader").Each(func(i int, s *goquery.Selection) {
		id, _ := s.Attr("id")
		class, _ := s.Attr("class")
		// Look for condition text
		text := strings.TrimSpace(s.Text())
		if len(text) > 200 {
			text = text[:200]
		}
		fmt.Printf("[%d] id='%s' class='%s'\n", i, id, class)
		if strings.Contains(strings.ToLower(text), "nuovo") || strings.Contains(strings.ToLower(text), "usato") {
			// Extract the relevant part
			for _, keyword := range []string{"Compra nuovo", "Compra usato", "Nuovo", "Usato"} {
				idx := strings.Index(text, keyword)
				if idx >= 0 {
					end := idx + len(keyword) + 50
					if end > len(text) {
						end = len(text)
					}
					fmt.Printf("    Found: '%s'\n", text[idx:end])
				}
			}
		}
	})

	// ── Check for accordion-based new/used sections ─────────────────────
	fmt.Println("\n=== buyBoxAccordion ===")
	bba := doc.Find("#buyBoxAccordion")
	if bba.Length() > 0 {
		fmt.Println("Found buyBoxAccordion!")
		bba.Find(".a-accordion-row").Each(func(i int, s *goquery.Selection) {
			header := strings.TrimSpace(s.Find(".a-accordion-row-a11y").Text())
			price := strings.TrimSpace(s.Find(".a-price-whole").First().Text())
			fmt.Printf("  Row %d: header='%s' price='%s'\n", i, header, price)
		})
	} else {
		fmt.Println("No #buyBoxAccordion found")
	}

	// ── New section vs used section ─────────────────────────────────────
	fmt.Println("\n=== #newBuySection / #usedBuySection ===")
	newBuy := doc.Find("#newBuySection")
	usedBuy := doc.Find("#usedBuySection")
	fmt.Printf("  #newBuySection exists: %v\n", newBuy.Length() > 0)
	fmt.Printf("  #usedBuySection exists: %v\n", usedBuy.Length() > 0)

	if newBuy.Length() > 0 {
		newPrice := strings.TrimSpace(newBuy.Find(".a-price-whole").First().Text())
		fmt.Printf("  #newBuySection price: %s\n", newPrice)
	}
	if usedBuy.Length() > 0 {
		usedPrice := strings.TrimSpace(usedBuy.Find(".a-price-whole").First().Text())
		fmt.Printf("  #usedBuySection price: %s\n", usedPrice)
	}

	// ── Specific accordion sections ─────────────────────────────────────
	fmt.Println("\n=== Accordion buying options ===")
	doc.Find("[data-buying-option-index]").Each(func(i int, s *goquery.Selection) {
		idx, _ := s.Attr("data-buying-option-index")
		id, _ := s.Attr("id")
		featureName, _ := s.Attr("data-feature-name")
		price := strings.TrimSpace(s.Find(".a-price-whole").First().Text())
		fmt.Printf("  [index=%s] id='%s' feature='%s' price='%s'\n", idx, id, featureName, price)
	})

	log.Println("Done. Full HTML saved to /tmp/amazon_new_product.html")
}
