package jikan

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

type Client struct {
	httpClient     *http.Client
	baseURL        string
	cache          *expirable.LRU[string, SearchResult]
	topCache       *expirable.LRU[int, TopAnimeResult]
	animeCache     *expirable.LRU[int, Anime]
	relationsCache *expirable.LRU[int, JikanRelationsResponse]
	episodesCache  *expirable.LRU[string, EpisodesResult]
}

func NewClient() *Client {
	cache := expirable.NewLRU[string, SearchResult](500, nil, time.Hour*1)
	topCache := expirable.NewLRU[int, TopAnimeResult](100, nil, time.Hour*1)
	animeCache := expirable.NewLRU[int, Anime](1000, nil, time.Hour*24)
	relationsCache := expirable.NewLRU[int, JikanRelationsResponse](1000, nil, time.Hour*24)
	episodesCache := expirable.NewLRU[string, EpisodesResult](500, nil, time.Hour*6)

	return &Client{
		httpClient:     &http.Client{Timeout: 10 * time.Second},
		baseURL:        "https://api.jikan.moe/v4",
		cache:          cache,
		topCache:       topCache,
		animeCache:     animeCache,
		relationsCache: relationsCache,
		episodesCache:  episodesCache,
	}
}

// fetchWithRetry provides robust fetching respecting Jikan's strict 3 req/sec rate limit
func (c *Client) fetchWithRetry(urlStr string, out interface{}) error {
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		// Base delay for Jikan rate limiting (3 requests per second)
		time.Sleep(340 * time.Millisecond)

		resp, err := c.httpClient.Get(urlStr)
		if err != nil {
			return fmt.Errorf("jikan api error: %w", err)
		}

		if resp.StatusCode == 429 {
			resp.Body.Close()
			time.Sleep(800 * time.Millisecond) // Double delay on rate limit
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return fmt.Errorf("jikan api returned status %d", resp.StatusCode)
		}

		err = json.NewDecoder(resp.Body).Decode(out)
		resp.Body.Close()
		return err
	}
	return fmt.Errorf("max retries exceeded for %s", urlStr)
}

// GetEpisodes fetches episodes for an anime (paginated, 100 per page)
func (c *Client) GetEpisodes(animeID int, page int) (EpisodesResult, error) {
	cacheKey := fmt.Sprintf("%d-%d", animeID, page)
	if cached, ok := c.episodesCache.Get(cacheKey); ok {
		return cached, nil
	}

	url := fmt.Sprintf("%s/anime/%d/episodes?page=%d", c.baseURL, animeID, page)
	var resp EpisodesResponse
	if err := c.fetchWithRetry(url, &resp); err != nil {
		return EpisodesResult{}, err
	}

	result := EpisodesResult{
		Episodes:    resp.Data,
		HasNextPage: resp.Pagination.HasNextPage,
	}
	c.episodesCache.Add(cacheKey, result)
	return result, nil
}

// GetAllEpisodes fetches all episodes for an anime (handles pagination)
func (c *Client) GetAllEpisodes(animeID int) ([]Episode, error) {
	var allEpisodes []Episode
	page := 1

	for {
		result, err := c.GetEpisodes(animeID, page)
		if err != nil {
			return nil, err
		}
		allEpisodes = append(allEpisodes, result.Episodes...)
		if !result.HasNextPage {
			break
		}
		page++
	}

	return allEpisodes, nil
}
