package commands

import (
	"context"
	"fmt"
	"game-price-tracker/myimplementations/types"
	"log/slog"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func timeAgo(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d minutes ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%d hours ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}

func DisplayStats(collection *mongo.Collection) {
	ctx := context.TODO()

	slog.Info("Running DisplayStats")

	totalGames, err := collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		slog.Error("Failed to count total games", "error", err)
		return
	}

	onSaleFilter := bson.M{"price_snapshot.discount": bson.M{"$ne": 0}}
	onSaleCount, err := collection.CountDocuments(ctx, onSaleFilter)
	if err != nil {
		slog.Error("Failed to count games on sale", "error", err)
		return
	}

	// --- Biggest current discount ---
	var biggestDeal types.Game
	opts := options.FindOne().SetSort(bson.M{"price_snapshot.discount": 1})
	err = collection.FindOne(ctx, onSaleFilter, opts).Decode(&biggestDeal)

	hasBiggestDeal := true
	if err != nil {
		if err != mongo.ErrNoDocuments {
			slog.Error("Failed to find biggest discount", "error", err)
			return
		}

		hasBiggestDeal = false
	}

	// --- Average discount among games on sale ---
	avgPipeline := mongo.Pipeline{
		{{Key: "$match", Value: onSaleFilter}},
		{{Key: "$group", Value: bson.M{
			"_id":         nil,
			"avgDiscount": bson.M{"$avg": "$price_snapshot.discount"},
			"totalChecks": bson.M{"$sum": bson.M{"$add": bson.A{bson.M{"$size": "$price_history"}, 1}}},
		}}},
	}

	cursor, err := collection.Aggregate(ctx, avgPipeline)
	if err != nil {
		slog.Error("Failed to run aggregate stats", "error", err)
		return
	}

	var avgResults []struct {
		AvgDiscount float64 `bson:"avgDiscount"`
		TotalChecks int64   `bson:"totalChecks"`
	}
	if err := cursor.All(ctx, &avgResults); err != nil {
		slog.Error("Failed to decode aggregate stats", "error", err)
		return
	}

	// --- Total checks across ALL games (not just on sale) ---
	totalChecksPipeline := mongo.Pipeline{
		{{Key: "$group", Value: bson.M{
			"_id":         nil,
			"totalChecks": bson.M{"$sum": bson.M{"$add": bson.A{bson.M{"$size": "$price_history"}, 1}}},
		}}},
	}

	cursor2, err := collection.Aggregate(ctx, totalChecksPipeline)
	if err != nil {
		slog.Error("Failed to run total checks aggregate", "error", err)
		return
	}
	var totalChecksResults []struct {
		TotalChecks int64 `bson:"totalChecks"`
	}
	if err := cursor2.All(ctx, &totalChecksResults); err != nil {
		slog.Error("Failed to decode total checks", "error", err)
		return
	}

	// --- Best historical low: unwind price_history + snapshot, find global min discount_price ---
	bestEverPipeline := mongo.Pipeline{
		{{Key: "$project", Value: bson.M{
			"title": 1,
			"all_prices": bson.M{"$concatArrays": bson.A{
				"$price_history",
				bson.A{"$price_snapshot"},
			}},
		}}},
		{{Key: "$unwind", Value: "$all_prices"}},
		{{Key: "$sort", Value: bson.M{"all_prices.discount_price": 1}}},
		{{Key: "$limit", Value: 1}},
	}
	cursor3, err := collection.Aggregate(ctx, bestEverPipeline)
	if err != nil {
		slog.Error("Failed to run best-ever aggregate", "error", err)
		return
	}
	var bestEverResults []struct {
		Title     string            `bson:"title"`
		AllPrices types.PriceRecord `bson:"all_prices"`
	}
	if err := cursor3.All(ctx, &bestEverResults); err != nil {
		slog.Error("Failed to decode best-ever result", "error", err)
		return
	}

	// --- Most / least recently checked ---
	var mostRecent, oldest types.Game
	mostRecentOpts := options.FindOne().SetSort(bson.M{"price_snapshot.checked_at": -1})
	if err := collection.FindOne(ctx, bson.M{}, mostRecentOpts).Decode(&mostRecent); err != nil && err != mongo.ErrNoDocuments {
		slog.Error("Failed to find most recently checked game", "error", err)
		return
	}
	oldestOpts := options.FindOne().SetSort(bson.M{"price_snapshot.checked_at": 1})
	if err := collection.FindOne(ctx, bson.M{}, oldestOpts).Decode(&oldest); err != nil && err != mongo.ErrNoDocuments {
		slog.Error("Failed to find oldest checked game", "error", err)
		return
	}

	// --- Print everything ---
	fmt.Println("Steam Price Tracker — Stats")
	fmt.Println("============================")
	fmt.Printf("Games tracked:        %d\n", totalGames)
	if totalGames > 0 {
		fmt.Printf("Currently on sale:    %d (%.0f%%)\n", onSaleCount, float64(onSaleCount)/float64(totalGames)*100)
	}
	if len(totalChecksResults) > 0 {
		fmt.Printf("Total price checks:   %d\n", totalChecksResults[0].TotalChecks)
	}
	fmt.Println()

	if hasBiggestDeal {
		fmt.Printf("Biggest discount:     %s — %.0f%% off ($%.2f → $%.2f)\n",
			biggestDeal.Title,
			biggestDeal.PriceSnapshot.Discount,
			biggestDeal.PriceSnapshot.OriginalPrice,
			biggestDeal.PriceSnapshot.DiscountPrice,
		)
	} else {
		fmt.Println("Biggest discount:     none currently")
	}
	if len(avgResults) > 0 && onSaleCount > 0 {
		fmt.Printf("Avg discount (sale):  %.0f%%\n", avgResults[0].AvgDiscount)
	}
	if len(bestEverResults) > 0 {
		fmt.Printf("Best deal ever seen:  %s — $%.2f (historical low)\n",
			bestEverResults[0].Title,
			bestEverResults[0].AllPrices.DiscountPrice,
		)
	}
	fmt.Println()

	if !mostRecent.PriceSnapshot.CheckedAt.IsZero() {
		fmt.Printf("Last checked:         %s (%s)\n", timeAgo(mostRecent.PriceSnapshot.CheckedAt), mostRecent.Title)
	}
	if !oldest.PriceSnapshot.CheckedAt.IsZero() {
		fmt.Printf("Oldest check:         %s (%s)\n", timeAgo(oldest.PriceSnapshot.CheckedAt), oldest.Title)
	}

	slog.Info("DisplayStats completed")
}
