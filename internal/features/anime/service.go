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

func (s *Service) Search(query string, page int) (jikan.SearchResult, error) {
	return s.jikanClient.Search(query, page)
}

func (s *Service) GetTopAnime(page int) (jikan.TopAnimeResult, error) {
	return s.jikanClient.GetTopAnime(page)
}

func (s *Service) GetAiringAnime(page int) (jikan.TopAnimeResult, error) {
	return s.jikanClient.GetSeasonsNow(page)
}

func (s *Service) GetUpcomingAnime(page int) (jikan.TopAnimeResult, error) {
	return s.jikanClient.GetSeasonsUpcoming(page)
}

func (s *Service) GetAnimeDetails(ctx context.Context, id int, userID string) (jikan.Anime, string, error) {
	anime, err := s.jikanClient.GetAnimeByID(id)
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

func (s *Service) GetRelations(id int) []jikan.RelationEntry {
	return s.jikanClient.GetFullRelations(id)
}

func (s *Service) GetSchedule(day string) (jikan.ScheduleResult, error) {
	return s.jikanClient.GetSchedule(day)
}

func (s *Service) GetRecommendations(animeID int) ([]jikan.Anime, error) {
	return s.jikanClient.GetRecommendations(animeID)
}

func (s *Service) GetWatchingAnime(ctx context.Context, userID string) ([]templates.WatchingAnimeWithDetails, error) {
	rows, err := s.db.GetWatchingAnime(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get watching anime: %w", err)
	}

	var result []templates.WatchingAnimeWithDetails
	for _, row := range rows {
		anime, err := s.jikanClient.GetAnimeByID(int(row.AnimeID))
		if err != nil {
			// Skip if we can't fetch anime details
			continue
		}

		result = append(result, templates.WatchingAnimeWithDetails{
			Entry: row,
			Anime: anime,
		})
	}

	return result, nil
}
