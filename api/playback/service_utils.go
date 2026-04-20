package playback

import (
	"strings"
)

func toSubtitleItems(source StreamSource) []SubtitleItem {
	items := make([]SubtitleItem, 0, len(source.Subtitles))
	for _, subtitle := range source.Subtitles {
		targetURL := strings.TrimSpace(subtitle.URL)
		if targetURL == "" {
			continue
		}

		items = append(items, SubtitleItem{
			Lang:    strings.TrimSpace(subtitle.Lang),
			URL:     targetURL,
			Referer: source.Referer,
		})
	}

	return items
}
