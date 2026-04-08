package jikan

import (
	"fmt"
	"strings"
	"time"
)

// ScheduleResult contains anime grouped by day
type ScheduleResult struct {
	Animes      []Anime
	HasNextPage bool
}

// GetSchedule fetches anime airing on a specific day
// day can be: monday, tuesday, wednesday, thursday, friday, saturday, sunday, unknown, other
func (c *Client) GetSchedule(day string) (ScheduleResult, error) {
	day = strings.ToLower(day)
	cacheKey := fmt.Sprintf("schedule_%s", day)

	var cached ScheduleResult
	if c.getCache(cacheKey, &cached) {
		return cached, nil
	}

	var result TopAnimeResponse
	reqURL := fmt.Sprintf("%s/schedules?filter=%s&sfw=true", c.baseURL, day)
	if err := c.fetchWithRetry(reqURL, &result); err != nil {
		return ScheduleResult{}, err
	}

	res := ScheduleResult{
		Animes:      result.Data,
		HasNextPage: result.Pagination.HasNextPage,
	}

	c.preWarmAnimeCache(result.Data)
	c.setCache(cacheKey, res, time.Hour*1)
	return res, nil
}

// GetFullSchedule fetches all days at once
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

// GetSeasonsNow fetches currently airing anime
func (c *Client) GetSeasonsNow(page int) (TopAnimeResult, error) {
	if page < 1 {
		page = 1
	}
	cacheKey := fmt.Sprintf("seasons_now:%d", page)
	var cached TopAnimeResult
	if c.getCache(cacheKey, &cached) {
		return cached, nil
	}

	var result TopAnimeResponse
	reqURL := fmt.Sprintf("%s/seasons/now?page=%d", c.baseURL, page)
	if err := c.fetchWithRetry(reqURL, &result); err != nil {
		return TopAnimeResult{}, err
	}

	res := TopAnimeResult{
		Animes:      result.Data,
		HasNextPage: result.Pagination.HasNextPage,
	}

	c.preWarmAnimeCache(result.Data)
	c.setCache(cacheKey, res, time.Hour*1)
	return res, nil
}

// GetSeasonsUpcoming fetches upcoming anime
func (c *Client) GetSeasonsUpcoming(page int) (TopAnimeResult, error) {
	if page < 1 {
		page = 1
	}
	cacheKey := fmt.Sprintf("seasons_upcoming:%d", page)
	var cached TopAnimeResult
	if c.getCache(cacheKey, &cached) {
		return cached, nil
	}

	var result TopAnimeResponse
	reqURL := fmt.Sprintf("%s/seasons/upcoming?page=%d", c.baseURL, page)
	if err := c.fetchWithRetry(reqURL, &result); err != nil {
		return TopAnimeResult{}, err
	}

	res := TopAnimeResult{
		Animes:      result.Data,
		HasNextPage: result.Pagination.HasNextPage,
	}

	c.preWarmAnimeCache(result.Data)
	c.setCache(cacheKey, res, time.Hour*1)
	return res, nil
}
