package anime

import (
	"context"
	"fmt"

	"mal/internal/database"
	"mal/internal/jikan"
	"mal/internal/templates"
)

type Service struct {
	jikanClient *jikan.Client
	db          database.Querier
}

func NewService(jikanClient *jikan.Client, db database.Querier) *Service {
	return &Service{
		jikanClient: jikanClient,
		db:          db,
	}
}

func (s *Service) Search(ctx context.Context, query string, page int) (jikan.SearchResult, error) {
	return s.jikanClient.Search(ctx, query, page)
}

func (s *Service) GetTopAnime(ctx context.Context, page int) (jikan.TopAnimeResult, error) {
	return s.jikanClient.GetTopAnime(ctx, page)
}

func (s *Service) GetAiringAnime(ctx context.Context, page int) (jikan.TopAnimeResult, error) {
	return s.jikanClient.GetSeasonsNow(ctx, page)
}

func (s *Service) GetUpcomingAnime(ctx context.Context, page int) (jikan.TopAnimeResult, error) {
	return s.jikanClient.GetSeasonsUpcoming(ctx, page)
}

func (s *Service) GetAnimeDetails(ctx context.Context, id int, userID string) (jikan.Anime, string, error) {
	anime, err := s.jikanClient.GetAnimeByID(ctx, id)
	if err != nil {
		return jikan.Anime{}, "", fmt.Errorf("failed to fetch anime details: %w", err)
	}

	currentStatus := ""
	if userID != "" {
		entry, err := s.db.GetWatchListEntry(ctx, database.GetWatchListEntryParams{
			UserID:  userID,
			AnimeID: int64(id),
		})
		if err == nil {
			currentStatus = entry.Status
		}
	}

	return anime, currentStatus, nil
}

func (s *Service) GetRelations(ctx context.Context, id int) ([]jikan.RelationEntry, error) {
	return s.jikanClient.GetFullRelations(ctx, id)
}

func (s *Service) GetSchedule(ctx context.Context, day string) (jikan.ScheduleResult, error) {
	return s.jikanClient.GetSchedule(ctx, day)
}

func (s *Service) GetRecommendations(ctx context.Context, animeID int, limit int) ([]jikan.Anime, error) {
	return s.jikanClient.GetRecommendations(ctx, animeID, limit)
}

func (s *Service) GetWatchingAnime(ctx context.Context, userID string) ([]templates.WatchingAnimeWithDetails, error) {
	rows, err := s.db.GetWatchingAnime(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get watching anime: %w", err)
	}

	var result []templates.WatchingAnimeWithDetails
	for _, row := range rows {
		anime, err := s.jikanClient.GetAnimeByID(ctx, int(row.AnimeID))
		if err != nil {
			// Instead of skipping, we still append it, but without the extra Jikan details
			// This prevents anime from vanishing from the watchlist when Jikan rate limits us.
			anime = jikan.Anime{}
		}
		result = append(result, templates.WatchingAnimeWithDetails{
			Entry: row,
			Anime: anime,
		})
	}

	return result, nil
}

func (s *Service) GetUpcomingSeasons(ctx context.Context, userID string) ([]database.GetUpcomingSeasonsRow, error) {
	rows, err := s.db.GetUpcomingSeasons(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get upcoming seasons: %w", err)
	}

	// Deduplicate by related anime ID
	// Because of the recursive query, multiple prequels can point to the same upcoming season
	seen := make(map[int64]bool)
	var deduped []database.GetUpcomingSeasonsRow
	for _, row := range rows {
		if !seen[row.ID] {
			seen[row.ID] = true
			deduped = append(deduped, row)
		}
	}

	return deduped, nil
}
