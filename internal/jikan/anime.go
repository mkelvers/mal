package jikan

import "fmt"

// GetAnimeByID fetches full details for a single anime
func (c *Client) GetAnimeByID(id int) (Anime, error) {
	if cached, ok := c.animeCache.Get(id); ok {
		return cached, nil
	}

	var result AnimeResponse
	reqURL := fmt.Sprintf("%s/anime/%d/full", c.baseURL, id)
	if err := c.fetchWithRetry(reqURL, &result); err != nil {
		return Anime{}, err
	}

	c.animeCache.Add(id, result.Data)
	return result.Data, nil
}
