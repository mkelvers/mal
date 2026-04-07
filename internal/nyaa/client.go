package nyaa

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

type Client struct {
	httpClient  *http.Client
	baseURL     string
	cache       *expirable.LRU[string, []Torrent]
	magnetCache *expirable.LRU[string, string]
}

type Torrent struct {
	Title    string `json:"title"`
	Magnet   string `json:"magnet"`
	Size     string `json:"size"`
	Seeders  int    `json:"seeders"`
	Leechers int    `json:"leechers"`
	Date     string `json:"date"`
	Episode  int    `json:"episode"`
	ViewURL  string `json:"view_url"`
}

// RSS feed structures
type rssResponse struct {
	XMLName xml.Name   `xml:"rss"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Items []rssItem `xml:"item"`
}

type rssItem struct {
	Title    string `xml:"title"`
	Link     string `xml:"link"`
	GUID     string `xml:"guid"`
	PubDate  string `xml:"pubDate"`
	Seeders  string `xml:"seeders"`
	Leechers string `xml:"leechers"`
	Size     string `xml:"size"`
}

func NewClient() *Client {
	cache := expirable.NewLRU[string, []Torrent](200, nil, time.Minute*15)
	magnetCache := expirable.NewLRU[string, string](500, nil, time.Hour*24)
	return &Client{
		httpClient:  &http.Client{Timeout: 15 * time.Second},
		baseURL:     "https://nyaa.si",
		cache:       cache,
		magnetCache: magnetCache,
	}
}

// SearchAnime searches for anime torrents on nyaa.si
// category 1_2 = Anime - English-translated
func (c *Client) SearchAnime(query string) ([]Torrent, error) {
	if cached, ok := c.cache.Get(query); ok {
		return cached, nil
	}

	// Build search URL with RSS feed
	params := url.Values{}
	params.Set("f", "0")   // filter: no filter
	params.Set("c", "1_2") // category: Anime - English-translated
	params.Set("q", query)
	params.Set("s", "seeders") // sort by seeders
	params.Set("o", "desc")    // descending order
	params.Set("page", "rss")  // RSS format

	reqURL := fmt.Sprintf("%s/?%s", c.baseURL, params.Encode())

	resp, err := c.httpClient.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("nyaa request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("nyaa returned status %d", resp.StatusCode)
	}

	var rss rssResponse
	if err := xml.NewDecoder(resp.Body).Decode(&rss); err != nil {
		return nil, fmt.Errorf("failed to parse nyaa rss: %w", err)
	}

	torrents := make([]Torrent, 0, len(rss.Channel.Items))
	for _, item := range rss.Channel.Items {
		seeders, _ := strconv.Atoi(item.Seeders)
		leechers, _ := strconv.Atoi(item.Leechers)

		// Extract torrent ID from download link
		// Link format: https://nyaa.si/download/1234567.torrent
		viewURL := extractViewURL(item.Link)

		t := Torrent{
			Title:    item.Title,
			Magnet:   "", // Will be fetched on demand
			Size:     item.Size,
			Seeders:  seeders,
			Leechers: leechers,
			Date:     item.PubDate,
			Episode:  parseEpisodeNumber(item.Title),
			ViewURL:  viewURL,
		}

		// Check if GUID is already a magnet link
		if strings.HasPrefix(item.GUID, "magnet:") {
			t.Magnet = item.GUID
		}

		torrents = append(torrents, t)
	}

	// Fetch magnets for top results (limit to avoid rate limiting)
	for i := range torrents {
		if i >= 20 {
			break
		}
		if torrents[i].Magnet == "" && torrents[i].ViewURL != "" {
			magnet, err := c.fetchMagnet(torrents[i].ViewURL)
			if err == nil {
				torrents[i].Magnet = magnet
			}
		}
	}

	c.cache.Add(query, torrents)
	return torrents, nil
}

// fetchMagnet scrapes the nyaa view page to get the magnet link
func (c *Client) fetchMagnet(viewURL string) (string, error) {
	if cached, ok := c.magnetCache.Get(viewURL); ok {
		return cached, nil
	}

	resp, err := c.httpClient.Get(viewURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("nyaa returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	// Find magnet link in page
	// Pattern: href="magnet:?xt=urn:btih:..."
	magnetRe := regexp.MustCompile(`href="(magnet:\?xt=urn:btih:[^"]+)"`)
	matches := magnetRe.FindSubmatch(body)
	if len(matches) < 2 {
		return "", fmt.Errorf("magnet link not found")
	}

	magnet := string(matches[1])
	c.magnetCache.Add(viewURL, magnet)
	return magnet, nil
}

