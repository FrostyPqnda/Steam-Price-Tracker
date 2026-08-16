package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gocolly/colly"
)

type PriceRecord struct {
	//AppId         int       `bson:"app_id" json:"app_id"`
	OriginalPrice float64   `bson:"original_price" json:"original_price"`
	DiscountPrice float64   `bson:"discount_price" json:"discount_price"`
	Discount      float64   `bson:"discount" json:"discount"`
	CheckedAt     time.Time `bson:"checked_at" json:"checked_at"`
}

type Game struct {
	AppId        int           `bson:"app_id" json:"app_id"`
	Title        string        `bson:"title" json:"title"`
	URL          string        `bson:"url" json:"url"`
	PriceHistory []PriceRecord `bson:"price_history" json:"price_history"`
}

var games []Game

func parse_pagination_data(totalGames, totalPages *int) {
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

func extract_value(s string) (float64, error) {
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

func track(games *[]Game) {
	var totalGames, totalPages int
	parse_pagination_data(&totalGames, &totalPages)
	fmt.Printf("total_games=%d, total_pages=%d\n", totalGames, totalPages)

	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	c.Limit(&colly.LimitRule{
		Delay:       60 * time.Second,
		RandomDelay: 5 * time.Second,
	})

	for page := 1; page <= 10; page++ {

		c.OnHTML("div#search_resultsRows > a", func(e *colly.HTMLElement) {
			discount, _ := extract_value(e.ChildText("div.discount_pct"))

			original_price, _ := extract_value(e.ChildText("div.discount_original_price"))

			discount_price, _ := extract_value(e.ChildText("div.discount_final_price"))

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
				AppId:        id,
				Title:        e.ChildText("span.title"),
				URL:          strings.TrimPrefix(e.Attr("href"), "?"),
				PriceHistory: []PriceRecord{price},
			}

			*games = append(*games, game)
		})

		c.OnRequest(func(r *colly.Request) {
			fmt.Println("Visiting:", r.URL)
		})

		c.OnError(func(r *colly.Response, err error) {
			fmt.Println("Request error:", err)
		})

		url := fmt.Sprintf("https://store.steampowered.com/search?hwtype=0&supportedlang=english&hidef2p=1&ndl=1&page=%d", page)
		var err = c.Visit(url)
		if err != nil {
			log.Fatal(err)
		}
	}
}

func writeJSON(games *[]Game) error {
	data, err := json.MarshalIndent(games, "", "\t")
	if err != nil {
		return err
	}

	return os.WriteFile("games.json", data, 0644)
}

func main() {
	track(&games)
	err := writeJSON(&games)
	if err != nil {
		panic(err)
	}
}
