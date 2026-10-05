package register

import (
	"errors"

	"game-price-tracker/commands"
	"game-price-tracker/myimplementations/filter"
	"game-price-tracker/myimplementations/types"
)

func requireID(o types.Options) error {
	if o.ID <= 0 {
		return errors.New("missing or invalid -id")
	}
	return nil
}

func LoadRegisters() map[string]types.Command {
	return map[string]types.Command{
		"stats": func(a *types.App, o types.Options) error {
			commands.DisplayStats(a.Games)
			return nil
		},
		"records": func(a *types.App, o types.Options) error {
			f, err := filter.ParseFilter(o.Filter)
			if err != nil {
				return err
			}
			commands.DisplayRecords(a.Games, o.Page, f)
			return nil
		},
		"lowest": func(a *types.App, o types.Options) error {
			if err := requireID(o); err != nil {
				return err
			}
			commands.SearchLowestPrice(a.Games, o.ID)
			return nil
		},
		"history": func(a *types.App, o types.Options) error {
			if err := requireID(o); err != nil {
				return err
			}
			commands.PriceHistory(a.Games, o.ID)
			return nil
		},
		"upsert": func(a *types.App, o types.Options) error {
			if err := requireID(o); err != nil {
				return err
			}
			commands.UpsertGame(a.Games, o.ID)
			return nil
		},
		"crawl": func(a *types.App, o types.Options) error {
			commands.Crawl(a.Games, a.State)
			return nil
		},
		"help": func(a *types.App, o types.Options) error {
			commands.PrintHelp()
			return nil
		},
	}
}
