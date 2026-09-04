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
	skip := int64((page - 1) * 25)

	opts := options.Find().SetLimit(pageSize).SetSkip(skip)

	cursor, err := collection.Find(ctx, bson.D{}, opts)
	var results []types.Game
	if err = cursor.All(context.TODO(), &results); err != nil {
		slog.Error("Failed to load", "page", page, "error", err)
	}

	for _, result := range results {
		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			slog.Error("Failed to marshal game", "error", err)
			continue
		}

		fmt.Println(string(data))
	}
}
