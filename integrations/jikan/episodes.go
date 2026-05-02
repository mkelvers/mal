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

	// Determining the episode number reliably. Jikan's Episode string can be "Episode 1" or just "1"
	episodeNum := 0
	if e.Episode != "" {
		re := regexp.MustCompile(`\d+`)
		match := re.FindString(e.Episode)
		if match != "" {
			episodeNum, _ = strconv.Atoi(match)
		}
	}

	// For Video episodes, MalID is often the episode number, but let's check
	if episodeNum == 0 {
		episodeNum = e.MalID
	}

	// Always trigger scraping if we encounter the banned icon OR the generic placeholder
	// OR if there is no image URL at all
	if imageUrl == bannedImageURL || imageUrl == placeholderImageURL || imageUrl == "" {
		// MAL URLs usually follow this format, and it redirects to the slug version
		episodeURL := fmt.Sprintf("https://myanimelist.net/anime/%d/_/episode/%d", animeID, episodeNum)
		fallbackURL := scrapeAnimeImageFromEpisodePage(episodeURL, episodeNum)
		
		if fallbackURL != "" {
			return fallbackURL
		}
	}

	return imageUrl
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

	// Log the status code for debugging
	if resp.StatusCode != 200 {
		// fmt.Printf("[DEBUG] Failed to fetch %s: Status %d\n", episodeURL, resp.StatusCode)
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	html := string(body)

	// MAL sometimes redirects to a URL with a slug. 
	// The JSON object is very likely to be present in the full page.
	// We extract the object {} containing "episode_number":X
	episodeStr := strconv.Itoa(episodeNum)
	objPattern := regexp.MustCompile(`\{[^{}]*"episode_number":\s*` + episodeStr + `[^{}]*\}`)
	match := objPattern.FindString(html)
	if match == "" {
		// Try a broader search if the strict one fails
		objPattern = regexp.MustCompile(`\{[^}]*"episode_number":\s*` + episodeStr + `[^}]*\}`)
		match = objPattern.FindString(html)
	}
	
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
