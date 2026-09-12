package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"game-price-tracker/myimplementations/types"
	"log/slog"
	"regexp"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func SearchGame(collection *mongo.Collection, prefix string) {
	filter := bson.M{
		"title": bson.M{
			"$regex":   "^" + regexp.QuoteMeta(prefix),
			"$options": "i",
		},
	}

	ctx := context.TODO()

	slog.Info("Running SearchGame", "prefix", prefix)

	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		slog.Error("Failed to query game", "prefix", prefix, "error", err)
		return
	}
	defer func() {
		if err := cursor.Close(ctx); err != nil {
			slog.Error("Failed to close cursor", "error", err)
		}
	}()

	var results []types.Game
	if err = cursor.All(ctx, &results); err != nil {
		slog.Error("Failed to load searched games", "prefix", prefix, "error", err)
		return
	}

	slog.Info("Search completed", "prefix", prefix, "matches", len(results))
	if len(results) == 0 {
		slog.Info("No games matched search prefix", "prefix", prefix)
		fmt.Printf("No games found matching \"%s\"\n", prefix)
		return
	}

	marshalErrors := 0
	for _, game := range results {
		data, err := json.MarshalIndent(game, "", "  ")
		if err != nil {
			slog.Error("Failed to marshal game", "title", game.Title, "error", err)
			marshalErrors++
			continue
		}

		fmt.Println(string(data))
	}

	slog.Info("SearchGame completed", "prefix", prefix, "matches", len(results), "marshalErrors", marshalErrors)
}
