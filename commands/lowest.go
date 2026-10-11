package commands

import (
	"fmt"
	"log/slog"
	"math"

	"game-price-tracker/myimplementations/database"
	mytable "game-price-tracker/myimplementations/table"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// SearchLowestPrice looks up the game with the given appId in collection and
// outputs a table summarizing its loweest recorded price.
//
// The lowest price is the minimum DiscountPrice across the game's PriceHistory
// and its current PriceSnapshot.
//
// The table also shows the current price, the discount at the lowest price, how
// far the current price is above the lowest, the no. of price checks, and how
// long ago the game was last checked.
func SearchLowestPrice(collection *mongo.Collection, appId int) {
	slog.Info("Running SearchLowestPrice", "appId", appId)

	game, ok := database.FetchGame(collection, appId)
	if !ok {
		return
	}

	// Include the current snapshot, not just the history. Starting from the
	// snapshot means a history record must be strictly cheaper to replace it,
	// so ties resolve in favor of the current price.
	current := game.PriceSnapshot
	lowest := current
	for _, record := range game.PriceHistory {
		if record.DiscountPrice < lowest.DiscountPrice {
			lowest = record
		}
	}

	// Highlight when the current price is the best seen; otherwise show the
	// gap in dollars.
	status := text.FgGreen.Sprint("At its lowest price")
	if current.DiscountPrice > lowest.DiscountPrice {
		status = fmt.Sprintf("$%.2f above the lowest", current.DiscountPrice-lowest.DiscountPrice)
	}

	// Build the summary table, titled with the (shortened) name and app ID.
	t := mytable.NewTable(fmt.Sprintf("%s (%d)", shortTitle(game.Title), appId))
	t.AppendRows([]table.Row{
		{"Current price", fmt.Sprintf("$%.2f", current.DiscountPrice)},
		{"Lowest price", text.FgGreen.Sprintf("$%.2f", lowest.DiscountPrice)},
		{"Discount at lowest", fmt.Sprintf("%.0f%%", math.Abs(float64(lowest.Discount)))},
		{"Recorded", lowest.CheckedAt.Local().Format("2006-01-02 15:04")},
		{"Status", status},
		{"Price checks", len(game.PriceHistory) + 1},
		{"Last checked", timeAgo(current.CheckedAt)},
	})
	// Right-align the value column so the numbers line up.
	t.SetColumnConfigs([]table.ColumnConfig{
		{Number: 2, Align: text.AlignRight},
	})
	t.Render()

	slog.Info("SearchLowestPrice completed", "appId", appId)
}
