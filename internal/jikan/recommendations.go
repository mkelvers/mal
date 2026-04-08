package jikan

import (
	"fmt"
	"time"
)

// RecommendationEntry represents a single recommendation
type RecommendationEntry struct {
	Entry struct {
		MalID  int    `json:"mal_id"`
		URL    string `json:"url"`
		Images struct {
			Webp struct {
				LargeImageURL string `json:"large_image_url"`
			} `json:"webp"`
		} `json:"images"`
		Title string `json:"title"`
	} `json:"entry"`
	Votes int `json:"votes"`
}

type RecommendationsResponse struct {
	Data []RecommendationEntry `json:"data"`
}

// GetRecommendations fetches full details for the top recommended anime
func (c *Client) GetRecommendations(animeID int, limit int) ([]Anime, error) {
	cacheKey := fmt.Sprintf("recs:%d", animeID)
	var cached []Anime
	if c.getCache(cacheKey, &cached) {
		if len(cached) > limit {
			return cached[:limit], nil
		}
		return cached, nil
	}

	var result RecommendationsResponse
	reqURL := fmt.Sprintf("%s/anime/%d/recommendations", c.baseURL, animeID)
	if err := c.fetchWithRetry(reqURL, &result); err != nil {
		return nil, err
	}

	max := len(result.Data)
	if limit > 0 && max > limit {
		max = limit
	}

	animes := make([]Anime, 0, max)
	for i := 0; i < max; i++ {
		rec := result.Data[i]
		// Fetch full details so we get English/Japanese titles
		fullAnime, err := c.GetAnimeByID(rec.Entry.MalID)
		if err == nil {
			animes = append(animes, fullAnime)
		} else {
			// Fallback to partial data if full fetch fails
			animes = append(animes, Anime{
				MalID: rec.Entry.MalID,
				Title: rec.Entry.Title,
				Images: struct {
					Jpg struct {
						LargeImageURL string `json:"large_image_url"`
					} `json:"jpg"`
					Webp struct {
						LargeImageURL string `json:"large_image_url"`
					} `json:"webp"`
				}{
					Webp: struct {
						LargeImageURL string `json:"large_image_url"`
					}{
						LargeImageURL: rec.Entry.Images.Webp.LargeImageURL,
					},
				},
			})
		}
	}

	c.setCache(cacheKey, animes, time.Hour*24)
	return animes, nil
}