// extractViewURL converts download URL to view URL
func extractViewURL(downloadURL string) string {
	// https://nyaa.si/download/1234567.torrent -> https://nyaa.si/view/1234567
	re := regexp.MustCompile(`/download/(\d+)\.torrent`)
	matches := re.FindStringSubmatch(downloadURL)
	if len(matches) < 2 {
		return ""
	}
	return fmt.Sprintf("https://nyaa.si/view/%s", matches[1])
}

// SearchEpisode searches for a specific episode of an anime
func (c *Client) SearchEpisode(animeTitle string, episode int) ([]Torrent, error) {
	// Format episode number with leading zero for single digits
	epStr := fmt.Sprintf("%02d", episode)
	query := fmt.Sprintf("%s %s", animeTitle, epStr)

	torrents, err := c.SearchAnime(query)
	if err != nil {
		return nil, err
	}

	// Filter to torrents that match the episode number
	var filtered []Torrent
	for _, t := range torrents {
		if t.Episode == episode || t.Episode == 0 {
			// Episode 0 means we couldn't parse it, include anyway
			filtered = append(filtered, t)
		}
	}

	// If no filtered results, return all (search might be specific enough)
	if len(filtered) == 0 {
		return torrents, nil
	}
	return filtered, nil
}

// parseEpisodeNumber tries to extract episode number from torrent title
func parseEpisodeNumber(title string) int {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)[-–]\s*(\d{1,4})(?:v\d)?(?:\s|\[|$)`),    // - 01 or - 01v2
		regexp.MustCompile(`(?i)\bE(\d{1,4})(?:v\d)?(?:\s|\[|$)`),        // E01
		regexp.MustCompile(`(?i)S\d{1,2}E(\d{1,4})(?:v\d)?(?:\s|\[|$)`),  // S01E01
		regexp.MustCompile(`(?i)Episode\s*(\d{1,4})(?:v\d)?(?:\s|\[|$)`), // Episode 01
		regexp.MustCompile(`(?i)\s(\d{2,4})(?:v\d)?\s*[\[\(]`),           // 01 [quality]
	}

	for _, re := range patterns {
		if matches := re.FindStringSubmatch(title); len(matches) > 1 {
			if ep, err := strconv.Atoi(matches[1]); err == nil {
				return ep
			}
		}
	}

	return 0
}

// FilterByQuality returns torrents matching the quality preference
func FilterByQuality(torrents []Torrent, quality string) []Torrent {
	if quality == "" {
		return torrents
	}

	var filtered []Torrent
	qualityLower := strings.ToLower(quality)

	for _, t := range torrents {
		titleLower := strings.ToLower(t.Title)
		if strings.Contains(titleLower, qualityLower) {
			filtered = append(filtered, t)
		}
	}

	if len(filtered) == 0 {
		return torrents
	}
	return filtered
}

// BestTorrent returns the torrent with the most seeders that has a magnet
func BestTorrent(torrents []Torrent) *Torrent {
	if len(torrents) == 0 {
		return nil
	}

	var best *Torrent
	for i := range torrents {
		if torrents[i].Magnet == "" {
			continue
		}
		if best == nil || torrents[i].Seeders > best.Seeders {
			best = &torrents[i]
		}
	}

	// Fallback to first with magnet
	if best == nil {
		for i := range torrents {
			if torrents[i].Magnet != "" {
				return &torrents[i]
			}
		}
	}

	return best
}
