# Steam Price Tracker

## Description

A tracking application that scrapes Steam to collect price data from games and tracks their price history over time.

Possible future updates will see notifications when a game is at an all-time low price or experiences drops in price.

For simplicity, this application only scrapes paid games, since F2P games don't necessarily contribute to the price tracker. However, if a previously paid-for game becomes free, it will be tracked. 

This project is an independent, unofficial Steam price tracker and is not affiliated with or endorsed by Valve Corporation.

## Getting Started

### Prerequisites

* Go Version 1.26.5
  
### Running the Application

* go run steam_tracker.go -command=&lt;command&gt;

## List of Available Commands

* ***stats***: Display tracker statistics 
* ***records*** -page=&lt;page&gt; -filter=&lt;filter value&gt;: Display paginated game records, with optional filtering
* ***lowest*** -id=&lt;App Id&gt;: Display the lowest price a game has ever been
* ***history*** -id=&lt;App Id&gt;: Display a game's price history as a trend graph
* ***upsert*** -id=&lt;App Id&gt;: Insert/Update a game data from the Steam page to MongoDB
* ***crawl***: Crawls the Steam store page and scrapes the game content into MongoDB

## License

MIT License
