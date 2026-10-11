package commands

import (
	"fmt"
	"os"
	"sort"
	"time"

	"log/slog"

	"github.com/NimbleMarkets/ntcharts/linechart/timeserieslinechart"
	"github.com/charmbracelet/lipgloss"
	"golang.org/x/term"

	"game-price-tracker/myimplementations/database"
	"game-price-tracker/myimplementations/types"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

// PriceHistory looks up the game with the given appId in collection and
// prints its price history as a chart to standard output.
//
// The collection must contian documents decodable into types.Game, keyed by
// _id. The database query is limited to 10 seconds.
//
// If no game matches appId, a message is printed to standard output and a
// warning is logged. Any other lookup failure (including a timeout) is
// only logged at error loevel, so nothing is printed to the user.
func PriceHistory(collection *mongo.Collection, appId int) {
	slog.Info("Running PriceHistory", "appId", appId)

	game, ok := database.FetchGame(collection, appId)
	if !ok {
		return
	}

	slog.Info("PriceHistory completed", "appId", appId, "title", game.Title, "size", len(game.PriceHistory))
	printChart(game)
}

// printChart renders game's price history as a terminal line chart and writes
// it to standard output, preceded by the game's title and app ID and followed
// by a legend with the number of price checks.
//
// The chart plots two series over time: the original price (gray) and the
// discounted price (green). The data is the game's PriceHistory plus its
// current PriceSnapshot, which is included only if it was checked after the
// most recent history record. Records are sorted oldest first. The input game
// is not modified.
//
// Prices are treated as step data: each value is held until the moment it
// changes, so changes appear as vertical drops rather than sloped lines. The
// final values are extended to the current time. The time axis runs from the
// oldest record to now, and the y axis from $0 to 110% of the highest original
// price (scaled to 1 if every original price is zero), so discounted prices
// are assumed never to exceed the highest original price.
//
// The chart is as wide as the terminal (minus one column) when standard output
// is a terminal wider than 30 columns, and 100 columns otherwise. Its height is
// fixed at 20 rows.
func printChart(game types.Game) {
	// Combine history and the current snapshot, oldest first. Copying into a
	// new slice keeps append from writing into game.PriceHistory's backing
	// array, so the caller's data is untouched.
	records := append([]types.PriceRecord(nil), game.PriceHistory...)

	// Add the snapshot only if it is newer than the last history record
	// (otherwise it is already represented). With empty history it is always
	// added, which also guarantees records[0] exists below.
	if n := len(records); n == 0 || game.PriceSnapshot.CheckedAt.After(records[n-1].CheckedAt) {
		records = append(records, game.PriceSnapshot)
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].CheckedAt.Before(records[j].CheckedAt)
	})

	// Find the highest original price to size the y axis.
	maxPrice := 0.0
	for _, r := range records {
		if r.OriginalPrice > maxPrice {
			maxPrice = r.OriginalPrice
		}
	}

	// Avoid a zero-height y range (e.g. free games) which the chart can't draw.
	if maxPrice == 0 {
		maxPrice = 1
	}

	// Default size, used when stdout isn't a terminal (piped or redirected).
	width, height := 100, 20

	// Use the real terminal width when available. Ignore tiny terminals, and
	// leave one column spare to avoid wrapping at the right edge.
	if w, _, err := term.GetSize(int(os.Stdout.Fd())); err == nil && w > 30 {
		width = w - 1
	}

	now := time.Now()
	chart := timeserieslinechart.New(width, height,
		// X axis spans the first record to now, so the last price is shown
		// holding up to the present.
		timeserieslinechart.WithTimeRange(records[0].CheckedAt, now),
		// 10% headroom above the max keeps the top line off the chart border.
		timeserieslinechart.WithYRange(0, maxPrice*1.1),
		// Whole-dollar labels keep the y axis narrow and readable.
		timeserieslinechart.WithYLabelFormatter(func(_ int, v float64) string {
			return fmt.Sprintf("$%.0f", v)
		}),
	)

	// ANSI colors: 8 = bright black (gray), 10 = bright green.
	chart.SetDataSetStyle("original", lipgloss.NewStyle().Foreground(lipgloss.Color("8")))
	chart.SetDataSetStyle("price", lipgloss.NewStyle().Foreground(lipgloss.Color("10")))

	// push adds one point to the named series.
	push := func(name string, t time.Time, v float64) {
		chart.PushDataSet(name, timeserieslinechart.TimePoint{Time: t, Value: v})
	}

	// Prices are step data: hold the old value until the moment it changes,
	// so the line drops straight down instead of sloping between checks.
	// For every record after the first, emit the previous values one second
	// before the new check, then the new values at the check time. The
	// one-second gap makes the transition near-vertical.
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

	// Extend the last known value to "now" so the lines reach the right edge.
	push("original", now, prevO)
	push("price", now, prevP)

	// Draw using braille characters for finer resolution than block characters.
	chart.DrawBrailleAll()

	// Header, chart, then a color-keyed legend matching the series styles above.
	fmt.Printf("\n%s (%d)\n", game.Title, game.AppId)
	fmt.Println(chart.View())
	fmt.Printf("%s original   %s discounted   (%d price checks)\n",
		lipgloss.NewStyle().Foreground(lipgloss.Color("8")).Render("━━"),
		lipgloss.NewStyle().Foreground(lipgloss.Color("10")).Render("━━"),
		len(records))
}
