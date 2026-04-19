package jikan

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"mal/internal/database"
)

type staleCacheQuerier struct {
	database.Querier
	staleJSON string
}

func (q *staleCacheQuerier) GetJikanCache(ctx context.Context, key string) (string, error) {
	return "", sql.ErrNoRows
}

func (q *staleCacheQuerier) GetJikanCacheStale(ctx context.Context, key string) (string, error) {
	if q.staleJSON == "" {
		return "", sql.ErrNoRows
	}

	return q.staleJSON, nil
}

func TestGetProducerByID_UsesStaleCacheOnFetchFailure(t *testing.T) {
	t.Parallel()

	q := &staleCacheQuerier{
		staleJSON: `{"data":{"mal_id":7,"about":"stale about"}}`,
	}

	client := NewClient(q)
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer testServer.Close()

	client.baseURL = testServer.URL
	client.httpClient = testServer.Client()

	result, err := client.GetProducerByID(context.Background(), 7)
	if err != nil {
		t.Fatalf("expected stale cache result, got error: %v", err)
	}

	if result.Data.MalID != 7 {
		t.Fatalf("expected stale mal_id 7, got %d", result.Data.MalID)
	}

	if result.Data.About != "stale about" {
		t.Fatalf("expected stale about field, got %q", result.Data.About)
	}
}

func TestGetAnimeByProducer_UsesStaleCacheOnFetchFailure(t *testing.T) {
	t.Parallel()

	q := &staleCacheQuerier{
		staleJSON: `{"Animes":[{"mal_id":42,"title":"Stale Anime"}],"HasNextPage":true,"StudioName":"Stale Studio"}`,
	}

	client := NewClient(q)
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer testServer.Close()

	client.baseURL = testServer.URL
	client.httpClient = testServer.Client()

	result, err := client.GetAnimeByProducer(context.Background(), 9, 1)
	if err != nil {
		t.Fatalf("expected stale cache result, got error: %v", err)
	}

	if len(result.Animes) != 1 {
		t.Fatalf("expected one stale anime, got %d", len(result.Animes))
	}

	if result.Animes[0].MalID != 42 {
		t.Fatalf("expected stale anime mal_id 42, got %d", result.Animes[0].MalID)
	}

	if !result.HasNextPage {
		t.Fatal("expected stale has_next_page=true")
	}

	if result.StudioName != "Stale Studio" {
		t.Fatalf("expected stale studio name, got %q", result.StudioName)
	}
}
