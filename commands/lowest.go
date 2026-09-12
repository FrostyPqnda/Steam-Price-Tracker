package commands

import (
	"context"
	"fmt"
	"game-price-tracker/myimplementations/types"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func SearchLowestPrice(collection *mongo.Collection, appId int) {
	ctx := context.TODO()

	slog.Info("Running SearchLowestPrice", "appId", appId)

	var existing types.Game
	err := collection.FindOne(
		ctx,
		bson.M{"_id": appId},
	).Decode(&existing)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			slog.Warn("No game found for app ID", "appId", appId)
		} else {
			slog.Error("Failed to fetch game", "appId", appId, "error", err)
		}
		return
	}

	if len(existing.PriceHistory) == 0 {
		slog.Info("No price history available", "appId", appId, "title", existing.Title)
		fmt.Printf("%s has no price history\n", existing.Title)
		return
	}

	lowest := existing.PriceHistory[0]
	for _, record := range existing.PriceHistory[1:] {
		if record.DiscountPrice < lowest.DiscountPrice {
			lowest = record
		}
	}

	fmt.Printf("Game: %s\n", existing.Title)
	fmt.Printf("Lowest Price: $%.2f\n", lowest.DiscountPrice)
	fmt.Printf("Discount: %.0f%%\n", lowest.Discount)
	fmt.Printf("Recorded: %s\n", lowest.CheckedAt.Format(time.RFC3339))

	slog.Info("SearchLowestPrice completed", "appId", appId)
}
