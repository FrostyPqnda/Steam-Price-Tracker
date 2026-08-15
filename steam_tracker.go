package main

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gocolly/colly"
)

type PriceRecord struct {
	original_price float64
	discount_price float64
	discount       float64
	checked_at     time.Time
}

type Game struct {
	app_id         int
	title          string
	url            string
	platforms      []string
	date_published string
	price_history  []PriceRecord
}

var totalGames, totalPages int
var games []Game

/*func parse_pagination_data(totalGames, totalPages *int) {
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

	var err = c.Visit("https://store.steampowered.com/search?hwtype=0&supportedlang=english&specials=1&hidef2p=1&ndl=1")
	if err != nil {
		log.Fatal(err)
	}
}*/

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
	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36"),
	)

	c.Limit(&colly.LimitRule{
		Delay:       60 * time.Second,
		RandomDelay: 5 * time.Second,
	})

	c.OnHTML("div#search_resultsRows > a", func(e *colly.HTMLElement) {
		discountPct := e.ChildText("div.discount_pct")
		if discountPct == "" {
			return
		}

		discount, err := extract_value(discountPct)
		// Could not extract discount percent
		if err != nil {
			return
		}

		original_price, err := extract_value(e.ChildText("div.discount_original_price"))
		// Could not extract original price
		if err != nil {
			return
		}

		discount_price, err := extract_value(e.ChildText("div.discount_final_price"))
		// Could not extract discount price
		if err != nil {
			return
		}

		platforms := []string{}
		e.ForEach("span.platform_img", func(_ int, el *colly.HTMLElement) {
			class := el.Attr("class")

			if strings.Contains(class, "win") {
				platforms = append(platforms, "Windows")
			}
			if strings.Contains(class, "mac") {
				platforms = append(platforms, "macOS")
			}
			if strings.Contains(class, "linux") {
				platforms = append(platforms, "Linux")
			}
		})

		id, err := strconv.Atoi(e.Attr("data-ds-appid"))
		if err != nil {
			return
		}

		price := PriceRecord{
			original_price: original_price,
			discount_price: discount_price,
			discount:       discount,
			checked_at:     time.Now(),
		}

		game := Game{
			app_id:         id,
			title:          e.ChildText("span.title"),
			url:            e.Attr("href"),
			platforms:      platforms,
			date_published: e.ChildText("div.search_released"),
			price_history:  []PriceRecord{price},
		}

		*games = append(*games, game)
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

func main() {
	//parse_pagination_data(&totalGames, &totalPages)
	//fmt.Printf("total_games=%d, total_pages=%d\n", totalGames, totalPages)
	track(&games)

	for _, game := range games {
		fmt.Println(game.app_id)
		fmt.Println(game.title)
		fmt.Println(game.url)
		fmt.Println(game.platforms)
		fmt.Println(game.date_published)
		fmt.Println("Price History:")
		for _, price := range game.price_history {
			fmt.Fprintf(os.Stdout, "\tOriginal Price: $%.2f\n", price.original_price)
			fmt.Fprintf(os.Stdout, "\tDiscount Price: $%.2f\n", price.discount_price)
			fmt.Fprintf(os.Stdout, "\tDiscount: %.0f%%\n", price.discount)
			fmt.Fprintf(os.Stdout, "\tLast checked: %s\n", price.checked_at.Format("2006-01-02 15:04:05"))
		}
		fmt.Println()
	}
}
