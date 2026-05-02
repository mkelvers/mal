package jikan

import (
	"fmt"
	"strings"
)

type SearchResult struct {
	Animes      []Anime
	HasNextPage bool
}

type TopAnimeResult struct {
	Animes      []Anime
	HasNextPage bool
}

type StudioAnimeResult struct {
	Animes      []Anime
	HasNextPage bool
	StudioName  string
}

type NamedEntity struct {
	MalID int    `json:"mal_id"`
	Name  string `json:"name"`
}

type Aired struct {
	From   string `json:"from"`
	To     string `json:"to"`
	String string `json:"string"`
}

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
	Rank         int           `json:"rank"`
	Popularity   int           `json:"popularity"`
	Status       string        `json:"status"`
	Airing       bool          `json:"airing"`
	Episodes     int           `json:"episodes"`
	Score        float64       `json:"score"`
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
	Relations []JikanRelationGroup `json:"relations"`
}

func (a Anime) ImageURL() string {
	return a.Images.Webp.LargeImageURL
}

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

func (a Anime) Premiered() string {
	if a.Season != "" && a.Year > 0 {
		return fmt.Sprintf("%s %d", seasonLabel(a.Season), a.Year)
	}
	return ""
}

func seasonLabel(season string) string {
	switch strings.ToLower(season) {
	case "winter":
		return "Winter"
	case "spring":
		return "Spring"
	case "summer":
		return "Summer"
	case "fall", "autumn":
		return "Fall"
	default:
		if season == "" {
			return ""
		}
		return strings.ToUpper(season[:1]) + strings.ToLower(season[1:])
	}
}

type AnimeResponse struct {
	Data Anime `json:"data"`
}

type Genre struct {
	MalID int    `json:"mal_id"`
	Name  string `json:"name"`
}

type GenresResponse struct {
	Data []Genre `json:"data"`
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

type EpisodeImages struct {
	Jpg struct {
		ImageURL string `json:"image_url"`
	} `json:"jpg"`
}

type Episode struct {
	MalID   int            `json:"mal_id"`
	Title   string         `json:"title"`
	Episode string         `json:"episode"`
	Filler  bool           `json:"filler"`
	Recap   bool           `json:"recap"`
	Images  *EpisodeImages `json:"images,omitempty"`
}

type EpisodesResponse struct {
	Data       []Episode  `json:"data"`
	Pagination Pagination `json:"pagination"`
}

type EpisodeResponse struct {
	Data Episode `json:"data"`
}

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
	Relation  string
	IsCurrent bool
	IsExtra   bool
}

func (a Anime) DisplayTitle() string {
	if a.TitleEnglish != "" {
		return a.TitleEnglish
	}
	if a.TitleJapanese != "" {
		return a.TitleJapanese
	}
	return a.Title
}
