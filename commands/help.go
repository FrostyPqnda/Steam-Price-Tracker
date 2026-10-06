package commands

import "fmt"

func PrintHelp() {
	fmt.Println("List of available commands")
	fmt.Println("--------------------------------")
	fmt.Println("-command=stats\n\tDisplay tracker statistics")
	fmt.Println("-command=records -page=<int> -filter=<string:value>\n\tDisplay a paginated record (25 per page)")
	fmt.Println("-command=lowest -id=<int>\n\tDisplay the lowest price a game has ever been")
	fmt.Println("-command=history -id=<int>\n\tDisplay a game's price history")
	fmt.Println("-command=upsert -id=<int>\n\tInsert/Update a game data to the MongoDB")
	fmt.Println("-command=crawl\n\tCrawls the Steam store page and scrapes the game content and loads it into the MongoDB")
	fmt.Println("--------------------------------")
}
