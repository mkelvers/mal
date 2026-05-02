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
	URL       string         `json:"url,omitempty"`
	Referer   string         `json:"referer,omitempty"`
	Token     string         `json:"token"`
	Subtitles []SubtitleItem `json:"subtitles"`
}

type SubtitleItem struct {
	Lang    string `json:"lang"`
	URL     string `json:"url,omitempty"`
	Referer string `json:"referer,omitempty"`
	Token   string `json:"token"`
}

type SkipSegment struct {
	Type  string  `json:"type"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

type WatchPageData struct {
	MalID             int
	Title             string
	CurrentEpisode    string
	StartTimeSeconds  float64
	CurrentStatus     string
	InitialMode       string
	AvailableModes    []string
	ModeSources       map[string]ModeSource
	Segments          []SkipSegment
	FallbackEpisodes  map[string]int
}
