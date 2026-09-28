package commands

import (
	"context"
	"fmt"
	"game-price-tracker/myimplementations/database"
	"game-price-tracker/myimplementations/types"
	"log/slog"
	"time"

	"github.com/gocolly/colly"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func UpsertGame(collection *mongo.Collection, appId int) (types.Game, error) {
	ctx := context.TODO()

	slog.Info("Running UpsertGame", "appId", appId)

	// Search for the game inside the Game document using its
	// app id.
	//
	// If it does not exists, insert into the document
	// and return the error if there was one.
	var existing types.Game
	err := collection.FindOne(
		ctx,
		bson.M{"_id": appId},
	).Decode(&existing)

	slog.Info("UpsertGame completed", "appId", appId)

	// Game does not exists
	if err == mongo.ErrNoDocuments {
		return insertGame(collection, appId)
	} else {
		return updateGame(collection, appId)
	}
}

func insertGame(collection *mongo.Collection, appId int) (types.Game, error) {
	searchURL := fmt.Sprintf("https://store.steampowered.com/app/%d", appId)

	slog.Info("Running InsertGame", "appId", appId)

	// Initialize a Collector instance to begin crawling
	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	// Setup a delay to avoid hitting the page too many times and triggering its rate limit
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*store.steampowered.com*",
		Delay:       10 * time.Second,
		RandomDelay: 5 * time.Second,
	})
	c.SetRequestTimeout(5 * time.Second)

	var game types.Game

	// Scan the game page
	c.OnHTML("div#tabletGrid", func(e *colly.HTMLElement) {

		first := e.DOM.Find("div.game_area_purchase_game_wrapper").First()

		discount, discErr := extractValue(first.Find(".discount_pct").Text())
		originalPrice, origErr := extractValue(first.Find(".discount_original_price").Text())
		discountPrice, finalErr := extractValue(first.Find(".discount_final_price").Text())
		isNoDiscount := e.DOM.Find("div.discount_block").HasClass("no_discount")

		// Steam omits the discount % and original price fields entirely when
		// there's no discount, so those two "errors" are expected in that case.
		// Only log them when they're NOT explained by a no-discount listing.
		if !isNoDiscount && (discErr != nil || origErr != nil) {
			slog.Warn("Partial price extraction",
				"app_id", appId,
				"discount_err", discErr,
				"original_price_err", origErr,
			)
		}

		if finalErr != nil {
			slog.Warn("Failed to extract discount price", "app_id", appId, "error", finalErr)
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
			AppId:         appId,
			Title:         e.ChildText("div#appHubAppName"),
			URL:           searchURL,
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

	// Visit the game page
	if err := c.Visit(searchURL); err != nil {
		slog.Error("Failed to visit", "url", searchURL, "error", err)
		return types.Game{}, fmt.Errorf("game with app_id %d not found", appId)
	}

	slog.Info("InsertGame completed", "app_id", appId)
	return game, nil
}

func updateGame(collection *mongo.Collection, appId int) (types.Game, error) {
	ctx := context.TODO()

	slog.Info("Running UpdateGame", "appId", appId)

	// Search for the game inside the Game document using its
	// app id.
	//
	// If it does not exists, insert into the document
	// and return the error if there was one.
	var existing types.Game
	err := collection.FindOne(
		ctx,
		bson.M{"_id": appId},
	).Decode(&existing)

	// Game does not exists
	if err == mongo.ErrNoDocuments {
		// Game does not exist yet, so we can't create it
		slog.Warn("Game not found for upsert", "appId", appId)
		return types.Game{}, fmt.Errorf("game with app_id %d not found", appId)
	}

	// If there is an error but insertion failed, return it
	if err != nil {
		slog.Error(
			"Failed to find game",
			"app_id", existing.AppId,
			"game", existing.Title,
			"error", err,
		)
		return types.Game{}, err
	}

	// Get the game's current price
	rec := existing.PriceSnapshot

	// True under any of the following condition
	// - The game does not have a pre-existing price history
	// - The last discount price does not match the current discount price
	// - The last original price does not match the current original price
	changed := len(existing.PriceHistory) > 1

	if !changed {
		// Get the last price record from its price history
		last := existing.PriceHistory[len(existing.PriceHistory)-1]

		changed = (last.DiscountPrice != rec.DiscountPrice) || (last.OriginalPrice != rec.OriginalPrice)

		if changed {
			slog.Info(
				"Updated game price data",
				"app_id", existing.AppId,
				"game", existing.Title,
				"old_price_data", last,
				"new_price_data", rec,
			)
		}
	} else {
		slog.Info(
			"No existing price history — recording first snapshot",
			"app_id", existing.AppId,
			"game", existing.Title,
			"price_data", rec,
		)
	}

	if !changed {
		slog.Info("No price change detected", "app_id", existing.AppId, "game", existing.Title)
	}

	// Initialize an update variable to update/set these
	update := bson.M{
		"$set": bson.M{
			"price_snapshot": rec,
			"title":          existing.Title,
			"url":            existing.URL,
		},
	}

	// If there was a change in the game's price history, add it to the
	// update map
	if changed {
		update["$push"] = bson.M{"price_history": rec}
	}

	// Update the game with the field using its app id and return the error
	_, err = collection.UpdateOne(ctx, bson.M{"_id": existing.AppId}, update)

	if err != nil {
		slog.Error(
			"Failed to update game",
			"app_id", existing.AppId,
			"game", existing.Title,
			"error", err,
		)

		return types.Game{}, err
	}

	slog.Info("UpdateGame completed", "app_id", existing.AppId, "game", existing.Title, "priceChanged", changed)

	return existing, nil
}
