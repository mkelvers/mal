package database

import "database/sql"

func DisplayTitle(titleEnglish, titleJapanese sql.NullString, titleOriginal string) string {
	if titleEnglish.Valid && titleEnglish.String != "" {
		return titleEnglish.String
	}
	if titleJapanese.Valid && titleJapanese.String != "" {
		return titleJapanese.String
	}
	return titleOriginal
}

func (r GetUserWatchListRow) DisplayTitle() string {
	return DisplayTitle(r.TitleEnglish, r.TitleJapanese, r.TitleOriginal)
}
