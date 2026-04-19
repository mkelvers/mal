package jikan

import (
	"context"
	"fmt"
)

type ProducerResponse struct {
	Data struct {
		MalID  int `json:"mal_id"`
		Titles []struct {
			Type  string `json:"type"`
			Title string `json:"title"`
		} `json:"titles"`
		Images struct {
			Jpg struct {
				ImageURL string `json:"image_url"`
			} `json:"jpg"`
		} `json:"images"`
		Favorites   int    `json:"favorites"`
		Established string `json:"established"`
		About       string `json:"about"`
		Count       int    `json:"count"`
		External    []struct {
			Name string `json:"name"`
			URL  string `json:"url"`
		} `json:"external"`
	} `json:"data"`
}

func (c *Client) GetAnimeByProducer(ctx context.Context, producerID int, page int) (StudioAnimeResult, error) {
	if page < 1 {
		page = 1
	}

	cacheKey := fmt.Sprintf("producer:%d:%d", producerID, page)
	var cached StudioAnimeResult
	if c.getCache(ctx, cacheKey, &cached) {
		return cached, nil
	}

	var stale StudioAnimeResult
	hasStale := c.getStaleCache(ctx, cacheKey, &stale)

	var result SearchResponse
	reqURL := fmt.Sprintf("%s/anime?producers=%d&page=%d", c.baseURL, producerID, page)

	if err := c.fetchWithRetry(ctx, reqURL, &result); err != nil {
		if hasStale {
			return stale, nil
		}

		return StudioAnimeResult{}, err
	}

	// Get producer info for the name
	producerName := ""
	var producerRes ProducerResponse
	producerURL := fmt.Sprintf("%s/producers/%d", c.baseURL, producerID)
	if err := c.fetchWithRetry(ctx, producerURL, &producerRes); err == nil {
		for _, title := range producerRes.Data.Titles {
			if title.Type == "Default" {
				producerName = title.Title
				break
			}
		}
	}

	res := StudioAnimeResult{
		Animes:      result.Data,
		HasNextPage: result.Pagination.HasNextPage,
		StudioName:  producerName,
	}

	c.setCache(ctx, cacheKey, res, shortCacheTTL)
	return res, nil
}

func (c *Client) GetProducerByID(ctx context.Context, producerID int) (ProducerResponse, error) {
	cacheKey := fmt.Sprintf("producer:info:%d", producerID)
	var cached ProducerResponse
	if c.getCache(ctx, cacheKey, &cached) {
		return cached, nil
	}

	var stale ProducerResponse
	hasStale := c.getStaleCache(ctx, cacheKey, &stale)

	var result ProducerResponse
	reqURL := fmt.Sprintf("%s/producers/%d/full", c.baseURL, producerID)

	if err := c.fetchWithRetry(ctx, reqURL, &result); err != nil {
		if hasStale {
			return stale, nil
		}

		return ProducerResponse{}, err
	}

	c.setCache(ctx, cacheKey, result, shortCacheTTL)
	return result, nil
}
