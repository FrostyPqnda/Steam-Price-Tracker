package commands

import (
	"context"
	"fmt"
	"game-price-tracker/myimplementations/types"
	"log/slog"
	"net/http"
	"sort"
	"time"

	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
	etypes "github.com/go-echarts/go-echarts/v2/types"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const chartTimeLayout = "2006-01-02 15:04:05"

func PriceHistory(collection *mongo.Collection, appId int) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	slog.Info("Running PriceHistory", "appId", appId)

	var existing types.Game
	err := collection.FindOne(
		ctx,
		bson.M{"_id": appId},
	).Decode(&existing)

	if err != nil {
		if err == mongo.ErrNoDocuments {
			slog.Warn("No game found for app ID", "appId", appId)
		} else {
			slog.Error("Failed to fetch game", "appId", appId, "error", err)
		}
		return
	}

	slog.Info("Found a game that matched the app ID", "appId", appId, "title", existing.Title)
	slog.Info("Fetched price records", "appId", appId, "title", existing.Title, "size", len(existing.PriceHistory))

	if len(existing.PriceHistory) == 0 {
		fmt.Printf("%s has no price history\n", existing.Title)
		return
	}
	addr := "127.0.0.1:8081"
	http.HandleFunc("/", trendHandler(existing))

	fmt.Printf("Chart for %s at http://%s (Ctrl+C to stop)\n", existing.Title, addr)
	if err := http.ListenAndServe(addr, nil); err != nil {
		slog.Error("HTTP server failed", "error", err)
	}
}

func trendHandler(game types.Game) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		line := charts.NewLine()
		line.SetGlobalOptions(
			charts.WithInitializationOpts(opts.Initialization{Theme: etypes.ThemeWesteros}),
			charts.WithTitleOpts(opts.Title{Title: game.Title}),
			charts.WithXAxisOpts(opts.XAxis{Type: "time"}),
			charts.WithYAxisOpts(opts.YAxis{Name: "USD"}),
			charts.WithTooltipOpts(opts.Tooltip{Show: opts.Bool(true), Trigger: "axis"}),
			charts.WithLegendOpts(opts.Legend{Show: opts.Bool(true), Top: "bottom"}),
			charts.WithDataZoomOpts(opts.DataZoom{Type: "slider"}),
		)

		records := make([]types.PriceRecord, len(game.PriceHistory))
		copy(records, game.PriceHistory)
		sort.Slice(records, func(i, j int) bool {
			return records[i].CheckedAt.Before(records[j].CheckedAt)
		})

		now := time.Now()
		stepOpts := charts.WithLineChartOpts(opts.LineChart{Step: "end"})
		noLabels := charts.WithLabelOpts(opts.Label{Show: opts.Bool(false)})

		line.AddSeries("Original price",
			generateLineItems(records, now, func(r types.PriceRecord) float64 { return r.OriginalPrice }),
			stepOpts, noLabels,
		).AddSeries("Discount price",
			generateLineItems(records, now, func(r types.PriceRecord) float64 { return r.DiscountPrice }),
			stepOpts, noLabels,
		)

		if err := line.Render(w); err != nil {
			slog.Error("Failed to render chart", "error", err)
			http.Error(w, "render failed", http.StatusInternalServerError)
		}
	}
}

func generateLineItems(records []types.PriceRecord, end time.Time,
	value func(types.PriceRecord) float64) []opts.LineData {

	items := make([]opts.LineData, 0, len(records)+1)
	for _, r := range records {
		items = append(items, opts.LineData{
			Value: []interface{}{r.CheckedAt.Format(chartTimeLayout), value(r)},
		})
	}

	last := records[len(records)-1]
	if end.After(last.CheckedAt) {
		items = append(items, opts.LineData{
			Value: []interface{}{end.Format(chartTimeLayout), value(last)},
		})
	}
	return items
}
