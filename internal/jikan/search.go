package jikan

import (
	"fmt"
	"net/url"
	"time"
)

func (c *Client) Search(query string, page int) (SearchResult, error) {
	if query == "" {
		return SearchResult{}, nil
	}
	if page < 1 {
		page = 1
	}

	cacheKey := fmt.Sprintf("search:%s:%d", query, page)
	var cached SearchResult
	if c.getCache(cacheKey, &cached) {
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

	c.setCache(cacheKey, res, time.Hour*1)
	return res, nil
}

func (c *Client) GetTopAnime(page int) (TopAnimeResult, error) {
	if page < 1 {
		page = 1
	}
	cacheKey := fmt.Sprintf("top:%d", page)
	var cached TopAnimeResult
	if c.getCache(cacheKey, &cached) {
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

	c.setCache(cacheKey, res, time.Hour*1)
	return res, nil
}
