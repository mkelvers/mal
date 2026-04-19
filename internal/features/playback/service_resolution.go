package playback

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
)

func (s *Service) resolveShow(ctx context.Context, malID int, titleCandidates []string) (string, string, error) {
	malText := strconv.Itoa(malID)
	modeCandidates := []string{"sub", "dub"}
	queries := buildTitleSearchQueries(titleCandidates)

	for _, query := range queries {
		resultsByMode := s.searchShowResultsByMode(ctx, query, modeCandidates)

		for _, mode := range modeCandidates {
			for _, result := range resultsByMode[mode] {
				if strings.TrimSpace(result.MalID) == malText && strings.TrimSpace(result.ID) != "" {
					return result.ID, result.Name, nil
				}
			}
		}

		for _, mode := range modeCandidates {
			results := resultsByMode[mode]
			if len(results) == 0 {
				continue
			}

			best := results[0]
			if strings.TrimSpace(best.ID) != "" {
				return best.ID, best.Name, nil
			}
		}
	}

	return "", "", errors.New("unable to resolve allanime show")
}

func (s *Service) searchShowResultsByMode(ctx context.Context, query string, modeCandidates []string) map[string][]searchResult {
	resultsByMode := make(map[string][]searchResult, len(modeCandidates))
	searchCh := make(chan searchModeResult, len(modeCandidates))

	var wg sync.WaitGroup
	for _, mode := range modeCandidates {
		modeValue := mode
		wg.Add(1)
		go func() {
			defer wg.Done()
			results, err := s.allAnimeClient.Search(ctx, query, modeValue)
			searchCh <- searchModeResult{Mode: modeValue, Results: results, Err: err}
		}()
	}

	wg.Wait()
	close(searchCh)

	for result := range searchCh {
		if result.Err != nil {
			continue
		}

		resultsByMode[result.Mode] = result.Results
	}

	return resultsByMode
}

func buildTitleSearchQueries(titleCandidates []string) []string {
	queries := make([]string, 0, len(titleCandidates)*4)
	seen := make(map[string]struct{})

	add := func(raw string) {
		normalized := normalizeSearchQuery(raw)
		if normalized == "" {
			return
		}

		key := strings.ToLower(normalized)
		if _, exists := seen[key]; exists {
			return
		}

		seen[key] = struct{}{}
		queries = append(queries, normalized)
	}

	for _, candidate := range titleCandidates {
		normalized := normalizeSearchQuery(candidate)
		if normalized == "" {
			continue
		}

		add(normalized)
		add(strings.ReplaceAll(normalized, "+", " "))

		withoutApostrophes := strings.NewReplacer("'", "", "’", "", "`", "").Replace(normalized)
		add(withoutApostrophes)
		add(strings.ReplaceAll(withoutApostrophes, "+", " "))
	}

	return queries
}

func normalizeSearchQuery(raw string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
}

func firstNonEmptyTitle(values []string) string {
	for _, value := range values {
		normalized := strings.TrimSpace(value)
		if normalized != "" {
			return normalized
		}
	}

	return ""
}

func normalizeMode(raw string) string {
	return strings.ToLower(strings.TrimSpace(raw))
}

func availableModes(modeSources map[string]ModeSource) []string {
	ordered := make([]string, 0, len(modeSources))
	if _, ok := modeSources["dub"]; ok {
		ordered = append(ordered, "dub")
	}
	if _, ok := modeSources["sub"]; ok {
		ordered = append(ordered, "sub")
	}

	extra := make([]string, 0)
	for mode := range modeSources {
		if mode == "dub" || mode == "sub" {
			continue
		}
		extra = append(extra, mode)
	}
	sort.Strings(extra)

	return append(ordered, extra...)
}

func selectInitialMode(requestedMode string, modeSources map[string]ModeSource) string {
	normalizedRequested := normalizeMode(requestedMode)
	if normalizedRequested != "" {
		if _, ok := modeSources[normalizedRequested]; ok {
			return normalizedRequested
		}
	}

	if _, ok := modeSources["dub"]; ok {
		return "dub"
	}
	if _, ok := modeSources["sub"]; ok {
		return "sub"
	}

	for mode := range modeSources {
		return mode
	}

	return "dub"
}
