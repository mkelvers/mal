package jikan

import "fmt"

// GetSeasonsNow fetches currently airing anime
func (c *Client) GetSeasonsNow(page int) (TopAnimeResult, error) {
	if page < 1 {
		page = 1
	}
	if cached, ok := c.airingCache.Get(page); ok {
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

	c.airingCache.Add(page, res)
	return res, nil
}

// GetSeasonsUpcoming fetches upcoming anime
func (c *Client) GetSeasonsUpcoming(page int) (TopAnimeResult, error) {
	if page < 1 {
		page = 1
	}
	if cached, ok := c.upcomingCache.Get(page); ok {
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

	c.upcomingCache.Add(page, res)
	return res, nil
}
