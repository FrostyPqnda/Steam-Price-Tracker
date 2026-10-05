package commands

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jedib0t/go-pretty/v6/table"
	"github.com/jedib0t/go-pretty/v6/text"
	"github.com/mattn/go-runewidth"

	mytable "game-price-tracker/myimplementations/table"
	"game-price-tracker/myimplementations/types"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func DisplayRecords(collection *mongo.Collection, page int) {
	ctx := context.TODO()

	pageSize := int64(25)
	skip := (int64(page) - 1) * pageSize

	total, err := collection.CountDocuments(ctx, bson.D{})
	if err != nil {
		slog.Error("Failed to count documents", "error", err)
		return
	}
	totalPages := int((total + pageSize - 1) / pageSize)

	if page > totalPages {
		fmt.Printf("Page %d is out of range (total pages: %d)\n", page, totalPages)
		return
	}

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

	printGames(results, page, totalPages)

	slog.Info("DisplayRecords completed", "page", page, "count", len(results))
}

func printGames(games []types.Game, page, totalPages int) {
	t := mytable.NewTable(fmt.Sprintf("Steam Price Tracker — Records - Page %d of %d", page, totalPages))

	t.AppendHeader(table.Row{"APP ID", "TITLE", "PRICE", "DISCOUNT", "LOWEST", "CHECKED"})

	for _, g := range games {
		s := g.PriceSnapshot

		price := fmt.Sprintf("$%.2f", s.DiscountPrice)
		discount := "-"
		if s.Discount < 0 {
			price = fmt.Sprintf("$%.2f (was $%.2f)", s.DiscountPrice, s.OriginalPrice)
			discount = text.FgGreen.Sprintf("%d%%", s.Discount*-1)
		}

		t.AppendRow(table.Row{
			g.AppId,
			runewidth.Truncate(g.Title, 40, "…"),
			price,
			discount,
			fmt.Sprintf("$%.2f", lowestPrice(g)),
			s.CheckedAt.Local().Format("2006-01-02 15:04"),
		})
	}

	t.SetColumnConfigs([]table.ColumnConfig{
		{Number: 1, Align: text.AlignRight},
		{Number: 4, Align: text.AlignRight, AlignHeader: text.AlignRight},
		{Number: 5, Align: text.AlignRight, AlignHeader: text.AlignRight},
	})

	t.SetCaption("Showing %d games. Next: -command=records -page=%d", len(games), page+1)
	t.Render()
}

func lowestPrice(g types.Game) float64 {
	low := g.PriceSnapshot.DiscountPrice
	for _, h := range g.PriceHistory {
		if h.DiscountPrice < low {
			low = h.DiscountPrice
		}
	}
	return low
}
