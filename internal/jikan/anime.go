package jikan

import (
	"fmt"
	"time"
)

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

	ttl := time.Hour * 24
	if result.Data.Status == "Finished Airing" {
		ttl = time.Hour * 24 * 30
	}

	c.setCache(cacheKey, result.Data, ttl)
	return result.Data, nil
}
