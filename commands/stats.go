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

// timeAgo returns a rough, human-readable description of how long ago t was,
// such as "just now", "5 minutes ago", "3 hours ago" or "12 days ago".
//
// Durations are truncated, not rounded, and the largest unit is days. Times
// in the future are reported as "just now".
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

// DisplayStats prints summary statistics for the tracker to standard output as
// a table, grouped into three sections:
//
//   - Overview: total games tracked, how many are on sale (and the
//     percentage), and the total number of price checks across all games.
//   - Deals: the game with the biggest current discount, the average discount
//     among games on sale, and the lowest price ever recorded for any game.
//   - Freshness: the most recently and least recently checked games.
//
// Rows are omitted when there is nothing to show (for example, no games on
// sale or an empty collection). The collection must contain documents
// decodable into types.Game.
func DisplayStats(collection *mongo.Collection) {
	ctx := context.TODO()

	slog.Info("Running DisplayStats")

	totalGames, err := collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		slog.Error("Failed to count total games", "error", err)
		return
	}

	// Discounts are stored as negative numbers (-50 means 50% off), so any
	// non-zero value is treated as "on sale".
	onSaleFilter := bson.M{"price_snapshot.discount": bson.M{"$ne": 0}}
	onSaleCount, err := collection.CountDocuments(ctx, onSaleFilter)
	if err != nil {
		slog.Error("Failed to count games on sale", "error", err)
		return
	}

	// --- Biggest current discount ---
	// Ascending sort puts the most negative discount (the biggest sale) first.
	var biggestDeal types.Game
	opts := options.FindOne().SetSort(bson.M{"price_snapshot.discount": 1})
	err = collection.FindOne(ctx, onSaleFilter, opts).Decode(&biggestDeal)

	// "No documents" just means nothing is on sale, which is not an error.
	hasBiggestDeal := true
	if err != nil {
		if err != mongo.ErrNoDocuments {
			slog.Error("Failed to find biggest discount", "error", err)
			return
		}

		hasBiggestDeal = false
	}

	// --- Average discount among games on sale ---
	// $group with _id nil collapses all matching documents into one result.
	// Each game's check count is len(price_history) + 1 for the snapshot.
	// NOTE: totalChecks is computed here but never read; the overall total
	// comes from the next pipeline.
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

	// Slice, not a single struct: an empty match produces zero results.
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
	// Merge history and the current snapshot into one array per game, flatten
	// it to one document per price record, then take the cheapest. $sort
	// followed by $limit 1 lets MongoDB keep only the top record while sorting
	// instead of ordering everything.
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
	// An empty collection is tolerated here; the zero-value check when
	// printing skips these rows.
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

	// Guard against dividing by zero on an empty collection.
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

	// Freshness: a zero CheckedAt means the lookup found nothing.
	if !mostRecent.PriceSnapshot.CheckedAt.IsZero() {
		t.AppendRow(table.Row{"Last checked", timeAgo(mostRecent.PriceSnapshot.CheckedAt), shortTitle(mostRecent.Title)})
	}
	if !oldest.PriceSnapshot.CheckedAt.IsZero() {
		t.AppendRow(table.Row{"Oldest check", timeAgo(oldest.PriceSnapshot.CheckedAt), shortTitle(oldest.Title)})
	}

	// Right-align the value column so the numbers line up.
	t.SetColumnConfigs([]table.ColumnConfig{
		{Number: 2, Align: text.AlignRight, AlignHeader: text.AlignRight},
	})
	t.Render()

	slog.Info("DisplayStats completed")
}

// commas formats n in base 10 with a comma between each group of three digits,
// for example 1234567 becomes "1,234,567".
//
// It is intended for non-negative counts. A negative n would get a comma
// directly after the minus sign when its digit count is a multiple of three
// (for example "-,123").
func commas(n int64) string {
	s := strconv.FormatInt(n, 10)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}

// shortTitle truncates s to at most 40 display columns, ending with "…" if it
// was shortened. It measures display width rather than bytes or runes, so
// wide characters (such as CJK) don't break table alignment
func shortTitle(s string) string {
	return runewidth.Truncate(s, 40, "…")
}
