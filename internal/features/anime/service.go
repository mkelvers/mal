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

func (s *Service) Search(ctx context.Context, query string, page int) (jikan.SearchResult, error) {
	return s.jikanClient.Search(ctx, query, page)
}

func (s *Service) QuickSearch(ctx context.Context, query string, page int, limit int) (jikan.SearchResult, error) {
	return s.jikanClient.SearchWithLimit(ctx, query, page, limit)
}

func (s *Service) GetTopAnime(ctx context.Context, page int) (jikan.TopAnimeResult, error) {
	return s.jikanClient.GetTopAnime(ctx, page)
}

func (s *Service) GetTopAnimeWithPlaceholder(ctx context.Context, page int) (jikan.TopAnimeResult, bool, error) {
	result, err := s.jikanClient.GetTopAnime(ctx, page)
	if err == nil {
		return result, false, nil
	}

	if jikan.IsRetryableError(err) {
		return jikan.TopAnimeResult{}, true, nil
	}

	return jikan.TopAnimeResult{}, false, err
}

func (s *Service) GetAiringAnime(ctx context.Context, page int) (jikan.TopAnimeResult, error) {
	return s.jikanClient.GetSeasonsNow(ctx, page)
}

func (s *Service) GetUpcomingAnime(ctx context.Context, page int) (jikan.TopAnimeResult, error) {
	return s.jikanClient.GetSeasonsUpcoming(ctx, page)
}

func (s *Service) GetAnimeDetails(ctx context.Context, id int, userID string) (jikan.Anime, string, int, error) {
	anime, err := s.jikanClient.GetAnimeByID(ctx, id)
	if err != nil {
		if jikan.IsNotFoundError(err) {
			return jikan.Anime{}, "", 1, err
		}

		s.jikanClient.EnqueueAnimeFetchRetry(ctx, id, err)
		if jikan.IsRetryableError(err) {
			return jikan.Anime{}, "", 1, ErrAnimePendingFetch
		}

		return jikan.Anime{}, "", 1, fmt.Errorf("failed to fetch anime details: %w", err)
	}

	currentStatus := ""
	nextEpisode := 1
	if userID != "" {
		entry, err := s.db.GetWatchListEntry(ctx, database.GetWatchListEntryParams{
			UserID:  userID,
			AnimeID: int64(id),
		})
		if err == nil {
			currentStatus = entry.Status
			if entry.CurrentEpisode.Valid {
				value := int(entry.CurrentEpisode.Int64)
				if value > 0 {
					nextEpisode = value
				}
			}
		}
	}

	return anime, currentStatus, nextEpisode, nil
}

func (s *Service) GetRelations(ctx context.Context, id int) ([]jikan.RelationEntry, error) {
	return s.jikanClient.GetFullRelations(ctx, id)
}

func (s *Service) GetRecommendations(ctx context.Context, animeID int, limit int) ([]jikan.Anime, error) {
	return s.jikanClient.GetRecommendations(ctx, animeID, limit)
}

func (s *Service) GetAnimeByProducer(ctx context.Context, producerID int, page int) (jikan.StudioAnimeResult, error) {
	return s.jikanClient.GetAnimeByProducer(ctx, producerID, page)
}

func (s *Service) GetProducerByID(ctx context.Context, producerID int) (jikan.ProducerResponse, error) {
	return s.jikanClient.GetProducerByID(ctx, producerID)
}

func (s *Service) GetEpisodes(ctx context.Context, animeID int) ([]jikan.Episode, error) {
	var allEpisodes []jikan.Episode
	page := 1

	for page <= 20 {
		result, err := s.jikanClient.GetEpisodes(ctx, animeID, page)
		if err != nil {
			if jikan.IsRetryableError(err) && len(allEpisodes) > 0 {
				// Return what we have if we're getting rate limited
				return allEpisodes, nil
			}
			return nil, err
		}

		allEpisodes = append(allEpisodes, result.Data...)

		if !result.Pagination.HasNextPage {
			break
		}
		page++
	}

	return allEpisodes, nil
}
