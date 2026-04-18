package jikan

import (
	"context"
	"fmt"
	"time"
)

func (c *Client) GetEpisodes(ctx context.Context, animeID int, page int) (EpisodesResponse, error) {
	if page < 1 {
		page = 1
	}

	cacheKey := fmt.Sprintf("anime:%d:episodes:%d", animeID, page)
	var cached EpisodesResponse
	if c.getCache(ctx, cacheKey, &cached) {
		return cached, nil
	}

	var stale EpisodesResponse
	hasStale := c.getStaleCache(ctx, cacheKey, &stale)

	var result EpisodesResponse
	reqURL := fmt.Sprintf("%s/anime/%d/episodes?page=%d", c.baseURL, animeID, page)
	if err := c.fetchWithRetry(ctx, reqURL, &result); err != nil {
		if hasStale {
			return stale, nil
		}

		return EpisodesResponse{}, err
	}

	c.setCache(ctx, cacheKey, result, 12*time.Hour)
	return result, nil
}
