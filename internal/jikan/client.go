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
	airingCache    *expirable.LRU[int, TopAnimeResult]
	upcomingCache  *expirable.LRU[int, TopAnimeResult]
	animeCache     *expirable.LRU[int, Anime]
	relationsCache *expirable.LRU[int, JikanRelationsResponse]
}

func NewClient() *Client {
	cache := expirable.NewLRU[string, SearchResult](500, nil, time.Hour*1)
	topCache := expirable.NewLRU[int, TopAnimeResult](100, nil, time.Hour*1)
	airingCache := expirable.NewLRU[int, TopAnimeResult](100, nil, time.Hour*1)
	upcomingCache := expirable.NewLRU[int, TopAnimeResult](100, nil, time.Hour*1)
	animeCache := expirable.NewLRU[int, Anime](1000, nil, time.Hour*24)
	relationsCache := expirable.NewLRU[int, JikanRelationsResponse](1000, nil, time.Hour*24)

	return &Client{
		httpClient:     &http.Client{Timeout: 10 * time.Second},
		baseURL:        "https://api.jikan.moe/v4",
		cache:          cache,
		topCache:       topCache,
		airingCache:    airingCache,
		upcomingCache:  upcomingCache,
		animeCache:     animeCache,
		relationsCache: relationsCache,
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
