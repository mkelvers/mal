package jikan

import (
	"context"
	"fmt"
	"io"
	"log"
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
		// MAL URLs follow this format: https://myanimelist.net/anime/20/Naruto/episode/1
		// The previous format used /_/ which is sometimes rejected with 405
		episodeURL := fmt.Sprintf("https://myanimelist.net/anime/%d/slug/episode/%d", animeID, episodeNum)
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
		log.Printf("[DEBUG] Scraper failed to fetch %s: Status %d", episodeURL, resp.StatusCode)
		return ""
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	html := string(body)

	// MAL sometimes redirects to a URL with a slug.
	// We look for the "thumbnail" field in the page source.
	
	// Pattern 1: Look for the specific episode object in the JSON data
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
			return strings.ReplaceAll(thumbMatch[1], `\/`, `/`)
		}
	}
	
	// Pattern 2: Fallback to og:image if it's the specific episode page
	ogRe := regexp.MustCompile(`<meta\s+property="og:image"\s+content="([^"]+)"`)
	ogMatch := ogRe.FindStringSubmatch(html)
	if len(ogMatch) > 1 {
		// Only use if it looks like an episode thumbnail (contains /episodes/)
		if strings.Contains(ogMatch[1], "/episodes/") {
			return ogMatch[1]
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

func (c *Client) GetAllEpisodes(ctx context.Context, animeID int) ([]Episode, error) {
	// First fetch the anime to get total episodes count
	anime, err := c.GetAnimeByID(ctx, animeID)
	if err != nil {
		return nil, err
	}

	totalEpisodes := anime.Episodes
	if totalEpisodes <= 0 {
		resp, err := c.GetEpisodes(ctx, animeID, 1)
		if err != nil {
			return nil, err
		}
		return resp.Data, nil
	}

	// Jikan /episodes/video (which has thumbnails) returns ~39-40 per page.
	// Jikan /episodes (standard) returns 100 per page.
	// Since the user wants to prioritize the metadata-rich video clips if possible,
	// we will calculate based on the 100-per-page standard endpoint for the full list,
	// but the background logic remains the same: last page to first.
	pageSize := 100
	lastPage := (totalEpisodes + (pageSize - 1)) / pageSize
	var allEpisodes []Episode
	
	// Fetch last page first (to get most recent episodes immediately)
	lastResp, err := c.GetEpisodes(ctx, animeID, lastPage)
	if err == nil {
		allEpisodes = append(allEpisodes, lastResp.Data...)
	}

	// For the rest, fetch them in reverse order in the background
	if lastPage > 1 {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			// Start from lastPage - 1 and go down to 1
			for p := lastPage - 1; p >= 1; p-- {
				_, _ = c.GetEpisodes(bgCtx, animeID, p)
				
				// Also pre-fetch the video episodes metadata (39 per page)
				// to warm the cache for thumbnails
				videoPageSize := 39
				vPageStart := ((p-1)*pageSize)/videoPageSize + 1
				vPageEnd := (p*pageSize)/videoPageSize + 1
				for v := vPageEnd; v >= vPageStart; v-- {
					_, _ = c.GetVideoEpisodes(bgCtx, animeID, v)
				}

				select {
				case <-bgCtx.Done():
					return
				case <-time.After(800 * time.Millisecond):
				}
			}
		}()
	}

	return allEpisodes, nil
}


	totalEpisodes := anime.Episodes
	if totalEpisodes <= 0 {
		// Fallback to simple page 1 fetch if count is unknown
		resp, err := c.GetEpisodes(ctx, animeID, 1)
		if err != nil {
			return nil, err
		}
		return resp.Data, nil
	}

	// Jikan /episodes returns 100 per page
	lastPage := (totalEpisodes + 99) / 100
	var allEpisodes []Episode
	
	// Fetch last page first (to get most recent episodes immediately)
	lastResp, err := c.GetEpisodes(ctx, animeID, lastPage)
	if err == nil {
		allEpisodes = append(allEpisodes, lastResp.Data...)
	}

	// Fetch first page
	if lastPage > 1 {
		firstResp, err := c.GetEpisodes(ctx, animeID, 1)
		if err == nil {
			allEpisodes = append(allEpisodes, firstResp.Data...)
		}
	}

	// Background fetching for intermediate pages (Working BACKWARDS)
	if lastPage > 2 {
		go func() {
			bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()

			// Count backwards from the second-to-last page to page 2
			for p := lastPage - 1; p >= 2; p-- {
				_, _ = c.GetEpisodes(bgCtx, animeID, p)

				select {
				case <-bgCtx.Done():
					return
				case <-time.After(600 * time.Millisecond): // Slightly slower to be safe
				}
			}
		}()
	}

	return allEpisodes, nil
}
