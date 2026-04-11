package watchorder

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromFile_Success(t *testing.T) {
	temporaryDirectory := t.TempDir()
	filePath := filepath.Join(temporaryDirectory, "watch_order.json")

	content := `{
  "1": [{"id": 1, "type": "TV", "title": "One"}],
  "2": [{"id": 2, "type": "Movie", "title": "Two"}]
}`

	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	store, err := LoadFromFile(filePath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if store.Len() != 2 {
		t.Fatalf("expected 2 ids, got %d", store.Len())
	}

	entries, ok := store.Get(1)
	if !ok {
		t.Fatalf("expected id 1 to exist")
	}

	if len(entries) != 1 || entries[0].ID != 1 {
		t.Fatalf("unexpected entries for id 1: %+v", entries)
	}
}

func TestLoadFromFile_InvalidIDKey(t *testing.T) {
	temporaryDirectory := t.TempDir()
	filePath := filepath.Join(temporaryDirectory, "watch_order.json")

	if err := os.WriteFile(filePath, []byte(`{"abc": []}`), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	_, err := LoadFromFile(filePath)
	if err == nil {
		t.Fatalf("expected error for invalid id key")
	}
}

func TestLoadFromFile_WrappedPayload(t *testing.T) {
	temporaryDirectory := t.TempDir()
	filePath := filepath.Join(temporaryDirectory, "watch_order.json")

	content := `{"data":{"10":[{"id":10,"type":"TV","title":"Ten"}]}}`
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	store, err := LoadFromFile(filePath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if store.Len() != 1 {
		t.Fatalf("expected 1 id, got %d", store.Len())
	}
}
