package jikan

import "fmt"

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

// GetRecommendations fetches recommendations for an anime
func (c *Client) GetRecommendations(animeID int) ([]Anime, error) {
	if cached, ok := c.recsCache.Get(animeID); ok {
		return cached, nil
	}

	var result RecommendationsResponse
	reqURL := fmt.Sprintf("%s/anime/%d/recommendations", c.baseURL, animeID)
	if err := c.fetchWithRetry(reqURL, &result); err != nil {
		return nil, err
	}

	// Convert to Anime slice (partial data)
	animes := make([]Anime, 0, len(result.Data))
	for _, rec := range result.Data {
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

	c.recsCache.Add(animeID, animes)
	return animes, nil
}
