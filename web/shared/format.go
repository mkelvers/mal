package shared

import (
	"fmt"
	"net/url"
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
func WatchlistURL(view string, status string, sortBy string, sortOrder string) string {
	return fmt.Sprintf("/watchlist?view=%s&status=%s&sort=%s&order=%s", view, status, sortBy, sortOrder)
}

// AnimeURL builds the anime detail URL
func AnimeURL(animeID int) string {
	return fmt.Sprintf("/anime/%d", animeID)
}
