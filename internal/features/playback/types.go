package playback

type StreamSource struct {
	URL       string
	Quality   string
	Provider  string
	Type      string
	Referer   string
	Subtitles []Subtitle
}

type Subtitle struct {
	Lang string
	URL  string
}

type ModeSource struct {
	URL       string         `json:"url"`
	Referer   string         `json:"referer"`
	Subtitles []SubtitleItem `json:"subtitles"`
}

type SubtitleItem struct {
	Lang    string `json:"lang"`
	URL     string `json:"url"`
	Referer string `json:"referer"`
}

type SkipSegment struct {
	Type  string  `json:"type"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

type WatchPageData struct {
	MalID            int
	Title            string
	CurrentEpisode   string
	StartTimeSeconds float64
	CurrentStatus    string
	InitialMode      string
	AvailableModes   []string
	ModeSources      map[string]ModeSource
	Segments         []SkipSegment
}
