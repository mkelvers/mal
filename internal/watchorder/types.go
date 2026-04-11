package watchorder

type WatchOrderEntry struct {
	ID       int    `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	TitleAlt string `json:"title_alt,omitempty"`
}
