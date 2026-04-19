package watchlist

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"mal/internal/database"
)

type Service struct {
	db    database.Querier
	sqlDB *sql.DB
}

var (
	ErrInvalidAnimeID = errors.New("invalid anime ID")
	ErrInvalidStatus  = errors.New("invalid watchlist status")
)

var validStatuses = map[string]struct{}{
	"watching":      {},
	"completed":     {},
	"on_hold":       {},
	"dropped":       {},
	"plan_to_watch": {},
}

func NewService(db database.Querier, sqlDB *sql.DB) *Service {
	return &Service{db: db, sqlDB: sqlDB}
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
	if req.AnimeID <= 0 {
		return ErrInvalidAnimeID
	}

	if _, ok := validStatuses[req.Status]; !ok {
		return ErrInvalidStatus
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
		ID:                 entryID,
		UserID:             userID,
		AnimeID:            req.AnimeID,
		Status:             req.Status,
		CurrentEpisode:     sql.NullInt64{Int64: 0, Valid: false},
		CurrentTimeSeconds: 0,
	})
	if err != nil {
		return fmt.Errorf("failed to update watchlist: %w", err)
	}

	return nil
}

func (s *Service) RemoveEntry(ctx context.Context, userID string, animeID int64) (database.Anime, error) {
	if animeID <= 0 {
		return database.Anime{}, ErrInvalidAnimeID
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

func (s *Service) GetContinueWatching(ctx context.Context, userID string) ([]database.GetContinueWatchingEntriesRow, error) {
	if strings.TrimSpace(userID) == "" {
		return nil, errors.New("invalid user id")
	}

	entries, err := s.db.GetContinueWatchingEntries(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch continue watching: %w", err)
	}

	return entries, nil
}

func (s *Service) DeleteContinueWatching(ctx context.Context, userID string, animeID int64) error {
	if strings.TrimSpace(userID) == "" {
		return errors.New("invalid user id")
	}

	if animeID <= 0 {
		return ErrInvalidAnimeID
	}

	params := database.DeleteContinueWatchingEntryParams{
		UserID:  userID,
		AnimeID: animeID,
	}

	clearProgress := database.SaveWatchProgressParams{
		CurrentEpisode:     sql.NullInt64{Valid: false},
		CurrentTimeSeconds: 0,
		UserID:             userID,
		AnimeID:            animeID,
	}

	if s.sqlDB == nil {
		if err := s.db.DeleteContinueWatchingEntry(ctx, params); err != nil {
			return fmt.Errorf("failed to delete continue watching entry: %w", err)
		}
		return s.db.SaveWatchProgress(ctx, clearProgress)
	}

	tx, err := s.sqlDB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	txQueries := database.New(tx)
	if err := txQueries.DeleteContinueWatchingEntry(ctx, params); err != nil {
		return fmt.Errorf("failed to delete continue watching entry: %w", err)
	}
	if err := txQueries.SaveWatchProgress(ctx, clearProgress); err != nil {
		return fmt.Errorf("failed to clear watchlist progress: %w", err)
	}

	return tx.Commit()
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
			Title:     database.DisplayTitle(entry.TitleEnglish, entry.TitleJapanese, entry.TitleOriginal),
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
			ID:                 uuid.New().String(),
			UserID:             userID,
			AnimeID:            entry.AnimeID,
			Status:             entry.Status,
			CurrentEpisode:     sql.NullInt64{Int64: 0, Valid: false},
			CurrentTimeSeconds: 0,
		})
		if err != nil {
			continue
		}
		imported++
	}
	return imported, nil
}
