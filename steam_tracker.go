package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"

	"game-price-tracker/myimplementations/database"
	"game-price-tracker/myimplementations/filter"
	"game-price-tracker/myimplementations/logging"
	"game-price-tracker/myimplementations/register"
	"game-price-tracker/myimplementations/types"
)

func main() {
	if err := run(); err != nil {
		slog.Error("Run failed", "error", err)
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	slog.Info("Run completed successfully")
}

func run() error {
	command := flag.String("command", "", "command to run (deals, stats, records, lowest, history, upsert, search, crawl)")
	id := flag.Int("id", 0, "Steam app ID")
	page := flag.Int("page", 1, "page number")
	filter := flag.String("filter", "", "filter records: "+filter.FilterHelp)

	flag.Parse()

	if *command == "" {
		flag.Usage()
		return fmt.Errorf("missing -command")
	}

	cmd, ok := register.LoadRegisters()[*command]
	if !ok {
		return fmt.Errorf("unknown command %q", *command)
	}

	logFile, err := logging.SetupLogging("tracker")
	if err != nil {
		return err
	}
	defer logFile.Close()

	slog.Info("Starting game price tracker", "command", *command)

	client, err := database.ConnectMongoDB()
	if err != nil {
		return fmt.Errorf("could not connect to MongoDB: %w", err)
	}
	defer func() {
		if err := client.Disconnect(context.Background()); err != nil {
			slog.Error("MongoDB disconnect error", "error", err)
		}
	}()

	db := client.Database("steam_price_tracker")
	app := &types.App{
		Games: db.Collection("game"),
		State: db.Collection("metadata"),
	}

	return cmd(app, types.Options{ID: *id, Page: *page, Filter: *filter})
}
