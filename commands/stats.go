package commands

import (
	"context"
	"fmt"
	mytable "game-price-tracker/myimplementations/table"
	"game-price-tracker/myimplementations/types"
	"log/slog"
	"math"
	"strconv"
	"time"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/mattn/go-runewidth"
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
	t := mytable.NewTable("Steam Price Tracker — Stats")
	t.AppendHeader(table.Row{"METRIC", "VALUE", "DETAIL"})

	// Overview
	t.AppendRow(table.Row{"Games tracked", commas(totalGames), ""})
	if totalGames > 0 {
		t.AppendRow(table.Row{
			"Currently on sale",
			commas(onSaleCount),
			fmt.Sprintf("%.0f%% of games", float64(onSaleCount)/float64(totalGames)*100),
		})
	}
	if len(totalChecksResults) > 0 {
		t.AppendRow(table.Row{"Total price checks", commas(totalChecksResults[0].TotalChecks), ""})
	}
	t.AppendSeparator()

	// Deals (discounts are stored as negatives, so show the absolute value)
	if hasBiggestDeal {
		d := biggestDeal.PriceSnapshot
		t.AppendRow(table.Row{
			"Biggest discount",
			text.FgGreen.Sprintf("%.0f%% off", math.Abs(float64(d.Discount))),
			fmt.Sprintf("%s ($%.2f → $%.2f)", shortTitle(biggestDeal.Title), d.OriginalPrice, d.DiscountPrice),
		})
	} else {
		t.AppendRow(table.Row{"Biggest discount", "none", ""})
	}
	if len(avgResults) > 0 && onSaleCount > 0 {
		t.AppendRow(table.Row{
			"Avg discount (on sale)",
			fmt.Sprintf("%.0f%%", math.Abs(avgResults[0].AvgDiscount)),
			"",
		})
	}
	if len(bestEverResults) > 0 {
		t.AppendRow(table.Row{
			"Best deal ever seen",
			fmt.Sprintf("$%.2f", bestEverResults[0].AllPrices.DiscountPrice),
			shortTitle(bestEverResults[0].Title),
		})
	}
	t.AppendSeparator()

	// Freshness
	if !mostRecent.PriceSnapshot.CheckedAt.IsZero() {
		t.AppendRow(table.Row{"Last checked", timeAgo(mostRecent.PriceSnapshot.CheckedAt), shortTitle(mostRecent.Title)})
	}
	if !oldest.PriceSnapshot.CheckedAt.IsZero() {
		t.AppendRow(table.Row{"Oldest check", timeAgo(oldest.PriceSnapshot.CheckedAt), shortTitle(oldest.Title)})
	}

	t.SetColumnConfigs([]table.ColumnConfig{
		{Number: 2, Align: text.AlignRight, AlignHeader: text.AlignRight},
	})
	t.Render()

	slog.Info("DisplayStats completed")
}

func commas(n int64) string {
	s := strconv.FormatInt(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

func shortTitle(s string) string {
	return runewidth.Truncate(s, 40, "…")
}
