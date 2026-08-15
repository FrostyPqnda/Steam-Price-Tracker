# Steam Price Tracker

## Description

A tracking application that scrapes the Steam page to collect price data for games and notifies users when a game is 
at an all-time low price or drops in price

For simplicity, this application only scrapes paid games, since F2P games don't necessarily contribute to the price tracker. 
However, if a previously paid-for game becomes free, it will be tracked. 

## Getting Started

### Prerequisites

* Go Version 1.26.5
  
### Running the Application

* go run steam_tracker.go &lt;game title&gt;

## License

MIT License
