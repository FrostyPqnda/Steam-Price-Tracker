package commands

import (
	"context"
	"errors"
	"fmt"
	"game-price-tracker/myimplementations/types"
	"log/slog"
	"time"

	"github.com/gocolly/colly"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// UpsertGame makes sure the game with the given appId is stored in collection,
// and returns the resulting game.
//
// If no document with _id equal to appId exists, the game is inserted with
// insertGame. In every other case, including when the lookup itself fails,
// the existing game is refreshed with updateGame. Any error from insertGame or
// updateGame is returned to the caller.
//
// The collection must contain documents decodable into types.Game, keyed by
// _id. The lookup has no timeout.
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

// insertGame scrapes the Steam store page for the game with the given appId
// and inserts it into collection. The current price becomes both the game's
// PriceSnapshot and the first entry of its PriceHistory. The inserted game is
// returned.
//
// The price comes from the first purchase option on the page. Steam shows only
// the final price when a game isn't on sale, so in that case OriginalPrice is
// set equal to DiscountPrice and Discount is 0. Discount is stored as a
// negative percentage (-50 means 50% off).
//
// It returns an error if the page can't be fetched, if the page has no
// recognizable game listing (for example an unknown app ID, or a region-locked
// or age-gated page), or if the insert fails. Failure to parse individual price
// fields is only logged and leaves those fields at zero.
//
// The page request times out after 5 seconds and the insert after 10 seconds.
func insertGame(collection *mongo.Collection, appId int) (types.Game, error) {
	slog.Info("Running InsertGame", "appId", appId)

	game, err := scrapeGame(appId)
	if err != nil {
		return types.Game{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if _, err := collection.InsertOne(ctx, game); err != nil {
		slog.Error("Failed to insert game", "app_id", appId, "title", game.Title, "error", err)
		return types.Game{}, err
	}

	slog.Info("InsertGame completed", "app_id", appId)
	return game, nil
}

// updateGame re-scrapes the Steam store page for an already-tracked game and
// records the fresh price. It returns the updated game.
//
// PriceSnapshot is always replaced with the new check. The new record is also
// appended to PriceHistory if the history is empty or if the original or
// discounted price differs from the most recent history entry, so history
// holds price changes rather than every check. The title is refreshed when the
// page provides one.
//
// It returns an error if no game with appId is stored, if scraping fails, or
// if the database lookup or update fails. Each database operation is limited
// to 10 seconds; the scrape has its own 5-second request timeout.
func updateGame(collection *mongo.Collection, appId int) (types.Game, error) {
	slog.Info("Running updateGame", "appId", appId)

	// Look up the stored game first: it fails fast if the game isn't tracked,
	// before spending time on a slow, rate-limited scrape.
	lookupCtx, cancelLookup := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelLookup()

	var existing types.Game
	err := collection.FindOne(lookupCtx, bson.M{"_id": appId}).Decode(&existing)
	if err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			slog.Warn("Game not found for update", "appId", appId)
			return types.Game{}, fmt.Errorf("game with app_id %d not found", appId)
		}
		slog.Error("Failed to find game", "appId", appId, "error", err)
		return types.Game{}, err
	}

	// Fetch the current price from Steam.
	scraped, err := scrapeGame(appId)
	if err != nil {
		slog.Error("Failed to scrape game", "appId", appId, "error", err)
		return types.Game{}, err
	}
	rec := scraped.PriceSnapshot

	// Only record a history entry when something changed.
	changed := len(existing.PriceHistory) == 0
	if !changed {
		last := existing.PriceHistory[len(existing.PriceHistory)-1]
		changed = last.DiscountPrice != rec.DiscountPrice ||
			last.OriginalPrice != rec.OriginalPrice
		if changed {
			slog.Info("Price changed", "app_id", appId, "game", existing.Title,
				"old_price_data", last, "new_price_data", rec)
		} else {
			slog.Info("No price change detected", "app_id", appId, "game", existing.Title)
		}
	} else {
		slog.Info("No existing price history, recording first snapshot",
			"app_id", appId, "game", existing.Title, "price_data", rec)
	}

	// Keep the stored title if the scrape came back without one.
	title := existing.Title
	if scraped.Title != "" {
		title = scraped.Title
	}

	set := bson.M{"price_snapshot": rec, "title": title}
	update := bson.M{"$set": set}
	if changed {
		update["$push"] = bson.M{"price_history": rec}
	}

	updateCtx, cancelUpdate := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelUpdate()

	if _, err := collection.UpdateOne(updateCtx, bson.M{"_id": appId}, update); err != nil {
		slog.Error("Failed to update game", "app_id", appId, "game", title, "error", err)
		return types.Game{}, err
	}

	// Mirror the database change in the returned value.
	existing.Title = title
	existing.PriceSnapshot = rec
	if changed {
		existing.PriceHistory = append(existing.PriceHistory, rec)
	}

	slog.Info("updateGame completed", "app_id", appId, "game", title, "priceChanged", changed)
	return existing, nil
}

// scrapeGame fetches the Steam store page for appId and returns a types.Game
// built from it, with the current price as both PriceSnapshot and the only
// PriceHistory entry. It does not touch the database.
//
// It returns an error if the page can't be fetched or has no recognizable
// game listing. Failure to parse individual price fields is only logged and
// leaves those fields at zero. The request times out after 5 seconds.
func scrapeGame(appId int) (types.Game, error) {
	searchURL := fmt.Sprintf("https://store.steampowered.com/app/%d", appId)

	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)
	c.Limit(&colly.LimitRule{
		DomainGlob:  "*store.steampowered.com*",
		Delay:       10 * time.Second,
		RandomDelay: 5 * time.Second,
	})
	c.SetRequestTimeout(5 * time.Second)

	var game types.Game
	found := false

	c.OnHTML("div#tabletGrid", func(e *colly.HTMLElement) {
		game = types.Game{ /* appId, Title, URL, PriceSnapshot, PriceHistory */ }
		found = true
	})
	c.OnRequest(func(r *colly.Request) {
		slog.Info("Visiting URL", "url", r.URL.String())
	})
	c.OnError(func(r *colly.Response, err error) {
		slog.Error("Request error", "url", r.Request.URL.String(), "status", r.StatusCode, "error", err)
		if r.StatusCode == 429 {
			slog.Warn("Rate limited, backing off 5 minutes...")
			time.Sleep(5 * time.Minute)
		}
	})

	if err := c.Visit(searchURL); err != nil {
		return types.Game{}, fmt.Errorf("fetching app_id %d: %w", appId, err)
	}
	if !found {
		return types.Game{}, fmt.Errorf("game with app_id %d not found", appId)
	}
	return game, nil
}
