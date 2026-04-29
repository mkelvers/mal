package shared

import (
	"fmt"
	"net/url"
	"time"
)

// BuildStreamURL constructs a stream URL from mode and token
func BuildStreamURL(mode string, token string) string {
	if token == "" {
		return ""
	}
	return fmt.Sprintf("/watch/proxy/stream?mode=%s&token=%s", url.QueryEscape(mode), url.QueryEscape(token))
}

// FormatProgressTime formats seconds into MM:SS format
func FormatProgressTime(seconds float64) string {
	total := int(seconds)
	if total < 0 {
		total = 0
	}
	minutes := total / 60
	remainingSeconds := total % 60
	return fmt.Sprintf("%02d:%02d", minutes, remainingSeconds)
}

// FormatEstablishedDate extracts YYYY-MM-DD from ISO date string
func FormatEstablishedDate(date string) string {
	if len(date) >= 10 {
		return date[:10]
	}
	return date
}

// WatchlistURL builds the watchlist URL with query parameters
func WatchlistURL(status string, sortBy string, sortOrder string) string {
	return fmt.Sprintf("/watchlist?status=%s&sort=%s&order=%s", status, sortBy, sortOrder)
}

// AnimeURL builds the anime detail URL
func AnimeURL(animeID int) string {
	return fmt.Sprintf("/anime/%d", animeID)
}

// FormatEpisodeAired turns an ISO/RFC3339 date string into "Jan 2, 2006".
func FormatEpisodeAired(date string) string {
	if date == "" {
		return ""
	}

	layouts := []string{time.RFC3339, "2006-01-02T15:04:05", "2006-01-02"}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, date); err == nil {
			return t.Format("Jan 2, 2006")
		}
	}
	if len(date) >= 10 {
		if t, err := time.Parse("2006-01-02", date[:10]); err == nil {
			return t.Format("Jan 2, 2006")
		}
	}
	return ""
}

// EpisodeModeLabel mirrors svelte's "Subbed & Dubbed | Dubbed | Subtitled" logic.
func EpisodeModeLabel(availableModes []string, currentMode string) string {
	if len(availableModes) >= 2 {
		return "Subbed & Dubbed"
	}
	if currentMode == "dub" {
		return "Dubbed"
	}
	return "Subtitled"
}
