package commands

import (
	"context"
	"fmt"
	"os"
	"sort"
	"time"

	"log/slog"

	"github.com/NimbleMarkets/ntcharts/linechart/timeserieslinechart"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"game-price-tracker/myimplementations/types"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func PriceHistory(collection *mongo.Collection, appId int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	slog.Info("Running PriceHistory", "appId", appId)

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

	slog.Info("Fetched price records", "appId", appId, "title", game.Title, "size", len(game.PriceHistory))
	printChart(game)
}

func printChart(game types.Game) {
	// history + current snapshot, oldest first
	records := append([]types.PriceRecord(nil), game.PriceHistory...)
	if n := len(records); n == 0 || game.PriceSnapshot.CheckedAt.After(records[n-1].CheckedAt) {
		records = append(records, game.PriceSnapshot)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].CheckedAt.Before(records[j].CheckedAt)
	})

	maxPrice := 0.0
	for _, r := range records {
		if r.OriginalPrice > maxPrice {
			maxPrice = r.OriginalPrice
		}
	}
	if maxPrice == 0 {
		maxPrice = 1
	}

	width, height := 100, 20
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 30 {
		width = w - 1
	}

	now := time.Now()
	chart := timeserieslinechart.New(width, height,
		timeserieslinechart.WithTimeRange(records[0].CheckedAt, now),
		timeserieslinechart.WithYRange(0, maxPrice*1.1),
		timeserieslinechart.WithYLabelFormatter(func(_ int, v float64) string {
			return fmt.Sprintf("$%.0f", v)
		}),
	)
	chart.SetDataSetStyle("original", lipgloss.NewStyle().Foreground(lipgloss.Color("8")))
	chart.SetDataSetStyle("price", lipgloss.NewStyle().Foreground(lipgloss.Color("10")))

	push := func(name string, t time.Time, v float64) {
		chart.PushDataSet(name, timeserieslinechart.TimePoint{Time: t, Value: v})
	}

	// Prices are step data: hold the old value until the moment it changes,
	// so the line drops straight down instead of sloping between checks.
	var prevO, prevP float64
	for i, r := range records {
		if i > 0 {
			t := r.CheckedAt.Add(-time.Second)
			push("original", t, prevO)
			push("price", t, prevP)
		}
		push("original", r.CheckedAt, r.OriginalPrice)
		push("price", r.CheckedAt, r.DiscountPrice)
		prevO, prevP = r.OriginalPrice, r.DiscountPrice
	}
	// extend the last value to "now"
	push("original", now, prevO)
	push("price", now, prevP)

	chart.DrawBrailleAll()

	fmt.Printf("\n%s (%d)\n", game.Title, game.AppId)
	fmt.Println(chart.View())
	fmt.Printf("%s original   %s discounted   (%d price checks)\n",
		lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("━━"),
		lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("━━"),
		len(records))
}
