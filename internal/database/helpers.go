package database

// DisplayTitle returns the English title if available, otherwise Japanese, otherwise original
func (r GetUserWatchListRow) DisplayTitle() string {
	if r.TitleEnglish.Valid && r.TitleEnglish.String != "" {
		return r.TitleEnglish.String
	}
	if r.TitleJapanese.Valid && r.TitleJapanese.String != "" {
		return r.TitleJapanese.String
	}
	return r.TitleOriginal
}
