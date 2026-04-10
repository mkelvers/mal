package jikan

import (
	"context"
	"slices"
	"sort"
	"strings"
	"time"
)

var canonicalRelationOrder = []string{
	"prequel",
	"sequel",
	"parent story",
	"full story",
	"alternative version",
	"alternative setting",
}

var extraRelationOrder = []string{
	"side story",
	"spin-off",
	"summary",
	"other",
}

var relationPriorityOrder = append(
	append([]string{}, canonicalRelationOrder...),
	extraRelationOrder...,
)

func relationKey(rel string) string {
	key := strings.ToLower(strings.TrimSpace(rel))
	key = strings.ReplaceAll(key, "_", " ")
	key = strings.Join(strings.Fields(key), " ")

	switch key {
	case "prequels":
		return "prequel"
	case "sequels":
		return "sequel"
	case "side stories":
		return "side story"
	case "spin off", "spinoff":
		return "spin-off"
	default:
		return key
	}
}

func relationLabel(rel string) string {
	key := relationKey(rel)
	switch key {
	case "prequel":
		return "Prequels"
	case "sequel":
		return "Sequels"
	case "parent story":
		return "Parent story"
	case "full story":
		return "Full story"
	case "alternative version":
		return "Alternative version"
	case "alternative setting":
		return "Alternative setting"
	case "side story":
		return "Side story"
	case "spin-off":
		return "Spin-off"
	case "summary":
		return "Summary"
	case "other":
		return "Other"
	default:
		return strings.TrimSpace(rel)
	}
}

func isCanonicalRelation(rel string) bool {
	return slices.Contains(canonicalRelationOrder, relationKey(rel))
}

func isExtraRelation(rel string) bool {
	return slices.Contains(extraRelationOrder, relationKey(rel))
}

func isFranchiseRelation(rel string) bool {
	return isCanonicalRelation(rel) || isExtraRelation(rel)
}

func relationOrder(rel string) int {
	key := relationKey(rel)
	for i, allowed := range relationPriorityOrder {
		if key == allowed {
			return i
		}
	}
	return len(relationPriorityOrder) + 1
}

func relationAiredAt(anime Anime) (time.Time, bool) {
	from := strings.TrimSpace(anime.Aired.From)
	if from != "" {
		if parsed, err := time.Parse(time.RFC3339, from); err == nil {
			return parsed, true
		}
		if parsed, err := time.Parse("2006-01-02", from); err == nil {
			return parsed, true
		}
	}

	if anime.Year > 0 {
		return time.Date(anime.Year, time.January, 1, 0, 0, 0, 0, time.UTC), true
	}

	return time.Time{}, false
}

func sortRelationEntriesChronological(entries []RelationEntry) {
	sort.SliceStable(entries, func(i int, j int) bool {
		left := entries[i]
		right := entries[j]

		leftAiredAt, leftHasAiredAt := relationAiredAt(left.Anime)
		rightAiredAt, rightHasAiredAt := relationAiredAt(right.Anime)

		if leftHasAiredAt != rightHasAiredAt {
			return leftHasAiredAt
		}

		if leftHasAiredAt && !leftAiredAt.Equal(rightAiredAt) {
			return leftAiredAt.Before(rightAiredAt)
		}

		leftRelationOrder := relationOrder(left.Relation)
		rightRelationOrder := relationOrder(right.Relation)
		if leftRelationOrder != rightRelationOrder {
			return leftRelationOrder < rightRelationOrder
		}

		leftTitle := strings.ToLower(left.Anime.DisplayTitle())
		rightTitle := strings.ToLower(right.Anime.DisplayTitle())
		if leftTitle != rightTitle {
			return leftTitle < rightTitle
		}

		return left.Anime.MalID < right.Anime.MalID
	})
}

func relationEntries(ctx context.Context, c *Client, anime Anime) ([]RelationEntry, error) {
	entries := make([]RelationEntry, 0)

	for _, group := range anime.Relations {
		if !isFranchiseRelation(group.Relation) {
			continue
		}

		for _, entry := range group.Entry {
			if entry.Type != "anime" {
				continue
			}

			relAnime, err := c.GetAnimeByID(ctx, entry.MalID)
			if err != nil {
				return nil, err
			}

			entries = append(entries, RelationEntry{
				Anime:     relAnime,
				Relation:  relationLabel(group.Relation),
				IsCurrent: false,
				IsExtra:   !isCanonicalRelation(group.Relation),
			})
		}
	}

	return entries, nil
}

func relationMap(ctx context.Context, c *Client, id int) (map[int]RelationEntry, error) {
	currentAnime, err := c.GetAnimeByID(ctx, id)
	if err != nil {
		return nil, err
	}

	result := map[int]RelationEntry{
		currentAnime.MalID: {
			Anime:     currentAnime,
			Relation:  "Current",
			IsCurrent: true,
			IsExtra:   false,
		},
	}

	queue := []Anime{currentAnime}
	visited := map[int]bool{currentAnime.MalID: true}

	for len(queue) > 0 {
		anime := queue[0]
		queue = queue[1:]

		entries, err := relationEntries(ctx, c, anime)
		if err != nil {
			return nil, err
		}

		for _, rel := range entries {
			existing, exists := result[rel.Anime.MalID]
			if !exists {
				result[rel.Anime.MalID] = rel
			} else if !existing.IsCurrent {
				if existing.IsExtra && !rel.IsExtra {
					// Prefer canonical timeline links over extras when both point to the same anime.
					result[rel.Anime.MalID] = rel
				} else if existing.IsExtra && rel.IsExtra && relationOrder(rel.Relation) < relationOrder(existing.Relation) {
					// Keep the most specific extra label when multiple extra relations exist.
					result[rel.Anime.MalID] = rel
				}
			}

			if !rel.IsExtra && !visited[rel.Anime.MalID] {
				visited[rel.Anime.MalID] = true
				queue = append(queue, rel.Anime)
			}
		}
	}

	return result, nil
}

func (c *Client) GetFullRelations(ctx context.Context, id int) ([]RelationEntry, error) {
	relationByID, err := relationMap(ctx, c, id)
	if err != nil {
		return nil, err
	}

	ordered := make([]RelationEntry, 0, len(relationByID))
	for _, entry := range relationByID {
		ordered = append(ordered, entry)
	}

	sortRelationEntriesChronological(ordered)

	return ordered, nil
}
