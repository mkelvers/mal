package anime

import (
	"context"
	"fmt"

	"mal/internal/database"
	"mal/internal/jikan"
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

func (s *Service) GetEpisodes(id int, page int) (jikan.EpisodesResult, error) {
	return s.jikanClient.GetEpisodes(id, page)
}

func (s *Service) GetAllEpisodes(id int) ([]jikan.Episode, error) {
	return s.jikanClient.GetAllEpisodes(id)
}

func (s *Service) GetAnime(id int) (jikan.Anime, error) {
	return s.jikanClient.GetAnimeByID(id)
}
