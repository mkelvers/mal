package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

func NullStringOr(n sql.NullString, fallback string) string {
	if n.Valid && n.String != "" {
		return n.String
	}
	return fallback
}

func DisplayTitle(titleEnglish, titleJapanese sql.NullString, titleOriginal string) string {
	return NullStringOr(titleEnglish, NullStringOr(titleJapanese, titleOriginal))
}

func (r GetUserWatchListRow) DisplayTitle() string {
	return DisplayTitle(r.TitleEnglish, r.TitleJapanese, r.TitleOriginal)
}

func BoolPtr(b sql.NullBool) *bool {
	if !b.Valid {
		return nil
	}
	return &b.Bool
}

func BeginTx(ctx context.Context, db *sql.DB) (*Queries, *sql.Tx, error) {
	if db == nil {
		return nil, nil, errors.New("database unavailable")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to begin transaction: %w", err)
	}

	return New(tx), tx, nil
}
