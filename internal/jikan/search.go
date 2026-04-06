package jikan

import (
	"fmt"
	"net/url"
)

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
