package watchlist

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"mal/internal/database"
)

type Service struct {
	db database.Querier
}

func NewService(db database.Querier) *Service {
	return &Service{db: db}
}

type AddRequest struct {
	AnimeID       int64
	TitleOriginal string
	TitleEnglish  string
	TitleJapanese string
	ImageURL      string
	Status        string
	Airing        bool
}

func (s *Service) AddEntry(ctx context.Context, userID string, req AddRequest) error {
	if req.AnimeID == 0 {
		return fmt.Errorf("invalid anime ID")
	}

	_, err := s.db.UpsertAnime(ctx, database.UpsertAnimeParams{
		ID:            req.AnimeID,
		TitleOriginal: req.TitleOriginal,
		TitleEnglish:  sql.NullString{String: req.TitleEnglish, Valid: req.TitleEnglish != ""},
		TitleJapanese: sql.NullString{String: req.TitleJapanese, Valid: req.TitleJapanese != ""},
		ImageUrl:      req.ImageURL,
		Airing:        sql.NullBool{Bool: req.Airing, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("failed to save anime reference: %w", err)
	}

	entryID := uuid.New().String()
	_, err = s.db.UpsertWatchListEntry(ctx, database.UpsertWatchListEntryParams{
		ID:      entryID,
		UserID:  userID,
		AnimeID: req.AnimeID,
		Status:  req.Status,
	})
	if err != nil {
		return fmt.Errorf("failed to update watchlist: %w", err)
	}

	return nil
}

func (s *Service) RemoveEntry(ctx context.Context, userID string, animeID int64) (database.Anime, error) {
	if animeID == 0 {
		return database.Anime{}, fmt.Errorf("invalid anime ID")
	}

	anime, err := s.db.GetAnime(ctx, animeID)
	if err != nil {
		return database.Anime{}, fmt.Errorf("anime not found: %w", err)
	}

	err = s.db.DeleteWatchListEntry(ctx, database.DeleteWatchListEntryParams{
		UserID:  userID,
		AnimeID: animeID,
	})
	if err != nil {
		return database.Anime{}, fmt.Errorf("failed to delete from watchlist: %w", err)
	}

	return anime, nil
}

func (s *Service) GetUserWatchlist(ctx context.Context, userID string) ([]database.GetUserWatchListRow, error) {
	entries, err := s.db.GetUserWatchList(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch watchlist: %w", err)
	}
	return entries, nil
}

type ExportEntry struct {
	AnimeID   int64  `json:"anime_id"`
	Title     string `json:"title"`
	ImageURL  string `json:"image_url"`
	Status    string `json:"status"`
	UpdatedAt string `json:"updated_at"`
}

type ExportData struct {
	ExportedAt string        `json:"exported_at"`
	Entries    []ExportEntry `json:"entries"`
}

// displayTitle returns the best available title
func displayTitle(e database.GetUserWatchListRow) string {
	if e.TitleEnglish.Valid && e.TitleEnglish.String != "" {
		return e.TitleEnglish.String
	}
	if e.TitleJapanese.Valid && e.TitleJapanese.String != "" {
		return e.TitleJapanese.String
	}
	return e.TitleOriginal
}

func (s *Service) Export(ctx context.Context, userID string) (ExportData, error) {
	entries, err := s.GetUserWatchlist(ctx, userID)
	if err != nil {
		return ExportData{}, err
	}

	export := ExportData{
		ExportedAt: time.Now().UTC().Format(time.RFC3339),
		Entries:    make([]ExportEntry, len(entries)),
	}

	for i, entry := range entries {
		export.Entries[i] = ExportEntry{
			AnimeID:   entry.AnimeID,
			Title:     displayTitle(entry),
			ImageURL:  entry.ImageUrl,
			Status:    entry.Status,
			UpdatedAt: entry.UpdatedAt.Format(time.RFC3339),
		}
	}

	return export, nil
}

func (s *Service) Import(ctx context.Context, userID string, export ExportData) (int, error) {
	imported := 0
	for _, entry := range export.Entries {
		_, err := s.db.UpsertAnime(ctx, database.UpsertAnimeParams{
			ID:            entry.AnimeID,
			TitleOriginal: entry.Title,
			TitleEnglish:  sql.NullString{},
			TitleJapanese: sql.NullString{},
			ImageUrl:      entry.ImageURL,
		})
		if err != nil {
			continue // skip failures and keep going
		}

		_, err = s.db.UpsertWatchListEntry(ctx, database.UpsertWatchListEntryParams{
			ID:      uuid.New().String(),
			UserID:  userID,
			AnimeID: entry.AnimeID,
			Status:  entry.Status,
		})
		if err != nil {
			continue
		}
		imported++
	}
	return imported, nil
}
