package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"game-price-tracker/myimplementations/types"
	"log/slog"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func DisplayDeals(collection *mongo.Collection) {
	ctx := context.TODO()
	filter := bson.M{"price_snapshot.discount": bson.M{"$ne": 0}}

	slog.Info("Running DisplayDeals")

	cursor, err := collection.Find(ctx, filter, nil)
	if err != nil {
		slog.Error("Failed to query deals", "error", err)
		return
	}
	defer cursor.Close(ctx)

	var results []types.Game
	if err = cursor.All(ctx, &results); err != nil {
		slog.Error("Failed to load deals", "error", err)
		return
	}

	slog.Info("Fetched deals", "count", len(results))

	marshalErrors := 0
	for _, result := range results {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			slog.Error("Failed to marshal game", "title", result.Title, "error", err)
			marshalErrors++
			continue
		}

		fmt.Println(string(data))
	}

	slog.Info("DisplayDeals completed", "count", len(results), "marshalErrors", marshalErrors)

}
