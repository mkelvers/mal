package jikan

import (
	"fmt"
	"time"
)

// GetAnimeByID fetches full details for a single anime
func (c *Client) GetAnimeByID(id int) (Anime, error) {
	cacheKey := fmt.Sprintf("anime:%d", id)
	var cached Anime
	if c.getCache(cacheKey, &cached) {
		return cached, nil
	}

	var result AnimeResponse
	reqURL := fmt.Sprintf("%s/anime/%d/full", c.baseURL, id)
	if err := c.fetchWithRetry(reqURL, &result); err != nil {
		return Anime{}, err
	}

	c.setCache(cacheKey, result.Data, time.Hour*24)
	return result.Data, nil
}
