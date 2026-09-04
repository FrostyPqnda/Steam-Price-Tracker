package commands

import (
	"context"
	"fmt"
	"game-price-tracker/myimplementations/types"
	"log/slog"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func PriceHistory(collection *mongo.Collection, appId int) {
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

	fmt.Printf("%s Price History\n", existing.Title)

	fmt.Println("========= Current Price Snapshot =========")

	fmt.Printf("Original Price: $%.2f\n", existing.PriceSnapshot.OriginalPrice)
	fmt.Printf("Discount Price: $%.2f\n", existing.PriceSnapshot.DiscountPrice)
	fmt.Printf("Discount: %.0f\n", existing.PriceSnapshot.Discount)
	fmt.Printf("Recorded: %s\n", existing.PriceSnapshot.CheckedAt)

	fmt.Println("============= Price History =============")

	for _, price := range existing.PriceHistory {
		fmt.Printf("Original Price: $%.2f\n", price.OriginalPrice)
		fmt.Printf("Discount Price: $%.2f\n", price.DiscountPrice)
		fmt.Printf("Discount: %.0f\n", price.Discount)
		fmt.Printf("Recorded: %s\n", price.CheckedAt)
	}

	fmt.Println("=========================================")

}
