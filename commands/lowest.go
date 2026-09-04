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
	var existing types.Game
	err := collection.FindOne(
		ctx,
		bson.M{"_id": appId},
	).Decode(&existing)

	if err == mongo.ErrNoDocuments {
		slog.Error("Failed to find game", "app_id", appId, "error", err)
		return
	}

	if len(existing.PriceHistory) == 0 {
		fmt.Printf("%s has no price history", existing.Title)
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
}
