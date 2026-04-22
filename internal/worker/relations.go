package worker

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"mal/integrations/jikan"
	"mal/internal/db"
)

type Worker struct {
	db     *database.Queries
	client *jikan.Client
}

func New(db *database.Queries, client *jikan.Client) *Worker {
	return &Worker{
		db:     db,
		client: client,
	}
}

func (w *Worker) Start(ctx context.Context) {
	log.Println("Starting relations sync worker...")
	ticker := time.NewTicker(1 * time.Minute)
	retryTicker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	defer retryTicker.Stop()

	// Run once immediately
	w.syncRelations(ctx)
	w.processAnimeFetchRetries(ctx)
	w.cleanupCache(ctx)

	cleanupCounter := 0

	for {
		select {
		case <-ctx.Done():
			return
		case <-w.client.RetrySignal():
			w.processAnimeFetchRetries(ctx)
		case <-retryTicker.C:
			w.processAnimeFetchRetries(ctx)
		case <-ticker.C:
			w.syncRelations(ctx)

			// Clean up cache every 60 runs (approx 1 hour)
			cleanupCounter++
			if cleanupCounter >= 60 {
				w.cleanupCache(ctx)
				cleanupCounter = 0
			}
		}
	}
}

func retryBackoff(attempts int64) string {
	if attempts < 1 {
		attempts = 1
	}

	delay := time.Minute
	if attempts > 1 {
		shift := attempts - 1
		if shift > 6 {
			shift = 6
		}
		delay = time.Minute * time.Duration(1<<shift)
	}

	if delay > 30*time.Minute {
		delay = 30 * time.Minute
	}

	minutes := int(delay / time.Minute)
	if minutes < 1 {
		minutes = 1
	}
	return fmt.Sprintf("+%d minutes", minutes)
}

func (w *Worker) processAnimeFetchRetries(ctx context.Context) {
	retries, err := w.db.GetDueAnimeFetchRetries(ctx, 20)
	if err != nil {
		log.Printf("worker: failed to load due anime fetch retries: %v", err)
		return
	}

	if len(retries) == 0 {
		return
	}

	for _, retry := range retries {
		_, err := w.client.GetAnimeByID(ctx, int(retry.AnimeID))
		if err != nil {
			if !jikan.IsRetryableError(err) {
				deleteErr := w.db.DeleteAnimeFetchRetry(ctx, retry.AnimeID)
				if deleteErr != nil {
					log.Printf("worker: failed deleting non-retryable anime retry %d: %v", retry.AnimeID, deleteErr)
				}
				continue
			}

			updateErr := w.db.MarkAnimeFetchRetryFailed(ctx, database.MarkAnimeFetchRetryFailedParams{
				Datetime:  retryBackoff(retry.Attempts + 1),
				LastError: err.Error(),
				AnimeID:   retry.AnimeID,
			})
			if updateErr != nil {
				log.Printf("worker: failed updating anime fetch retry %d: %v", retry.AnimeID, updateErr)
			}
			continue
		}

		deleteErr := w.db.DeleteAnimeFetchRetry(ctx, retry.AnimeID)
		if deleteErr != nil {
			log.Printf("worker: failed deleting successful anime retry %d: %v", retry.AnimeID, deleteErr)
		}
	}
}

func (w *Worker) cleanupCache(ctx context.Context) {
	err := w.db.DeleteExpiredJikanCache(ctx)
	if err != nil {
		log.Printf("worker: failed to clean up expired jikan cache: %v", err)
	}
}

