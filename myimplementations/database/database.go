package database

import (
	"context"
	"fmt"
	"game-price-tracker/myimplementations/types"
	"log/slog"
	"os"
	"time"

	"github.com/joho/godotenv"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.mongodb.org/mongo-driver/v2/mongo/readpref"
)

// connectMongoDB establishes a connection to the MongoDB
//
// On success, connectMongoDB returns a MongoDB client and null error.
// On failure, it returns null client and a Connect/Ping error
func ConnectMongoDB() (*mongo.Client, error) {
	// Load the .env file and log the error if it odes not exists
	if err := godotenv.Load(); err != nil {
		slog.Error("No .env file found, relying on real environment variables", "error", err)
	}

	// Get the MONGO_URI variable and log the error
	// if it is not set
	mongoURI := os.Getenv("MONGO_URI")
	if mongoURI == "" {
		slog.Error("MONGO_URI environment variable not set")
	}

	username := os.Getenv("MONGO_USERNAME")
	password := os.Getenv("MONGO_PASSWORD")
	if username == "" || password == "" {
		return nil, fmt.Errorf("MONGO_USERNAME or MONGO_PASSWORD environment variable not set")
	}

	// Establish a connection to MongoDB and instantiate a mongoDB client
	//
	// Log an error if a connection fails
	clientOpts := options.Client().ApplyURI(mongoURI).SetAuth(options.Credential{
		Username: username,
		Password: password,
	})

	slog.Debug("Connecting to MongoDB")

	client, err := mongo.Connect(clientOpts)
	if err != nil {
		slog.Error("Failed to establish a connection", "error", err)
	}

	// Create a context and ping the MongoDB, giving it a 2 second deadline
	//
	// If it fails to ping, return a null client and an error
	// Otherwise, return the client and a null error
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		client.Disconnect(context.Background())
		return nil, err
	}

	slog.Debug("Connection established")

	return client, nil
}

// upsertGame inserts a game and its price record into the Game document
// if it does not exist. Otherwise, it will update the price history
// if there has been a change, i.e., discount change or price change
func UpsertGame(collection *mongo.Collection, g types.Game) error {
	ctx := context.TODO()

	// Search for the game inside the Game document using its
	// app id.
	//
	// If it does not exists, insert into the document
	// and return the error if there was one.
	var existing types.Game
	err := collection.FindOne(
		ctx,
		bson.M{"_id": g.AppId},
	).Decode(&existing)

	if err == mongo.ErrNoDocuments {
		_, err := collection.InsertOne(context.TODO(), g)
		if err != nil {
			slog.Error(
				"Failed to insert new game",
				"app_id", g.AppId,
				"game", g.Title,
				"error", err,
			)

			return err
		}

		slog.Info(
			"Inserted new game",
			"app_id", g.AppId,
			"game", g.Title,
		)

		return nil
	}

	// If there is an error but insertion failed, return it
	if err != nil {
		slog.Error(
			"Failed to find game",
			"app_id", g.AppId,
			"game", g.Title,
			"error", err,
		)
		return err
	}

	// Get the game's current price
	rec := g.PriceSnapshot

	// Get the last price record from its price history
	last := existing.PriceHistory[len(existing.PriceHistory)-1]

	// True under any of the following condition
	// - The game does not have a pre-existing price history
	// - The last discount price does not match the current discount price
	// - The last original price does not match the current original price
	changed := len(existing.PriceHistory) == 0 ||
		last.DiscountPrice != rec.DiscountPrice ||
		last.OriginalPrice != rec.OriginalPrice

	// Initialize an update variable to update/set these
	// 3 specific field
	update := bson.M{
		"$set": bson.M{
			"price_snapshot": rec,
			"title":          g.Title,
			"url":            g.URL,
		},
	}

	// If there was a change in the game's price history, add it to the
	// update map
	if changed {
		update["$push"] = bson.M{"price_history": rec}
		slog.Info(
			"Updated game price data",
			"app_id", g.AppId,
			"game", g.Title,
			"old_price_data", last,
			"new_price_data", rec,
		)
	}

	// Update the game with the field using its app id and return the error
	_, err = collection.UpdateOne(context.TODO(), bson.M{"_id": g.AppId}, update)
	return err
}
