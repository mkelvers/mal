package jikan

import (
	"fmt"
	"strings"
	"time"
)

type ScheduleResult struct {
	Animes      []Anime
	HasNextPage bool
}

func (c *Client) GetSchedule(day string) (ScheduleResult, error) {
	day = strings.ToLower(day)
	cacheKey := fmt.Sprintf("schedule_limit24_%s", day)

	var cached ScheduleResult
	if c.getCache(cacheKey, &cached) {
		return cached, nil
	}

	var result TopAnimeResponse
	reqURL := fmt.Sprintf("%s/schedules?filter=%s&sfw=true&limit=24", c.baseURL, day)
	if err := c.fetchWithRetry(reqURL, &result); err != nil {
		return ScheduleResult{}, err
	}

	res := ScheduleResult{
		Animes:      result.Data,
		HasNextPage: result.Pagination.HasNextPage,
	}

	c.setCache(cacheKey, res, time.Hour*1)
	return res, nil
}

func (c *Client) GetFullSchedule() (map[string][]Anime, error) {
	days := []string{"monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday"}
	schedule := make(map[string][]Anime)

	for _, day := range days {
		res, err := c.GetSchedule(day)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch %s schedule: %w", day, err)
		}
		schedule[day] = res.Animes
	}

	return schedule, nil
}

func (c *Client) GetSeasonsNow(page int) (TopAnimeResult, error) {
	if page < 1 {
		page = 1
	}
	cacheKey := fmt.Sprintf("seasons_now_limit24:%d", page)
	var cached TopAnimeResult
	if c.getCache(cacheKey, &cached) {
		return cached, nil
	}

	var result TopAnimeResponse
	reqURL := fmt.Sprintf("%s/seasons/now?limit=24&page=%d", c.baseURL, page)
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

func (c *Client) GetSeasonsUpcoming(page int) (TopAnimeResult, error) {
	if page < 1 {
		page = 1
	}
	cacheKey := fmt.Sprintf("seasons_upcoming_limit24:%d", page)
	var cached TopAnimeResult
	if c.getCache(cacheKey, &cached) {
		return cached, nil
	}

	var result TopAnimeResponse
	reqURL := fmt.Sprintf("%s/seasons/upcoming?limit=24&page=%d", c.baseURL, page)
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
