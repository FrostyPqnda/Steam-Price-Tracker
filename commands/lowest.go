package commands

import (
	"context"
	"fmt"
	"log/slog"
	"math"

	mytable "game-price-tracker/myimplementations/table"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"

	"game-price-tracker/myimplementations/types"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func SearchLowestPrice(collection *mongo.Collection, appId int) {
	ctx := context.TODO()

	slog.Info("Running SearchLowestPrice", "appId", appId)

	var game types.Game
	err := collection.FindOne(ctx, bson.M{"_id": appId}).Decode(&game)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			slog.Warn("No game found for app ID", "appId", appId)
			fmt.Printf("No game found with app ID %d\n", appId)
		} else {
			slog.Error("Failed to fetch game", "appId", appId, "error", err)
		}
		return
	}

	// Include the current snapshot, not just the history
	current := game.PriceSnapshot
	lowest := current
	for _, record := range game.PriceHistory {
		if record.DiscountPrice < lowest.DiscountPrice {
			lowest = record
		}
	}

	status := text.FgGreen.Sprint("At its lowest price")
	if current.DiscountPrice > lowest.DiscountPrice {
		status = fmt.Sprintf("$%.2f above the lowest", current.DiscountPrice-lowest.DiscountPrice)
	}

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
	t.SetColumnConfigs([]table.ColumnConfig{
		{Number: 2, Align: text.AlignRight},
	})
	t.Render()

	slog.Info("SearchLowestPrice completed", "appId", appId)
}
