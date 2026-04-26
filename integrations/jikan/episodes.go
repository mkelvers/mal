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
	var result EpisodesResponse
	reqURL := fmt.Sprintf("%s/anime/%d/episodes?page=%d", c.baseURL, animeID, page)

	err := c.getWithCache(ctx, cacheKey, 12*time.Hour, reqURL, &result)
	return result, err
}

func (c *Client) GetEpisode(ctx context.Context, animeID int, episode int) (EpisodeResponse, error) {
	cacheKey := fmt.Sprintf("anime:%d:episode:%d", animeID, episode)
	var result EpisodeResponse
	reqURL := fmt.Sprintf("%s/anime/%d/episodes/%d", c.baseURL, animeID, episode)

	err := c.getWithCache(ctx, cacheKey, 24*time.Hour, reqURL, &result)
	return result, err
}
