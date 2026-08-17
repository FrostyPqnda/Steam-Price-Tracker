package main

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/gocolly/colly"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

type PriceRecord struct {
	OriginalPrice float64   `bson:"original_price" json:"original_price"`
	DiscountPrice float64   `bson:"discount_price" json:"discount_price"`
	Discount      float64   `bson:"discount" json:"discount"`
	CheckedAt     time.Time `bson:"checked_at" json:"checked_at"`
}

type Game struct {
	AppId         int           `bson:"_id" json:"app_id"`
	Title         string        `bson:"title" json:"title"`
	URL           string        `bson:"url" json:"url"`
	PriceSnapshot PriceRecord   `bson:"price_snapshot" json:"price_snapshot"`
	PriceHistory  []PriceRecord `bson:"price_history" json:"price_history"`
}

var games []Game

func connectMongoDB() (*mongo.Client, error) {
	client, err := mongo.Connect(
		options.Client().ApplyURI("mongodb://localhost:27017/"),
	)
	if err != nil {

	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		client.Disconnect(context.Background())
		return nil, err
	}

	return client, nil
}

func upsertGame(collection *mongo.Collection, g Game) error {
	var existing Game
	err := collection.FindOne(
		context.TODO(),
		bson.M{"_id": g.AppId},
	).Decode(&existing)

	if err == mongo.ErrNoDocuments {
		_, insertErr := collection.InsertOne(context.TODO(), g)
		return insertErr
	}

	if err != nil {
		return err
	}

	rec := g.PriceSnapshot
	last := existing.PriceHistory[len(existing.PriceHistory)-1]
	changed := len(existing.PriceHistory) == 0 ||
		last.DiscountPrice != rec.DiscountPrice ||
		last.OriginalPrice != rec.OriginalPrice

	update := bson.M{"$set": bson.M{
		"price_snapshot": rec,
		"title":          g.Title,
		"url":            g.URL,
	}}
	if changed {
		update["$push"] = bson.M{"price_history": rec}
	}

	_, err = collection.UpdateOne(context.TODO(), bson.M{"_id": g.AppId}, update)
	return err
}

func parsePaginationData(totalGames, totalPages *int) {
	c := colly.NewCollector()

	c.OnHTML("div.search_pagination", func(e *colly.HTMLElement) {
		var pagination_left_data string = e.ChildText("div.search_pagination_left")
		var lastSpace = strings.LastIndex(pagination_left_data, " ")
		value, err := strconv.Atoi(pagination_left_data[lastSpace+1:])
		if err != nil {
			log.Fatal("Could not extract total games:", err)
		}
		*totalGames = value
	})

	c.OnHTML("div.search_pagination > div.search_pagination_right", func(e *colly.HTMLElement) {
		e.ForEach("a[href]", func(_ int, el *colly.HTMLElement) {
			page, err := strconv.Atoi(strings.TrimSpace(el.Text))
			if err != nil {
				return
			}

			if page > *totalPages {
				*totalPages = page
			}
		})
	})

	c.OnRequest(func(r *colly.Request) {
		fmt.Println("Visiting:", r.URL)
	})

	c.OnError(func(r *colly.Response, err error) {
		fmt.Println("Request error:", err)
	})

	var err = c.Visit("https://store.steampowered.com/search?hwtype=0&supportedlang=english&hidef2p=1&ndl=1&page=1")
	if err != nil {
		log.Fatal(err)
	}
}

func extractValue(s string) (float64, error) {
	s = strings.TrimSpace(s)

	if strings.HasSuffix(s, "%") {
		s = strings.TrimSuffix(s, "%")
		return strconv.ParseFloat(strings.TrimSpace(s), 64)
	}

	if strings.HasPrefix(s, "$") {
		s = strings.TrimPrefix(s, "$")
		return strconv.ParseFloat(strings.TrimSpace(s), 64)
	}

	return 0, fmt.Errorf("unsupported value: %q", s)
}

func cleanURL(href string) string {
	if idx := strings.Index(href, "?"); idx != -1 {
		return href[:idx]
	}
	return href
}

func track(collection *mongo.Collection) {
	var totalGames, totalPages int
	parsePaginationData(&totalGames, &totalPages)
	fmt.Printf("total_games=%d, total_pages=%d\n", totalGames, totalPages)

	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	c.Limit(&colly.LimitRule{
		Delay:       90 * time.Second,
		RandomDelay: 30 * time.Second,
	})
	c.SetRequestTimeout(30 * time.Second)

	c.OnHTML("div#search_resultsRows > a", func(e *colly.HTMLElement) {
		discount, _ := extractValue(e.ChildText("div.discount_pct"))

		original_price, _ := extractValue(e.ChildText("div.discount_original_price"))

		discount_price, _ := extractValue(e.ChildText("div.discount_final_price"))

		id, err := strconv.Atoi(e.Attr("data-ds-appid"))
		if err != nil {
			return
		}

		if e.DOM.Find("div.discount_block").HasClass("no_discount") {
			original_price = discount_price
			discount = 0
		}

		price := PriceRecord{
			OriginalPrice: original_price,
			DiscountPrice: discount_price,
			Discount:      discount,
			CheckedAt:     time.Now(),
		}

		game := Game{
			AppId:         id,
			Title:         e.ChildText("span.title"),
			URL:           cleanURL(e.Attr("href")),
			PriceSnapshot: price,
			PriceHistory:  []PriceRecord{price},
		}

		if err := upsertGame(collection, game); err != nil {
			fmt.Println("Upsert error:", err)
		}

	})

	c.OnRequest(func(r *colly.Request) {
		fmt.Println("Visiting:", r.URL)
	})

	c.OnError(func(r *colly.Response, err error) {
		fmt.Println("Request error:", err)
	})

	for page := 1; page <= totalPages; page++ {
		url := fmt.Sprintf("https://store.steampowered.com/search?hwtype=0&supportedlang=english&hidef2p=1&ndl=1&page=%d", page)
		var err = c.Visit(url)
		if err != nil {
			log.Fatal(err)
		}
	}

}

func main() {
	client, err := connectMongoDB()
	if err != nil {
		log.Fatal("Could not connect to MongoDB:", err)
	}

	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			log.Println("MongoDB disconnect error:", err)
		}
	}()

	collection := client.Database("steam_price_tracker").Collection("game")

	track(collection)
}
