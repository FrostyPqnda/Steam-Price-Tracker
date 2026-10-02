package main

import (
	"context"
	"game-price-tracker/commands"
	"game-price-tracker/myimplementations/database"
	"game-price-tracker/myimplementations/logging"
	"log"
	"log/slog"
	"os"
)

func main() {
	logFile, err := logging.SetupLogging("tracker")
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()

	slog.Info("Starting game price tracker")

	// Establish a connection to MongoDB
	client, err := database.ConnectMongoDB()
	if err != nil {
		slog.Error("Could not connect to MongoDB", "error", err)
		os.Exit(1)
	}

	// Disconnect when main returns
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			slog.Error("MongoDB disconnect error", "error", err)
		}
	}()

	// Connect to the Steam Price Tracker database and get the GAME collection
	gameCollection := client.Database("steam_price_tracker").Collection("game")
	//stateCollection := client.Database("steam_price_tracker").Collection("metadata")

	commands.PriceHistory(gameCollection, 1086940)

	slog.Info("Run completed successfully")
}
