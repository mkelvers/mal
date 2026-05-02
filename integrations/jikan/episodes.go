package jikan

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const bannedImageURL = "https://myanimelist.net/images/icon-banned-youtube.png"
const placeholderImageURL = "https://myanimelist.net/images/episodes/videos/icon-thumbs-not-available.png"

var httpClient = &http.Client{Timeout: 10 * time.Second}

func (e *Episode) GetFallbackImage(animeID int) string {
	imageUrl := ""
	if e.Images != nil {
		imageUrl = e.Images.Jpg.ImageURL
	}

	if imageUrl != bannedImageURL && imageUrl != placeholderImageURL && imageUrl != "" {
		return imageUrl
	}

	episodeNum := 1
	if e.Episode != "" {
		// e.Episode can be "Episode 1" or just "1"
		re := regexp.MustCompile(`\d+`)
		match := re.FindString(e.Episode)
		if match != "" {
			episodeNum, _ = strconv.Atoi(match)
		} else {
			episodeNum, _ = strconv.Atoi(e.Episode)
		}
	}
	if episodeNum == 0 {
		episodeNum = e.MalID
	}

	episodeURL := fmt.Sprintf("https://myanimelist.net/anime/%d/episode/%d", animeID, episodeNum)
	fallbackURL := scrapeAnimeImageFromEpisodePage(episodeURL, episodeNum)
	if fallbackURL != "" {
		return fallbackURL
	}

	return e.Images.Jpg.ImageURL
}

func scrapeAnimeImageFromEpisodePage(episodeURL string, episodeNum int) string {
	req, err := http.NewRequest("GET", episodeURL, nil)
	if err != nil {
		return ""
	}
	// Setting User-Agent is important for MAL
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	html := string(body)

	// Look for the JSON data in MAL.episodeVideo.aroundVideos
	// We extract the object {} containing "episode_number":X
	episodeStr := strconv.Itoa(episodeNum)
	objPattern := regexp.MustCompile(`\{[^{}]*"episode_number":\s*` + episodeStr + `[^{}]*\}`)
	match := objPattern.FindString(html)
	if match != "" {
		thumbRe := regexp.MustCompile(`"thumbnail":\s*"([^"]+)"`)
		thumbMatch := thumbRe.FindStringSubmatch(match)
		if len(thumbMatch) > 1 {
			// Unescape backslashes in URL
			return strings.ReplaceAll(thumbMatch[1], `\/`, `/`)
		}
	}

	return ""
}

func (c *Client) GetEpisodes(ctx context.Context, animeID int, page int) (EpisodesResponse, error) {
	if page < 1 {
		page = 1
	}

	cacheKey := fmt.Sprintf("anime:%d:episodes:%d", animeID, page)
	var result EpisodesResponse
	reqURL := fmt.Sprintf("%s/anime/%d/episodes?page=%d", c.baseURL, animeID, page)

	err := c.getWithCache(ctx, cacheKey, 12*time.Hour, reqURL, &result)
	return result, err
}

func (c *Client) GetVideoEpisodes(ctx context.Context, animeID int, page int) (EpisodesResponse, error) {
	if page < 1 {
		page = 1
	}

	cacheKey := fmt.Sprintf("anime:%d:videos:episodes:%d", animeID, page)
	var result EpisodesResponse
	reqURL := fmt.Sprintf("%s/anime/%d/videos/episodes?page=%d", c.baseURL, animeID, page)

	err := c.getWithCache(ctx, cacheKey, 12*time.Hour, reqURL, &result)
	return result, err
}

func (c *Client) GetEpisode(ctx context.Context, animeID int, episode int) (EpisodeResponse, error) {
	cacheKey := fmt.Sprintf("anime:%d:episode:%d", animeID, episode)
	var result EpisodeResponse
	reqURL := fmt.Sprintf("%s/anime/%d/episodes/%d", c.baseURL, animeID, episode)

	err := c.getWithCache(ctx, cacheKey, 24*time.Hour, reqURL, &result)
	return result, err
}
