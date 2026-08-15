# Steam Price Tracker

## Description

A tracking application that scrapes Steam to collect price data for games and tracks their price history over time.

Possible future updates will see notifications when a game is at an all-time low price or experiences drops in price.

For simplicity, this application only scrapes paid games, since F2P games don't necessarily contribute to the price tracker. 
However, if a previously paid-for game becomes free, it will be tracked. 

This project is an independent, unofficial Steam price tracker and is not affiliated with or endorsed by Valve Corporation.

## Getting Started

### Prerequisites

* Go Version 1.26.5
  
### Running the Application

* go run steam_tracker.go &lt;game title&gt;

## License

MIT License
