package watchorder

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
)

type Store struct {
	byID map[int][]WatchOrderEntry
}

func EmptyStore() *Store {
	return &Store{byID: make(map[int][]WatchOrderEntry)}
}

func (s *Store) Len() int {
	if s == nil {
		return 0
	}

	return len(s.byID)
}

func (s *Store) Get(id int) ([]WatchOrderEntry, bool) {
	if s == nil {
		return nil, false
	}

	entries, ok := s.byID[id]
	if !ok {
		return nil, false
	}

	return entries, true
}

func LoadFromFile(path string) (*Store, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read watch-order file %q: %w", path, err)
	}

	rawMessages := make(map[string]json.RawMessage)
	if err := json.Unmarshal(content, &rawMessages); err != nil {
		return nil, fmt.Errorf("failed to parse watch-order file %q: %w", path, err)
	}

	raw := make(map[string][]WatchOrderEntry)
	if wrappedData, ok := rawMessages["data"]; ok && len(rawMessages) == 1 {
		if err := json.Unmarshal(wrappedData, &raw); err != nil {
			return nil, fmt.Errorf("failed to parse watch-order data in file %q: %w", path, err)
		}
	} else {
		if err := json.Unmarshal(content, &raw); err != nil {
			return nil, fmt.Errorf("failed to parse watch-order file %q: %w", path, err)
		}
	}

	byID := make(map[int][]WatchOrderEntry, len(raw))
	for key, entries := range raw {
		id, err := strconv.Atoi(key)
		if err != nil {
			return nil, fmt.Errorf("invalid anime id key %q in watch-order file %q: %w", key, path, err)
		}

		byID[id] = entries
	}

	return &Store{byID: byID}, nil
}
