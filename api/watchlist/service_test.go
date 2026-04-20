package watchlist

import (
	"context"
	"database/sql"
	"testing"
	"time"

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

func TestExport_UsesDisplayTitleFallbackOrder(t *testing.T) {
	t.Parallel()

	q := &fakeQuerier{
		addRows: []database.GetUserWatchListRow{
			{
				AnimeID:       101,
				TitleOriginal: "Original",
				TitleEnglish:  sql.NullString{String: "English", Valid: true},
				Status:        "watching",
				ImageUrl:      "https://img",
				UpdatedAt:     time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC),
			},
			{
				AnimeID:       102,
				TitleOriginal: "Original 2",
				TitleJapanese: sql.NullString{String: "JP Title", Valid: true},
				Status:        "completed",
				ImageUrl:      "https://img2",
				UpdatedAt:     time.Date(2026, 1, 3, 3, 4, 5, 0, time.UTC),
			},
			{
				AnimeID:       103,
				TitleOriginal: "Original 3",
				Status:        "on_hold",
				ImageUrl:      "https://img3",
				UpdatedAt:     time.Date(2026, 1, 4, 3, 4, 5, 0, time.UTC),
			},
		},
	}

	svc := NewService(q, nil)
	export, err := svc.Export(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(export.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(export.Entries))
	}

	if export.Entries[0].Title != "English" {
		t.Fatalf("expected english title first, got %q", export.Entries[0].Title)
	}

	if export.Entries[1].Title != "JP Title" {
		t.Fatalf("expected japanese title fallback, got %q", export.Entries[1].Title)
	}

	if export.Entries[2].Title != "Original 3" {
		t.Fatalf("expected original title fallback, got %q", export.Entries[2].Title)
	}
}
