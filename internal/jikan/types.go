package jikan

import "fmt"

type SearchResult struct {
	Animes      []Anime
	HasNextPage bool
}

type TopAnimeResult struct {
	Animes      []Anime
	HasNextPage bool
}

// NamedEntity represents genres, studios, producers, etc.
type NamedEntity struct {
	MalID int    `json:"mal_id"`
	Name  string `json:"name"`
}

// Aired represents the airing date range
type Aired struct {
	From   string `json:"from"`
	To     string `json:"to"`
	String string `json:"string"`
}

// Anime struct matching the Jikan v4 API structure
type Anime struct {
	MalID         int      `json:"mal_id"`
	Title         string   `json:"title"`
	TitleEnglish  string   `json:"title_english"`
	TitleJapanese string   `json:"title_japanese"`
	TitleSynonyms []string `json:"title_synonyms"`
	Images        struct {
		Jpg struct {
			LargeImageURL string `json:"large_image_url"`
		} `json:"jpg"`
		Webp struct {
			LargeImageURL string `json:"large_image_url"`
		} `json:"webp"`
	} `json:"images"`
	Synopsis     string        `json:"synopsis"`
	Score        float64       `json:"score"`
	ScoredBy     int           `json:"scored_by"`
	Rank         int           `json:"rank"`
	Popularity   int           `json:"popularity"`
	Status       string        `json:"status"`
	Airing       bool          `json:"airing"`
	Episodes     int           `json:"episodes"`
	Season       string        `json:"season"`
	Year         int           `json:"year"`
	Type         string        `json:"type"`
	Rating       string        `json:"rating"`
	Duration     string        `json:"duration"`
	Aired        Aired         `json:"aired"`
	Genres       []NamedEntity `json:"genres"`
	Studios      []NamedEntity `json:"studios"`
	Producers    []NamedEntity `json:"producers"`
	Themes       []NamedEntity `json:"themes"`
	Source       string        `json:"source"`
	Demographics []NamedEntity `json:"demographics"`
	Broadcast    struct {
		Day      string `json:"day"`
		Time     string `json:"time"`
		Timezone string `json:"timezone"`
		String   string `json:"string"`
	} `json:"broadcast"`
	Streaming []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	} `json:"streaming"`
}

// ImageURL returns the webp large image URL
func (a Anime) ImageURL() string {
	return a.Images.Webp.LargeImageURL
}

// ShortRating returns abbreviated rating (e.g., "PG-13" from "PG-13 - Teens 13 or older")
func (a Anime) ShortRating() string {
	if a.Rating == "" {
		return ""
	}
	// Rating format: "PG-13 - Teens 13 or older"
	for i, c := range a.Rating {
		if c == ' ' && i > 0 {
			return a.Rating[:i]
		}
	}
	return a.Rating
}

// ShortDuration returns abbreviated duration (e.g., "23m" from "23 min per ep")
func (a Anime) ShortDuration() string {
	if a.Duration == "" {
		return ""
	}
	// Duration format: "23 min per ep" or "1 hr 30 min"
	var num string
	for _, c := range a.Duration {
		if c >= '0' && c <= '9' {
			num += string(c)
		} else if c == ' ' && num != "" {
			break
		}
	}
	if num != "" {
		return num + "m"
	}
	return a.Duration
}

// Premiered returns season + year (e.g., "Fall 2002")
func (a Anime) Premiered() string {
	if a.Season != "" && a.Year > 0 {
		return fmt.Sprintf("%s %d", a.Season, a.Year)
	}
	return ""
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

// Episode represents a single anime episode from Jikan API
type Episode struct {
	MalID    int    `json:"mal_id"`
	Title    string `json:"title"`
	TitleJP  string `json:"title_japanese"`
	TitleRom string `json:"title_romanji"`
	Aired    string `json:"aired"`
	Filler   bool   `json:"filler"`
	Recap    bool   `json:"recap"`
}

type EpisodesResponse struct {
	Data       []Episode  `json:"data"`
	Pagination Pagination `json:"pagination"`
}

type EpisodesResult struct {
	Episodes    []Episode
	HasNextPage bool
}
