package jikan

import (
	"context"
	"fmt"
	"strings"
	"time"

	"mal/internal/watchorder"
)

const chiakiWatchOrderURL = "https://chiaki.site/?/tools/watch_order/id/%d"
const watchOrderCacheTTL = time.Hour * 24
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

func relationCacheKey(id int) string {
	return fmt.Sprintf("relations:watch-order:%d", id)
}

func (c *Client) getWatchOrder(ctx context.Context, id int) (watchorder.WatchOrderResult, error) {
	cacheKey := relationCacheKey(id)

	var cached watchorder.WatchOrderResult
	if c.getCache(ctx, cacheKey, &cached) {
		return cached, nil
	}

	watchOrderURL := fmt.Sprintf(chiakiWatchOrderURL, id)
	requestCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	result, err := watchorder.FetchWatchOrder(requestCtx, c.httpClient, watchOrderURL)
	if err != nil {
		return watchorder.WatchOrderResult{}, err
	}

	c.setCache(ctx, cacheKey, result, watchOrderCacheTTL)
	return result, nil
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
	result, err := c.getWatchOrder(ctx, id)
	if err != nil {
		return c.currentOnlyRelation(ctx, id)
	}

	seen := make(map[int]bool)
	relations := make([]RelationEntry, 0, len(result.WatchOrder)+1)

	for _, watchOrderEntry := range result.WatchOrder {
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
