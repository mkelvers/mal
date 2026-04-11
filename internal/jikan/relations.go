package jikan

import (
	"context"
	"log"
	"strings"
)

const maxWatchOrderEntries = 120

func watchOrderTypeLabel(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	switch normalized {
	case "tv":
		return "TV"
	case "movie":
		return "Movie"
	default:
		return strings.TrimSpace(value)
	}
}

func isAllowedWatchOrderType(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized == "tv" || normalized == "movie"
}

func (c *Client) currentOnlyRelation(ctx context.Context, id int) ([]RelationEntry, error) {
	currentAnime, err := c.GetAnimeByID(ctx, id)
	if err != nil {
		return nil, err
	}

	return []RelationEntry{{
		Anime:     currentAnime,
		Relation:  "Current",
		IsCurrent: true,
		IsExtra:   false,
	}}, nil
}

func (c *Client) GetFullRelations(ctx context.Context, id int) ([]RelationEntry, error) {
	watchOrder, found := c.watchOrders.Get(id)
	if !found {
		log.Printf("relations: no local watch-order data for %d", id)
		return c.currentOnlyRelation(ctx, id)
	}

	seen := make(map[int]bool)
	relations := make([]RelationEntry, 0, len(watchOrder)+1)

	for _, watchOrderEntry := range watchOrder {
		if len(relations) >= maxWatchOrderEntries {
			break
		}

		if !isAllowedWatchOrderType(watchOrderEntry.Type) {
			continue
		}

		if seen[watchOrderEntry.ID] {
			continue
		}

		anime, err := c.GetAnimeByID(ctx, watchOrderEntry.ID)
		if err != nil {
			log.Printf("relations: skipping related anime %d for root %d: %v", watchOrderEntry.ID, id, err)
			continue
		}

		seen[watchOrderEntry.ID] = true
		relations = append(relations, RelationEntry{
			Anime:     anime,
			Relation:  watchOrderTypeLabel(watchOrderEntry.Type),
			IsCurrent: watchOrderEntry.ID == id,
			IsExtra:   false,
		})
		if watchOrderEntry.ID == id {
			relations[len(relations)-1].Relation = "Current"
		}
	}

	if !seen[id] {
		currentAnime, err := c.GetAnimeByID(ctx, id)
		if err != nil {
			return nil, err
		}

		relations = append([]RelationEntry{{
			Anime:     currentAnime,
			Relation:  "Current",
			IsCurrent: true,
			IsExtra:   false,
		}}, relations...)
	}

	if len(relations) == 0 {
		return c.currentOnlyRelation(ctx, id)
	}

	return relations, nil
}
