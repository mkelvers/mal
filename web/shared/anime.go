package shared

import (
	"mal/integrations/jikan"
	"strings"
)

func JoinNames(entities []jikan.NamedEntity) string {
	names := make([]string, len(entities))
	for i, e := range entities {
		names[i] = e.Name
	}
	return strings.Join(names, ", ")
}

func JoinStreamingNames(anime jikan.Anime) string {
	names := make([]string, len(anime.Streaming))
	for i, s := range anime.Streaming {
		names[i] = s.Name
	}
	return strings.Join(names, ", ")
}

func WatchTargetEpisode(currentStatus string, currentEpisode int) int {
	if currentStatus != "" && currentEpisode > 0 {
		return currentEpisode
	}
	return 1
}

func HasExtraSidebarDetails(anime jikan.Anime) bool {
	return anime.TitleJapanese != "" ||
		len(anime.TitleSynonyms) > 0 ||
		len(anime.Studios) > 0 ||
		len(anime.Producers) > 0 ||
		anime.Source != "" ||
		len(anime.Demographics) > 0 ||
		len(anime.Themes) > 0 ||
		anime.Broadcast.String != "" ||
		len(anime.Streaming) > 0
}
