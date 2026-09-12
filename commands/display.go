package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"game-price-tracker/myimplementations/types"
	"log/slog"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func DisplayRecords(collection *mongo.Collection, page int) {
	ctx := context.TODO()

	pageSize := int64(25)
	skip := int64((page - 1) * int(pageSize))

	slog.Info("Running DisplayRecords", "page", page, "pageSize", pageSize, "skip", skip)

	opts := options.Find().SetLimit(pageSize).SetSkip(skip)

	cursor, err := collection.Find(ctx, bson.D{}, opts)
	var results []types.Game
	if err = cursor.All(ctx, &results); err != nil {
		slog.Error("Failed to load", "page", page, "error", err)
	}

	slog.Info("Fetched records", "page", page, "count", len(results))
	if len(results) == 0 {
		slog.Warn("No records found for page", "page", page, "skip", skip)
	}

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

	slog.Info("DisplayRecords completed", "page", page, "count", len(results), "marshalErrors", marshalErrors)
}
