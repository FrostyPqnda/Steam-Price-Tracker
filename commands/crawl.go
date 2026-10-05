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
	var err = c.Visit("https://store.steampowered.com/search?hwtype=0&category1=998&supportedlang=english&hidef2p=1&ndl=1&page=1")
	if err != nil {
		slog.Error("failed to extract pagination data", "error", err)
		return
	}

	slog.Info("pagination data extracted", "total_pages", *totalPages)
}

// extractValue extracts the floating point value from a Steam game, specifically
// its price (original and discount) and the discount percent
func extractPercent(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if !strings.HasSuffix(s, "%") {
		return 0, fmt.Errorf("not a percent: %q", s)
	}
	s = strings.TrimSpace(strings.TrimSuffix(s, "%"))
	return strconv.ParseInt(s, 10, 64)
}

func extractPrice(s string) (float64, error) {
	s = strings.TrimSpace(s)
	if !strings.HasPrefix(s, "$") {
		return 0, fmt.Errorf("not a price: %q", s)
	}
	s = strings.TrimSpace(strings.TrimPrefix(s, "$"))
	return strconv.ParseFloat(s, 64)
}

// cleanURL cleans up the Steam game webpage link by removing
// the serial number.
//
// Input: https://store.steampowered.com/app/<app id>/<title>/?snr=<serial no.>
// Output: https://store.steampowered.com/app/<app id>
func cleanURL(href string) string {
	if before, _, found := strings.Cut(href, "/?"); found {
		if idx := strings.LastIndex(before, "/"); idx != -1 {
			return before[:idx]
		}
		return before
	}

	return href
}

// Crawl crawls the Steam store webpage and collects the game data into a MongoDB collection
func Crawl(gameCollection, stateCollection *mongo.Collection) {
	slog.Info("Starting price tracking extraction")

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

	gamesFound := 0
	upsertErrors := 0
	extractionWarnings := 0
	var pagesAttempted, pagesFailed int

	// Search the Steam store games list
	c.OnHTML("div#search_resultsRows > a", func(e *colly.HTMLElement) {
		// Extract the game's app id and end the crawling if there was
		// an error
		id, err := strconv.Atoi(e.Attr("data-ds-appid"))
		if err != nil {
			slog.Error("Failed to extract app id", "error", err)
			return
		}

		// Extract the discount, original price, and discount price
		discount, discErr := extractPercent(e.ChildText("div.discount_pct"))
		originalPrice, origErr := extractPrice(e.ChildText("div.discount_original_price"))
		discountPrice, finalErr := extractPrice(e.ChildText("div.discount_final_price"))
		isNoDiscount := e.DOM.Find("div.discount_block").HasClass("no_discount")

		// Steam omits the discount % and original price fields entirely when
		// there's no discount, so those two "errors" are expected in that case.
		// Only log them when they're NOT explained by a no-discount listing.
		if !isNoDiscount && (discErr != nil || origErr != nil) {
			slog.Warn("Partial price extraction",
				"app_id", id,
				"discount_err", discErr,
				"original_price_err", origErr,
			)
			extractionWarnings++
		}

		if finalErr != nil {
			slog.Warn("Failed to extract discount price", "app_id", id, "error", finalErr)
			extractionWarnings++
		}

		// If the game does not have a discount, set the original price
		// and the discount price to be the same
		//
		// Context: Steam puts the final price in the discount field
		// even if there is no discount
		if isNoDiscount {
			originalPrice = discountPrice
			discount = 0
		}

		// Create a PriceRecord data at the most recent time checked
		price := types.PriceRecord{
			OriginalPrice: originalPrice,
			DiscountPrice: discountPrice,
			Discount:      discount * -1,
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
		if err := database.UpsertGame(gameCollection, game); err != nil {
			slog.Error("Upsertion error",
				"app_id", game.AppId,
				"title", game.Title,
				"error", err,
			)
			upsertErrors++
		}
	})

	// Print out the current page being visited
	c.OnRequest(func(r *colly.Request) {
		slog.Info("Visiting URL", "url", r.URL.String())
	})

	// Sleep to prevent rate limiting
	c.OnResponse(func(r *colly.Response) {
		if r.StatusCode == 429 {
			slog.Warn("Rate limited, backing off 5 minutes...")
			time.Sleep(5 * time.Minute)
		}
	})

	// Print out the error during scraping
	c.OnError(func(r *colly.Response, err error) {
		slog.Error("Request error", "url", r.Request.URL.String(), "status", r.StatusCode, "error", err)
	})

	//startPage, err := .//LoadState(stateCollection)
	startPage, lastPage, err := database.LoadMetadataState(stateCollection)
	if err != nil {
		slog.Error("Failed to load metadata", "error", err)
		return
	}

	if startPage == lastPage {
		var totalPages int
		parsePaginationData(&totalPages)

		if totalPages == 0 {
			slog.Error("Pagination extraction returned 0 pages, aborting crawl")
			return
		}

		startPage = 1
		lastPage = totalPages

		if err := database.SaveCrawlRange(stateCollection, startPage, lastPage); err != nil {
			slog.Error("Failed to save initial crawl range, aborting", "error", err)
			return
		}
	} else {
		slog.Info("Resuming crawl, skipping pagination re-check", "start_page", startPage, "last_page", lastPage)
	}

	// Visit all pages starting from 1 to n and log any errors that occurs
	// during visit
	for page := startPage; page <= lastPage; page++ {
		pagesAttempted++
		url := fmt.Sprintf("https://store.steampowered.com/search?hwtype=0&category1=998&supportedlang=english&hidef2p=1&ndl=1&sort_by=Released_ASC&page=%d", page)
		if err := c.Visit(url); err != nil {
			slog.Error("Failed to visit", "url", url, "page", page, "error", err)
			pagesFailed++
			continue
		}

		if err := database.SaveCurrentCrawl(stateCollection, page); err != nil {
			slog.Error("Failed to save crawl checkpoint", "page", page, "error", err)
		}
	}

	slog.Info("Crawl completed",
		"pagesAttempted", pagesAttempted,
		"pagesFailed", pagesFailed,
		"totalPagesAvailable", lastPage,
		"gamesFound", gamesFound,
		"upsertErrors", upsertErrors,
		"extractionWarnings", extractionWarnings,
	)
}
