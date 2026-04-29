package shared

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
)

// WatchPageData holds the data needed for the watch page
type WatchPageData struct {
	MalID            int
	Title            string
	TitleEnglish     string
	TitleJapanese    string
	ImageURL         string
	Airing           bool
	CurrentEpisode   string
	TotalEpisodes    int
	StartTimeSeconds float64
	CurrentStatus    string
	InitialMode      string
	AvailableModes   []string
	ModeSources      map[string]ModeSource
	Segments         []SkipSegment
	EpisodeTitle     string
	EpisodeAired     string
	NextEpisodeTitle string
}

// ModeSource represents a stream source for a specific mode (dub/sub)
type ModeSource struct {
	Token     string         `json:"token"`
	Subtitles []SubtitleItem `json:"subtitles"`
}

// SubtitleItem represents a subtitle track
type SubtitleItem struct {
	Lang  string `json:"lang"`
	Token string `json:"token"`
}

// SkipSegment represents a skippable segment (intro/outro)
type SkipSegment struct {
	Type  string  `json:"type"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

func ModeToken(mode string, modeSources map[string]ModeSource) string {
	normalizedMode := mode
	if _, ok := modeSources[normalizedMode]; !ok {
		if _, ok := modeSources["dub"]; ok {
			normalizedMode = "dub"
		} else if _, ok := modeSources["sub"]; ok {
			normalizedMode = "sub"
		} else {
			for key := range modeSources {
				normalizedMode = key
				break
			}
		}
	}

	source, ok := modeSources[normalizedMode]
	if !ok {
		return ""
	}
	return source.Token
}

func ToJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		log.Printf("ToJSON error: %v", err)
		return "{}"
	}
	return string(b)
}

func EpisodeWithOffsetURL(animeID int, currentEpisode string, offset int) string {
	episodeID, err := strconv.Atoi(currentEpisode)
	if err != nil {
		episodeID = 1
	}
	nextEpisode := episodeID + offset
	if nextEpisode < 1 {
		nextEpisode = 1
	}
	return fmt.Sprintf("/watch/%d/%d", animeID, nextEpisode)
}

func CanGoPrevEpisode(currentEpisode string) bool {
	episodeID, err := strconv.Atoi(currentEpisode)
	if err != nil {
		return false
	}
	return episodeID > 1
}

func CanGoNextEpisode(currentEpisode string, totalEpisodes int) bool {
	if totalEpisodes <= 0 {
		return true
	}
	episodeID, err := strconv.Atoi(currentEpisode)
	if err != nil {
		return false
	}
	return episodeID < totalEpisodes
}

func ModeAvailable(modes []string, mode string) bool {
	for _, value := range modes {
		if value == mode {
			return true
		}
	}
	return false
}

func ModeButtonTitle(label string, enabled bool) string {
	if enabled {
		return label
	}
	return label + " unavailable for this episode"
}
