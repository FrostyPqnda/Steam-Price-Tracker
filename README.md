# Steam Price Tracker

A command-line tool that scrapes the Steam store, records game prices in MongoDB, and tracks how they change over time.

> This project is an independent, unofficial Steam price tracker and is not affiliated with or endorsed by Valve Corporation.

## Features

- Crawls the Steam store and stores a price snapshot for every paid game
- Keeps a full price history per game
- Paginated, filterable game listings in a readable table
- Stats on tracked games, discounts and all-time lows
- Terminal price-history chart for any game

For simplicity, only paid games are scraped, since free-to-play games don't contribute much to a price tracker. If a previously paid game becomes free, it will continue to be tracked.

## Prerequisites

- Go 1.26.5 (or the version in `go.mod`)
- A MongoDB instance (local install or a hosted cluster such as MongoDB Atlas)
- To preload the games document, just open the MongoDB Atlas application and click [Add Data]

## Setup

### 1. MongoDB

The tracker stores its data in the `steam_price_tracker` database, in the `game` and `metadata` collections. MongoDB creates them on first write.

### 2. Configuration

Copy the example file and fill in your values:

```bash
cp .env.example .env
```

| Variable         | Description                                          |
|------------------|------------------------------------------------------|
| `MONGO_URI`      | Connection string, e.g. `mongodb://localhost:27017`  |
| `MONGO_USERNAME` | Database user                                        |
| `MONGO_PASSWORD` | Database password                                    |

The app reads `.env` from the directory you run it in. Real environment variables work too, so a `.env` file is optional.

> Put credentials in the separate username/password variables, not in the URI.
> `.env` is git-ignored. Don't commit it.

### 3. Install

```bash
git clone https://github.com/FrostyPqnda/Steam-Price-Tracker
cd game-price-tracker
go mod download
```

### Optional: load sample data

A full crawl takes many hours. To try the commands right away, import `games.json` into the `game` collection first:

```bash
mongoimport --uri "<your MONGO_URI>" --db steam_price_tracker --collection game --file games.json
```

In MongoDB Atlas you can do the same from the UI: open your cluster, choose **Browse Collections**, select (or create) the `game` collection in the `steam_price_tracker` database, then click **Add Data** → **Import JSON or CSV file** and pick `games.json`.

## Usage

```bash
go run steam_tracker.go -command=<command> [options]
```

Or build a binary:

```bash
go build -o tracker steam_tracker.go
./tracker -command=stats
```

Flags must come before any other arguments.

### Commands

| Command   | Options                  | Description                                                  |
|-----------|--------------------------|--------------------------------------------------------------|
| `crawl`   |                          | Crawl the Steam store and save game data to MongoDB          |
| `stats`   |                          | Show tracker statistics                                      |
| `records` | `-page`, `-filter`       | Show paginated game records, with optional filtering         |
| `deals`   |                          | Show current deals                                           |
| `lowest`  | `-id=<App ID>`           | Show the lowest price a game has ever had                    |
| `history` | `-id=<App ID>`           | Show a game's price history as a trend chart                 |
| `upsert`  | `-id=<App ID>`           | Insert or update a single game from its Steam page           |
| `help`    |                          | List available commands                                      |

### Filters

The `records` command accepts `-filter` with one or more filters. Combine filters with commas and no spaces. All filters must match.

| Filter             | Matches                                           |
|--------------------|---------------------------------------------------|
| `sale`             | Games currently on sale                           |
| `free`             | Games that are currently free                     |
| `under:<price>`    | Games priced at or under `<price>` (excludes free)|
| `over:<price>`     | Games priced at or over `<price>`                 |
| `discount:<min>`   | Games with at least `<min>`% discount             |
| `name:<text>`      | Games whose title contains `<text>`               |
| `prefix:<text>`    | Games whose title starts with `<text>`            |

Name and prefix matching is case-insensitive. Wrap the filter in quotes if a value contains spaces: `-filter="name:dark souls"`.

### Examples

```bash
go run steam_tracker.go -command=crawl
go run steam_tracker.go -command=stats
go run steam_tracker.go -command=records -page=2
go run steam_tracker.go -command=records -filter=sale
go run steam_tracker.go -command=records -filter=sale,under:10
go run steam_tracker.go -command=records -filter=discount:75,under:20
go run steam_tracker.go -command=records -filter=prefix:half
go run steam_tracker.go -command=lowest -id=49520
go run steam_tracker.go -command=history -id=49520
```

<!-- Add a screenshot of `records` and `history` here:
![records](docs/records.png) -->

## How It Works

`crawl` scrapes the Steam store and saves a price snapshot for each game. On every later check, the previous snapshot is appended to the game's `price_history` and replaced by the new one. Discounts are stored as percentages. Because of this, `lowest` and `history` become useful only after a game has been crawled several times.

A full crawl covers tens of thousands of games and takes a while. Please be considerate of Steam's servers, and expect the scraper to need updates if Steam changes its page layout.

## Logs

Logs are written to `log/tracker.log`, relative to the directory you run the command from. Errors and warnings go to the log file rather than the console, so check it if a command prints nothing or fails unexpectedly.

```bash
tail -f log/tracker.log      # follow the log while crawling
```

The log rotates at 50 MB. The last 10 rotated files are kept for 14 days and compressed with gzip. Set `LOG_LEVEL=debug` for more detail:

```bash
LOG_LEVEL=debug go run steam_tracker.go -command=crawl
```

On Windows PowerShell:

```powershell
$env:LOG_LEVEL="debug"; go run steam_tracker.go -command=crawl
```

## Roadmap

- [ ] Notifications when a game reaches an all-time low
- [ ] Notifications on price drops
- [ ] `-sort` option for `records` (e.g. by discount)

## Contributing

Issues and pull requests are welcome. Please open an issue first for larger changes.

## License

MIT License. See [LICENSE](LICENSE).