func (w *Worker) syncRelations(ctx context.Context) {
	// Find up to 20 anime that need their relations synced
	animes, err := w.db.GetAnimeNeedingRelationSync(ctx)
	if err != nil {
		log.Printf("worker error: failed to get anime needing sync: %v", err)
		return
	}

	if len(animes) == 0 {
		return // silent heartbeat
	}

	for _, a := range animes {
		func() {
			animeData, err := w.client.GetAnimeByID(ctx, int(a.ID))
			if err != nil {
				log.Printf("worker: failed to fetch anime details for %d: %v", a.ID, err)
				// Sleep a bit on error to respect rate limits, but DO NOT mark as synced
				// so it will be retried on the next worker run.
				time.Sleep(2 * time.Second)
				return
			}

			// If we got here, we successfully fetched the data, so we mark it as synced.
			defer func() {
				err := w.db.MarkRelationsSynced(ctx, a.ID)
				if err != nil {
					log.Printf("worker: failed to mark relations synced for %d: %v", a.ID, err)
				}
				time.Sleep(400 * time.Millisecond)
			}()

			for _, rel := range animeData.Relations {
				for _, entry := range rel.Entry {
					if entry.Type == "anime" {
						// We just insert the relation.
						err := w.db.UpsertAnimeRelation(ctx, database.UpsertAnimeRelationParams{
							AnimeID:        a.ID,
							RelatedAnimeID: int64(entry.MalID),
							RelationType:   rel.Relation,
						})
						if err != nil {
							log.Printf("worker: failed to insert relation %d -> %d: %v", a.ID, entry.MalID, err)
						}

						// If it's a Sequel, we should also make sure the related anime is tracked
						if rel.Relation == "Sequel" {
							w.ensureAnimeExistsAndStatusUpdated(ctx, entry.MalID)
						}
					}
				}
			}

			err = w.db.UpdateAnimeStatus(ctx, database.UpdateAnimeStatusParams{
				Status: sql.NullString{String: animeData.Status, Valid: true},
				ID:     a.ID,
			})
			if err != nil {
				log.Printf("worker: failed to update status for %d: %v", a.ID, err)
			}
		}()
	}
}

func (w *Worker) ensureAnimeExistsAndStatusUpdated(ctx context.Context, malID int) {
	// check if we have it
	_, err := w.db.GetAnime(ctx, int64(malID))
	if err != nil {
		// we don't have it, let's fetch it
		animeDetails, err := w.client.GetAnimeByID(ctx, malID)
		if err != nil {
			log.Printf("worker: failed to fetch related anime %d: %v", malID, err)
			return
		}

		_, err = w.db.UpsertAnime(ctx, database.UpsertAnimeParams{
			ID:            int64(animeDetails.MalID),
			TitleOriginal: animeDetails.Title,
			TitleEnglish:  sql.NullString{String: animeDetails.TitleEnglish, Valid: animeDetails.TitleEnglish != ""},
			TitleJapanese: sql.NullString{String: animeDetails.TitleJapanese, Valid: animeDetails.TitleJapanese != ""},
			ImageUrl:      animeDetails.ImageURL(),
			Airing:        sql.NullBool{Bool: animeDetails.Airing, Valid: true},
		})
		if err != nil {
			log.Printf("worker: failed to insert related anime %d: %v", malID, err)
			return
		}

		err = w.db.UpdateAnimeStatus(ctx, database.UpdateAnimeStatusParams{
			Status: sql.NullString{String: animeDetails.Status, Valid: true},
			ID:     int64(animeDetails.MalID),
		})
		if err != nil {
			log.Printf("worker: failed to update status for related anime %d: %v", malID, err)
		}

		time.Sleep(400 * time.Millisecond)
	} else {
		// We have it, but maybe status is outdated. Fetching every time might be too much,
		// but since it's a Sequel to something they watched, we could fetch it.
		// For now, let's just let the worker naturally pick it up if it gets added to watchlist,
		// OR we can explicitly fetch its details to keep sequels up to date.
		animeDetails, err := w.client.GetAnimeByID(ctx, malID)
		if err == nil {
			if err := w.db.UpdateAnimeStatus(ctx, database.UpdateAnimeStatusParams{
				Status: sql.NullString{String: animeDetails.Status, Valid: true},
				ID:     int64(animeDetails.MalID),
			}); err != nil {
				log.Printf("worker: failed to update status for anime %d: %v", animeDetails.MalID, err)
			}
		}
		time.Sleep(400 * time.Millisecond)
	}
}
