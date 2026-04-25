package watchlist

import (
	"context"
	"testing"

	"mal/internal/db"
)

type fakeQuerier struct {
	database.Querier
	upsertAnimeCalled bool
	upsertEntryCalled bool
	addRows           []database.GetUserWatchListRow
}

func (f *fakeQuerier) UpsertAnime(ctx context.Context, arg database.UpsertAnimeParams) (database.Anime, error) {
	f.upsertAnimeCalled = true
	return database.Anime{}, nil
}

func (f *fakeQuerier) UpsertWatchListEntry(ctx context.Context, arg database.UpsertWatchListEntryParams) (database.WatchListEntry, error) {
	f.upsertEntryCalled = true
	return database.WatchListEntry{}, nil
}

func (f *fakeQuerier) GetUserWatchList(ctx context.Context, userID string) ([]database.GetUserWatchListRow, error) {
	return f.addRows, nil
}

func TestAddEntry_RejectsInvalidAnimeID(t *testing.T) {
	t.Parallel()

	q := &fakeQuerier{}
	svc := NewService(q, nil)

	err := svc.AddEntry(context.Background(), "user-1", AddRequest{
		AnimeID: 0,
		Status:  "watching",
	})

	if err != ErrInvalidAnimeID {
		t.Fatalf("expected ErrInvalidAnimeID, got %v", err)
	}

	if q.upsertAnimeCalled || q.upsertEntryCalled {
		t.Fatal("expected no database writes for invalid anime id")
	}
}

func TestAddEntry_RejectsInvalidStatus(t *testing.T) {
	t.Parallel()

	q := &fakeQuerier{}
	svc := NewService(q, nil)

	err := svc.AddEntry(context.Background(), "user-1", AddRequest{
		AnimeID: 1,
		Status:  "invalid",
	})

	if err != ErrInvalidStatus {
		t.Fatalf("expected ErrInvalidStatus, got %v", err)
	}

	if q.upsertAnimeCalled || q.upsertEntryCalled {
		t.Fatal("expected no database writes for invalid status")
	}
}
