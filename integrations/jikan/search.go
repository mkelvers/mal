package jikan

import (
	"context"
	"fmt"
	"net/url"
)

func (c *Client) Search(ctx context.Context, query string, page int) (SearchResult, error) {
	return c.search(ctx, query, page, 0)
}

func (c *Client) SearchAdvanced(ctx context.Context, query, animeType, status, orderBy, sort string, page, limit int) (SearchResult, error) {
	if page < 1 {
		page = 1
	}
	if limit < 0 {
		limit = 0
	}

	cacheKey := fmt.Sprintf("search:%s:%s:%s:%s:%s:%d:%d", query, animeType, status, orderBy, sort, page, limit)

	var result SearchResponse
	reqURL := fmt.Sprintf("%s/anime?page=%d", c.baseURL, page)
	if query != "" {
		reqURL += "&q=" + url.QueryEscape(query)
	}
	if animeType != "" {
		reqURL += "&type=" + url.QueryEscape(animeType)
	}
	if status != "" {
		reqURL += "&status=" + url.QueryEscape(status)
	}
	if orderBy != "" {
		reqURL += "&order_by=" + url.QueryEscape(orderBy)
	}
	if sort != "" {
		reqURL += "&sort=" + url.QueryEscape(sort)
	}
	if limit > 0 {
		reqURL += fmt.Sprintf("&limit=%d", limit)
	}

	if err := c.getWithCache(ctx, cacheKey, shortCacheTTL, reqURL, &result); err != nil {
		if IsRetryableError(err) {
			if fallbackErr := c.fetchWithRetry(ctx, reqURL, &result); fallbackErr == nil {
				return SearchResult{
					Animes:      result.Data,
					HasNextPage: result.Pagination.HasNextPage,
				}, nil
			}
		}
		return SearchResult{}, err
	}

	return SearchResult{
		Animes:      result.Data,
		HasNextPage: result.Pagination.HasNextPage,
	}, nil
}

func (c *Client) search(ctx context.Context, query string, page int, limit int) (SearchResult, error) {
	if query == "" {
		return SearchResult{}, nil
	}
	if page < 1 {
		page = 1
	}
	if limit < 0 {
		limit = 0
	}

	cacheKey := fmt.Sprintf("search:%s:%d:%d", query, page, limit)

	var result SearchResponse
	reqURL := fmt.Sprintf("%s/anime?q=%s&page=%d", c.baseURL, url.QueryEscape(query), page)
	if limit > 0 {
		reqURL = fmt.Sprintf("%s&limit=%d", reqURL, limit)
	}

	if err := c.getWithCache(ctx, cacheKey, shortCacheTTL, reqURL, &result); err != nil {
		if limit > 0 && IsRetryableError(err) {
			fallbackURL := fmt.Sprintf("%s/anime?q=%s&page=%d", c.baseURL, url.QueryEscape(query), page)
			if fallbackErr := c.fetchWithRetry(ctx, fallbackURL, &result); fallbackErr == nil {
				res := SearchResult{
					Animes:      result.Data,
					HasNextPage: result.Pagination.HasNextPage,
				}
				c.setCache(ctx, cacheKey, res, shortCacheTTL)
				return res, nil
			}
		}

		var stale SearchResult
		if c.getStaleCache(ctx, cacheKey, &stale) {
			return stale, nil
		}

		return SearchResult{}, err
	}

	return SearchResult{
		Animes:      result.Data,
		HasNextPage: result.Pagination.HasNextPage,
	}, nil
}

func (c *Client) GetTopAnime(ctx context.Context, page int) (TopAnimeResult, error) {
	if page < 1 {
		page = 1
	}
	cacheKey := fmt.Sprintf("top:%d", page)

	var result TopAnimeResponse
	reqURL := fmt.Sprintf("%s/top/anime?page=%d", c.baseURL, page)

	if err := c.getWithCache(ctx, cacheKey, shortCacheTTL, reqURL, &result); err != nil {
		var stale TopAnimeResult
		if c.getStaleCache(ctx, cacheKey, &stale) {
			return stale, nil
		}
		return TopAnimeResult{}, err
	}

	return TopAnimeResult{
		Animes:      result.Data,
		HasNextPage: result.Pagination.HasNextPage,
	}, nil
}
