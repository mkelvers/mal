package jikan

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

type SearchResult struct {
	Animes      []Anime
	HasNextPage bool
}

type TopAnimeResult struct {
	Animes      []Anime
	HasNextPage bool
}

type Client struct {
	httpClient     *http.Client
	baseURL        string
	cache          *expirable.LRU[string, SearchResult]
	topCache       *expirable.LRU[int, TopAnimeResult]
	animeCache     *expirable.LRU[int, Anime]
	relationsCache *expirable.LRU[int, JikanRelationsResponse]
}

func NewClient() *Client {
	cache := expirable.NewLRU[string, SearchResult](500, nil, time.Hour*1)
	topCache := expirable.NewLRU[int, TopAnimeResult](100, nil, time.Hour*1)
	animeCache := expirable.NewLRU[int, Anime](1000, nil, time.Hour*24)
	relationsCache := expirable.NewLRU[int, JikanRelationsResponse](1000, nil, time.Hour*24)

	return &Client{
		httpClient:     &http.Client{Timeout: 10 * time.Second},
		baseURL:        "https://api.jikan.moe/v4",
		cache:          cache,
		topCache:       topCache,
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

// Search returns the anime list with pagination support
func (c *Client) Search(query string, page int) (SearchResult, error) {
	if query == "" {
		return SearchResult{}, nil
	}
	if page < 1 {
		page = 1
	}

	cacheKey := fmt.Sprintf("search:%s:%d", query, page)
	if cached, ok := c.cache.Get(cacheKey); ok {
		return cached, nil
	}

	var result SearchResponse
	reqURL := fmt.Sprintf("%s/anime?q=%s&page=%d", c.baseURL, url.QueryEscape(query), page)
	if err := c.fetchWithRetry(reqURL, &result); err != nil {
		return SearchResult{}, err
	}

	res := SearchResult{
		Animes:      result.Data,
		HasNextPage: result.Pagination.HasNextPage,
	}

	c.cache.Add(cacheKey, res)
	return res, nil
}

// GetTopAnime fetches the top anime by popularity
func (c *Client) GetTopAnime(page int) (TopAnimeResult, error) {
	if page < 1 {
		page = 1
	}
	if cached, ok := c.topCache.Get(page); ok {
		return cached, nil
	}

	var result TopAnimeResponse
	reqURL := fmt.Sprintf("%s/top/anime?filter=bypopularity&page=%d", c.baseURL, page)
	if err := c.fetchWithRetry(reqURL, &result); err != nil {
		return TopAnimeResult{}, err
	}

	res := TopAnimeResult{
		Animes:      result.Data,
		HasNextPage: result.Pagination.HasNextPage,
	}

	c.topCache.Add(page, res)
	return res, nil
}

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

// GetRelationsData fetches the raw relationships for an anime
func (c *Client) GetRelationsData(id int) (JikanRelationsResponse, error) {
	if cached, ok := c.relationsCache.Get(id); ok {
		return cached, nil
	}

	var result JikanRelationsResponse
	reqURL := fmt.Sprintf("%s/anime/%d/relations", c.baseURL, id)
	if err := c.fetchWithRetry(reqURL, &result); err != nil {
		return JikanRelationsResponse{}, err
	}

	c.relationsCache.Add(id, result)
	return result, nil
}

// findFirstAnimeRelation extracts the first related anime ID for a specific relation type
func findFirstAnimeRelation(res JikanRelationsResponse, relType string) *int {
	for _, group := range res.Data {
		if group.Relation == relType {
			for _, entry := range group.Entry {
				if entry.Type == "anime" {
					id := entry.MalID
					return &id
				}
			}
		}
	}
	return nil
}

// fetchChain recursively builds the relational chain (Prequels or Sequels)
func (c *Client) fetchChain(startID int, direction string, visited map[int]bool) []RelationEntry {
	rels, err := c.GetRelationsData(startID)
	if err != nil {
		return nil
	}

	nextIDPtr := findFirstAnimeRelation(rels, direction)
	if nextIDPtr == nil {
		return nil
	}

	nextID := *nextIDPtr
	if visited[nextID] { // prevent loops
		return nil
	}
	visited[nextID] = true

	anime, err := c.GetAnimeByID(nextID)
	if err != nil {
		return nil
	}

	entry := RelationEntry{Anime: anime, IsCurrent: false}
	rest := c.fetchChain(nextID, direction, visited)

	if direction == "Prequel" {
		return append(rest, entry)
	}
	return append([]RelationEntry{entry}, rest...)
}

// GetFullRelations resolves the full Prequel/Sequel chronological chain synchronously
func (c *Client) GetFullRelations(id int) []RelationEntry {
	currentAnime, err := c.GetAnimeByID(id)
	if err != nil {
		return nil
	}

	visited := map[int]bool{id: true}

	prequels := c.fetchChain(id, "Prequel", visited)

	// Clone visited set for sequels so we don't block valid paths if there's weird branching
	visitedSeq := make(map[int]bool)
	for k, v := range visited {
		visitedSeq[k] = v
	}

	sequels := c.fetchChain(id, "Sequel", visitedSeq)

	var result []RelationEntry
	result = append(result, prequels...)
	result = append(result, RelationEntry{Anime: currentAnime, IsCurrent: true})
	result = append(result, sequels...)

	return result
}
