package jikan

// Anime struct matching the Jikan v4 API structure
type Anime struct {
	MalID         int    `json:"mal_id"`
	Title         string `json:"title"`
	TitleEnglish  string `json:"title_english"`
	TitleJapanese string `json:"title_japanese"`
	Images        struct {
		Webp struct {
			LargeImageURL string `json:"large_image_url"`
		} `json:"webp"`
	} `json:"images"`
	Synopsis   string  `json:"synopsis"`
	Score      float64 `json:"score"`
	ScoredBy   int     `json:"scored_by"`
	Rank       int     `json:"rank"`
	Popularity int     `json:"popularity"`
	Status     string  `json:"status"`
	Episodes   int     `json:"episodes"`
	Season     string  `json:"season"`
	Year       int     `json:"year"`
	Type       string  `json:"type"`
}

type AnimeResponse struct {
	Data Anime `json:"data"`
}

type SearchResponse struct {
	Data       []Anime    `json:"data"`
	Pagination Pagination `json:"pagination"`
}

type Pagination struct {
	HasNextPage bool `json:"has_next_page"`
}

type TopAnimeResponse struct {
	Data       []Anime    `json:"data"`
	Pagination Pagination `json:"pagination"`
}

// Relation Types
type JikanRelationEntry struct {
	MalID int    `json:"mal_id"`
	Type  string `json:"type"`
	Name  string `json:"name"`
	URL   string `json:"url"`
}

type JikanRelationGroup struct {
	Relation string               `json:"relation"`
	Entry    []JikanRelationEntry `json:"entry"`
}

type JikanRelationsResponse struct {
	Data []JikanRelationGroup `json:"data"`
}

type RelationEntry struct {
	Anime     Anime
	IsCurrent bool
}

// DisplayTitle prefers English, falls back to Japanese, then standard Title
func (a Anime) DisplayTitle() string {
	if a.TitleEnglish != "" {
		return a.TitleEnglish
	}
	if a.TitleJapanese != "" {
		return a.TitleJapanese
	}
	return a.Title
}
