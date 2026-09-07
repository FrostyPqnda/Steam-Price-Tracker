package main

import (
	"context"
	"fmt"
	"game-price-tracker/commands"
	"game-price-tracker/myimplementations/database"
	"log/slog"
	"os"
	"path/filepath"
	"time"
)

func setupLogging(serviceName string) (*os.File, error) {
	var logDir string = "log"

	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	timestamp := time.Now().Format("2006-01-02_10-19-59")
	logPath := filepath.Join(logDir, fmt.Sprintf("%s_%s.log", serviceName, timestamp))

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)

	if err != nil {
		return nil, fmt.Errorf("failed to open log file: %w", err)
	}

	logger := slog.New(slog.NewTextHandler(logFile, nil))
	slog.SetDefault(logger)
	return logFile, nil
}

func main() {
	/*logFile, err := setupLogging("tracker")
	if err != nil {
		log.Fatal(err)
	}
	defer logFile.Close()*/

	// Establish a connection to MongoDB
	client, err := database.ConnectMongoDB()
	if err != nil {
		slog.Error("Could not connect to MongoDB", "error", err)
	}

	// Dissconnect if there was an issue with connecting
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			slog.Error("MongoDB disconnect error", "error", err)
		}
	}()

	// Connect to the Steam Price Tracker database and get the GAME collection
	collection := client.Database("steam_price_tracker").Collection("game")

	//commands.Crawl(collection)
	commands.DisplayStats(collection)
}
