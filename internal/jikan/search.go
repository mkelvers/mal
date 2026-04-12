package jikan

import (
	"context"
	"fmt"
	"net/url"
	"time"
)

func (c *Client) Search(ctx context.Context, query string, page int) (SearchResult, error) {
	if query == "" {
		return SearchResult{}, nil
	}
	if page < 1 {
		page = 1
	}

	cacheKey := fmt.Sprintf("search:limit24:%s:%d", query, page)
	var cached SearchResult
	if c.getCache(ctx, cacheKey, &cached) {
		return cached, nil
	}

	var stale SearchResult
	hasStale := c.getStaleCache(ctx, cacheKey, &stale)

	var result SearchResponse
	reqURL := fmt.Sprintf("%s/anime?q=%s&limit=24&page=%d", c.baseURL, url.QueryEscape(query), page)

	if err := c.fetchWithRetry(ctx, reqURL, &result); err != nil {
		if hasStale {
			return stale, nil
		}

		return SearchResult{}, err
	}

	res := SearchResult{
		Animes:      result.Data,
		HasNextPage: result.Pagination.HasNextPage,
	}

	c.setCache(ctx, cacheKey, res, time.Hour*1)
	return res, nil
}

func (c *Client) GetTopAnime(ctx context.Context, page int) (TopAnimeResult, error) {
	if page < 1 {
		page = 1
	}
	cacheKey := fmt.Sprintf("top:limit24:%d", page)
	var cached TopAnimeResult
	if c.getCache(ctx, cacheKey, &cached) {
		return cached, nil
	}

	var stale TopAnimeResult
	hasStale := c.getStaleCache(ctx, cacheKey, &stale)

	var result TopAnimeResponse
	reqURL := fmt.Sprintf("%s/top/anime?filter=bypopularity&limit=24&page=%d", c.baseURL, page)

	if err := c.fetchWithRetry(ctx, reqURL, &result); err != nil {
		if hasStale {
			return stale, nil
		}

		return TopAnimeResult{}, err
	}

	res := TopAnimeResult{
		Animes:      result.Data,
		HasNextPage: result.Pagination.HasNextPage,
	}

	c.setCache(ctx, cacheKey, res, time.Hour*1)
	return res, nil
}
