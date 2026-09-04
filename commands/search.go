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

	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		slog.Error("Failed to query game", "prefix", prefix, "error", err)
		return
	}

	var results []types.Game
	if err = cursor.All(ctx, &results); err != nil {
		slog.Error("Failed to load searched games", "prefix", prefix, "error", err)
		return
	}

	for _, game := range results {
		data, err := json.MarshalIndent(game, "", "  ")
		if err != nil {
			slog.Error("Failed to marshal game", "error", err)
			continue
		}

		fmt.Println(string(data))
	}
}
