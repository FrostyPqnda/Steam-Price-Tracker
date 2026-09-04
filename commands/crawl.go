package commands

import (
	"fmt"
	"game-price-tracker/myimplementations/database"
	"game-price-tracker/myimplementations/types"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/gocolly/colly"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// parsePaginationData crawls the steam store webpage's pagination
// section and extracts the total no. of pages.
func parsePaginationData(totalPages *int) {
	const searchURL = "https://store.steampowered.com/search?hwtype=0&supportedlang=english&hidef2p=1&ndl=1&page=1"

	slog.Debug("Staring pagination data extraction", "url", searchURL)

	// Insantiate a Collecter instance to begin scraping
	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	// Scrape this specific part of the HTML, extracting the total page count
	c.OnHTML("div.search_pagination > div.search_pagination_right", func(e *colly.HTMLElement) {
		e.ForEach("a[href]", func(_ int, el *colly.HTMLElement) {
			page, err := strconv.Atoi(strings.TrimSpace(el.Text))
			if err != nil {
				slog.Debug(
					"Skipping non-numeric pagination element",
					"text", el.Text,
				)
				return
			}

			if page > *totalPages {
				*totalPages = page
			}
		})
	})

	// Print the URL being visted
	c.OnRequest(func(r *colly.Request) {
		slog.Debug("Visiting URL", "url", r.URL.String())
	})

	// Print any error that happens during the scraping
	c.OnError(func(r *colly.Response, err error) {
		slog.Error(
			"Request failed",
			"url", r.Request.URL.String(),
			"status", r.StatusCode,
			"error", err,
		)
	})

	// Visit the Steam store page and log the error if it exists
	var err = c.Visit("https://store.steampowered.com/search?hwtype=0&supportedlang=english&hidef2p=1&ndl=1&page=1")
	if err != nil {
		slog.Error("failed to extract pagination data", "error", err)
		return
	}

	slog.Info("pagination data extracted", "total_pages", *totalPages)
}

// extractValue extracts the floating point value from a Steam game, specifically
// its price (original and discount) and the discount percent
func extractValue(s string) (float64, error) {
	// Remove all leading and trailing whitespaces
	s = strings.TrimSpace(s)

	// Extract and return the discount off percent
	if strings.HasSuffix(s, "%") {
		s = strings.TrimSuffix(s, "%")
		return strconv.ParseFloat(strings.TrimSpace(s), 64)
	}

	// Extract and return the price
	if strings.HasPrefix(s, "$") {
		s = strings.TrimPrefix(s, "$")
		return strconv.ParseFloat(strings.TrimSpace(s), 64)
	}

	// Return an error if value could not be extracted
	return 0, fmt.Errorf("unsupported value: %q", s)
}

// cleanURL cleans up the Steam game webpage link by removing
// the serial number.
//
// Input: https://store.steampowered.com/app/<app id>/<title>/?snr=<serial no.>
// Output: https://store.steampowered.com/app/<app id>/<title>/
func cleanURL(href string) string {
	if idx := strings.Index(href, "?"); idx != -1 {
		return href[:idx]
	}
	return href
}

// Crawl crawls the Steam store webpage and collects the game data into a MongoDB collection
func Crawl(collection *mongo.Collection) {
	slog.Debug("Startig price tracking extraction")

	// Extract the total no. of pages from the pagination data
	var totalPages int
	parsePaginationData(&totalPages)

	// Initialize a Collector instance to begin crawling
	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	// Setup a delay to avoid hitting the page too many times and triggering its rate limit
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*store.steampowered.com*",
		Delay:       90 * time.Second,
		RandomDelay: 30 * time.Second,
	})
	c.SetRequestTimeout(30 * time.Second)

	// Search the Steam store games list
	c.OnHTML("div#search_resultsRows > a", func(e *colly.HTMLElement) {
		// Extract the discount, original price, and discount price
		discount, _ := extractValue(e.ChildText("div.discount_pct"))

		original_price, _ := extractValue(e.ChildText("div.discount_original_price"))

		discount_price, _ := extractValue(e.ChildText("div.discount_final_price"))

		// Extract the game's app id and end the crawling if there was
		// an error
		id, err := strconv.Atoi(e.Attr("data-ds-appid"))
		if err != nil {
			slog.Error("Failed to extract app id", "error", err)
			return
		}

		// If the game does not have a discount, set the original price
		// and the discount price to be the same
		//
		// Context: Steam puts the final price in the discount field
		// even if there is no discount
		if e.DOM.Find("div.discount_block").HasClass("no_discount") {
			original_price = discount_price
			discount = 0
		}

		// Create a PriceRecord data at the most recent time checked
		price := types.PriceRecord{
			OriginalPrice: original_price,
			DiscountPrice: discount_price,
			Discount:      discount,
			CheckedAt:     time.Now(),
		}

		// Create a Game data storing
		// - id
		// - title
		// - url
		// - current price snapshot data
		// - price history
		game := types.Game{
			AppId:         id,
			Title:         e.ChildText("span.title"),
			URL:           cleanURL(e.Attr("href")),
			PriceSnapshot: price,
			PriceHistory:  []types.PriceRecord{price},
		}

		// Upsert the game into the Game document and throw an error if it fails
		if err := database.UpsertGame(collection, game); err != nil {
			slog.Error("Upsertion error",
				"app_id", game.AppId,
				"title", game.Title,
				"error", err,
			)
		}
	})

	// Print out the current page being visited
	c.OnRequest(func(r *colly.Request) {
		slog.Info("Visiting URL", "url", r.URL.String())
	})

	c.OnResponse(func(r *colly.Response) {
		if r.StatusCode == 429 {
			slog.Warn("Rate limited, backing off 5 minutes...")
			time.Sleep(5 * time.Minute)
		}
	})

	// Print out the error during scraping
	c.OnError(func(r *colly.Response, err error) {
		slog.Error("Request error", "error", err)
	})

	//q, _ := queue.New(1, &queue.InMemoryQueueStorage{MaxSize: 10000})

	// Visit all pages starting from 1 to n and log any errors that occurs
	// during visit
	for page := 1; page <= 3; page++ {
		url := fmt.Sprintf("https://store.steampowered.com/search?hwtype=0&supportedlang=english&hidef2p=1&ndl=1&page=%d", page)
		var err = c.Visit(url)
		if err != nil {
			slog.Error("Failed to visit", "error", err)
		}
	}
}
