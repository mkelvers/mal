package jikan

import (
	"context"
	"fmt"
	"strings"
)

type ScheduleResult struct {
	Animes      []Anime
	HasNextPage bool
}

func (c *Client) GetSchedule(ctx context.Context, day string) (ScheduleResult, error) {
	day = strings.ToLower(day)
	cacheKey := fmt.Sprintf("schedule_limit%d_%s", ListPageSize, day)

	var cached ScheduleResult
	if c.getCache(ctx, cacheKey, &cached) {
		return cached, nil
	}

	var stale ScheduleResult
	hasStale := c.getStaleCache(ctx, cacheKey, &stale)

	var result TopAnimeResponse
	reqURL := fmt.Sprintf("%s/schedules?filter=%s&sfw=true&limit=%d", c.baseURL, day, ListPageSize)
	if err := c.fetchWithRetry(ctx, reqURL, &result); err != nil {
		if hasStale {
			return stale, nil
		}

		return ScheduleResult{}, err
	}

	res := ScheduleResult{
		Animes:      result.Data,
		HasNextPage: result.Pagination.HasNextPage,
	}

	c.setCache(ctx, cacheKey, res, shortCacheTTL)
	return res, nil
}

func (c *Client) GetFullSchedule(ctx context.Context) (map[string][]Anime, error) {
	days := []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}
	schedule := make(map[string][]Anime)

	for _, day := range days {
		res, err := c.GetSchedule(ctx, day)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch %s schedule: %w", day, err)
		}
		schedule[day] = res.Animes
	}

	return schedule, nil
}

func (c *Client) GetSeasonsNow(ctx context.Context, page int) (TopAnimeResult, error) {
	if page < 1 {
		page = 1
	}
	cacheKey := fmt.Sprintf("seasons_now_limit%d:%d", ListPageSize, page)
	var cached TopAnimeResult
	if c.getCache(ctx, cacheKey, &cached) {
		return cached, nil
	}

	var stale TopAnimeResult
	hasStale := c.getStaleCache(ctx, cacheKey, &stale)

	var result TopAnimeResponse
	reqURL := fmt.Sprintf("%s/seasons/now?limit=%d&page=%d", c.baseURL, ListPageSize, page)
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

	c.setCache(ctx, cacheKey, res, shortCacheTTL)
	return res, nil
}

func (c *Client) GetSeasonsUpcoming(ctx context.Context, page int) (TopAnimeResult, error) {
	if page < 1 {
		page = 1
	}
	cacheKey := fmt.Sprintf("seasons_upcoming_limit%d:%d", ListPageSize, page)
	var cached TopAnimeResult
	if c.getCache(ctx, cacheKey, &cached) {
		return cached, nil
	}

	var stale TopAnimeResult
	hasStale := c.getStaleCache(ctx, cacheKey, &stale)

	var result TopAnimeResponse
	reqURL := fmt.Sprintf("%s/seasons/upcoming?limit=%d&page=%d", c.baseURL, ListPageSize, page)
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

	c.setCache(ctx, cacheKey, res, shortCacheTTL)
	return res, nil
}
