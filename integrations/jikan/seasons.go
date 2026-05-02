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
	cacheKey := fmt.Sprintf("schedule_%s", day)

	var result TopAnimeResponse
	reqURL := fmt.Sprintf("%s/schedules?filter=%s&sfw=true", c.baseURL, day)

	err := c.getWithCache(ctx, cacheKey, shortCacheTTL, reqURL, &result)
	if err != nil {
		return ScheduleResult{}, err
	}

	return ScheduleResult{
		Animes:      result.Data,
		HasNextPage: result.Pagination.HasNextPage,
	}, nil
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
	cacheKey := fmt.Sprintf("seasons_now:%d", page)

	var result TopAnimeResponse
	reqURL := fmt.Sprintf("%s/seasons/now?page=%d", c.baseURL, page)

	err := c.getWithCache(ctx, cacheKey, shortCacheTTL, reqURL, &result)
	if err != nil {
		return TopAnimeResult{}, err
	}

	return TopAnimeResult{
		Animes:      result.Data,
		HasNextPage: result.Pagination.HasNextPage,
	}, nil
}

func (c *Client) GetSeasonsUpcoming(ctx context.Context, page int) (TopAnimeResult, error) {
	if page < 1 {
		page = 1
	}
	cacheKey := fmt.Sprintf("seasons_upcoming:%d", page)

	var result TopAnimeResponse
	reqURL := fmt.Sprintf("%s/seasons/upcoming?page=%d", c.baseURL, page)

	err := c.getWithCache(ctx, cacheKey, shortCacheTTL, reqURL, &result)
	if err != nil {
		return TopAnimeResult{}, err
	}

	return TopAnimeResult{
		Animes:      result.Data,
		HasNextPage: result.Pagination.HasNextPage,
	}, nil
}

func (c *Client) GetRandomAnime(ctx context.Context) (Anime, error) {
	var result struct {
		Data Anime `json:"data"`
	}

	reqURL := fmt.Sprintf("%s/random/anime", c.baseURL)
	err := c.fetchWithRetry(ctx, reqURL, &result)
	if err != nil {
		return Anime{}, err
	}

	return result.Data, nil
}
