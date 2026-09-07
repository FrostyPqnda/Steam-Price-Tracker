package commands

import (
	"context"
	"fmt"
	"game-price-tracker/myimplementations/types"
	"log/slog"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

func UpsertGame(collection *mongo.Collection, appId int) (types.Game, error) {
	ctx := context.TODO()

	// Search for the game inside the Game document using its
	// app id.
	//
	// If it does not exists, insert into the document
	// and return the error if there was one.
	var existing types.Game
	err := collection.FindOne(
		ctx,
		bson.M{"_id": appId},
	).Decode(&existing)

	// Game does not exists
	if err == mongo.ErrNoDocuments {
		// Game does not exist yet, so we can't create it
		return types.Game{}, fmt.Errorf("game with app_id %d not found", appId)
	}

	// If there is an error but insertion failed, return it
	if err != nil {
		slog.Error(
			"Failed to find game",
			"app_id", existing.AppId,
			"game", existing.Title,
			"error", err,
		)
		return types.Game{}, err
	}

	// Get the game's current price
	rec := existing.PriceSnapshot

	// True under any of the following condition
	// - The game does not have a pre-existing price history
	// - The last discount price does not match the current discount price
	// - The last original price does not match the current original price
	changed := len(existing.PriceHistory) > 1

	if !changed {
		// Get the last price record from its price history
		last := existing.PriceHistory[len(existing.PriceHistory)-1]

		changed = (last.DiscountPrice != rec.DiscountPrice) || (last.OriginalPrice != rec.OriginalPrice)

		if changed {
			slog.Info(
				"Updated game price data",
				"app_id", existing.AppId,
				"game", existing.Title,
				"old_price_data", last,
				"new_price_data", rec,
			)
		}
	}

	// Initialize an update variable to update/set these
	update := bson.M{
		"$set": bson.M{
			"price_snapshot": rec,
			"title":          existing.Title,
			"url":            existing.URL,
		},
	}

	// If there was a change in the game's price history, add it to the
	// update map
	if changed {
		update["$push"] = bson.M{"price_history": rec}
	}

	// Update the game with the field using its app id and return the error
	_, err = collection.UpdateOne(ctx, bson.M{"_id": existing.AppId}, update)

	if err != nil {
		slog.Error(
			"Failed to update game",
			"app_id", existing.AppId,
			"game", existing.Title,
			"error", err,
		)

		return types.Game{}, err
	}

	return existing, nil
}
