package playback

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"mal/internal/db"
)

func (s *Service) SaveProgress(ctx context.Context, userID string, animeID int64, episode int, timeSeconds float64, animeSeed *database.UpsertAnimeParams) error {
	if strings.TrimSpace(userID) == "" || animeID <= 0 || episode <= 0 {
		return errors.New("invalid save progress input")
	}

	txQueries, tx, err := database.BeginTx(ctx, s.sqlDB)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	if animeSeed != nil {
		if _, err := txQueries.UpsertAnime(ctx, *animeSeed); err != nil {
			return fmt.Errorf("failed to save anime reference: %w", err)
		}
	}

	watchListEntry, watchListErr := txQueries.GetWatchListEntry(ctx, database.GetWatchListEntryParams{
		UserID:  userID,
		AnimeID: animeID,
	})
	if watchListErr != nil && !errors.Is(watchListErr, sql.ErrNoRows) {
		return fmt.Errorf("failed to load watchlist entry: %w", watchListErr)
	}

	if err := txQueries.SaveWatchProgress(ctx, database.SaveWatchProgressParams{
		CurrentEpisode:     sql.NullInt64{Int64: int64(episode), Valid: true},
		CurrentTimeSeconds: timeSeconds,
		UserID:             userID,
		AnimeID:            animeID,
	}); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to save watchlist progress: %w", err)
	}

	if _, err := txQueries.UpsertContinueWatchingEntry(ctx, database.UpsertContinueWatchingEntryParams{
		ID:                 uuid.New().String(),
		UserID:             userID,
		AnimeID:            animeID,
		CurrentEpisode:     sql.NullInt64{Int64: int64(episode), Valid: true},
		CurrentTimeSeconds: timeSeconds,
	}); err != nil {
		return fmt.Errorf("failed to upsert continue entry: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit save progress transaction: %w", err)
	}

	return nil
}

func (s *Service) CompleteAnime(ctx context.Context, userID string, animeID int64, episode int, animeSeed *database.UpsertAnimeParams) error {
	if strings.TrimSpace(userID) == "" || animeID <= 0 || episode <= 0 {
		return errors.New("invalid complete anime input")
	}

	txQueries, tx, err := database.BeginTx(ctx, s.sqlDB)
	if err != nil {
		return err
	}

	defer tx.Rollback()

	watchListEntry, watchListErr := txQueries.GetWatchListEntry(ctx, database.GetWatchListEntryParams{
		UserID:  userID,
		AnimeID: animeID,
	})
	if watchListErr != nil && !errors.Is(watchListErr, sql.ErrNoRows) {
		return fmt.Errorf("failed to load watchlist entry: %w", watchListErr)
	}

	alreadyCompleted := watchListErr == nil && watchListEntry.Status == "completed"

	if !alreadyCompleted {
		if animeSeed != nil {
			if _, err := txQueries.UpsertAnime(ctx, *animeSeed); err != nil {
				return fmt.Errorf("failed to save anime reference: %w", err)
			}
		}

		if _, err := txQueries.UpsertWatchListEntry(ctx, database.UpsertWatchListEntryParams{
			ID:                 uuid.New().String(),
			UserID:             userID,
			AnimeID:            animeID,
			Status:             "completed",
			CurrentEpisode:     sql.NullInt64{Int64: int64(episode), Valid: true},
			CurrentTimeSeconds: 0,
		}); err != nil {
			return fmt.Errorf("failed to mark watchlist as completed: %w", err)
		}
	}

	if err := txQueries.DeleteContinueWatchingEntry(ctx, database.DeleteContinueWatchingEntryParams{
		UserID:  userID,
		AnimeID: animeID,
	}); err != nil {
		return fmt.Errorf("failed to clear continue entry: %w", err)
	}

	if err := txQueries.SaveWatchProgress(ctx, database.SaveWatchProgressParams{
		CurrentEpisode:     sql.NullInt64{Int64: int64(episode), Valid: true},
		CurrentTimeSeconds: 0,
		UserID:             userID,
		AnimeID:            animeID,
	}); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed to reset watch progress: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit complete anime transaction: %w", err)
	}

	return nil
}
