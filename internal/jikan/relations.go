package jikan

import (
	"context"
	"maps"
)

func findFirstAnimeRelation(groups []JikanRelationGroup, relType string) *int {
	for _, group := range groups {
		if group.Relation == relType {
			for _, entry := range group.Entry {
				if entry.Type == "anime" {
					id := entry.MalID
					return &id
				}
			}
		}
	}
	return nil
}

func (c *Client) fetchChain(ctx context.Context, startID int, direction string, visited map[int]bool) ([]RelationEntry, error) {
	anime, err := c.GetAnimeByID(ctx, startID)
	if err != nil {
		return nil, err
	}

	nextIDPtr := findFirstAnimeRelation(anime.Relations, direction)
	if nextIDPtr == nil {
		return nil, nil
	}

	nextID := *nextIDPtr
	if visited[nextID] { // prevent loops
		return nil, nil
	}
	visited[nextID] = true

	nextAnime, err := c.GetAnimeByID(ctx, nextID)
	if err != nil {
		return nil, err
	}

	entry := RelationEntry{Anime: nextAnime, IsCurrent: false}
	rest, err := c.fetchChain(ctx, nextID, direction, visited)
	if err != nil {
		return nil, err
	}

	if direction == "Prequel" {
		return append(rest, entry), nil
	}
	return append([]RelationEntry{entry}, rest...), nil
}

func (c *Client) GetFullRelations(ctx context.Context, id int) ([]RelationEntry, error) {
	currentAnime, err := c.GetAnimeByID(ctx, id)
	if err != nil {
		return nil, err
	}

	visited := map[int]bool{id: true}

	prequels, err1 := c.fetchChain(ctx, id, "Prequel", visited)

	visitedSeq := make(map[int]bool)
	maps.Copy(visitedSeq, visited)

	sequels, err2 := c.fetchChain(ctx, id, "Sequel", visitedSeq)

	var result []RelationEntry
	result = append(result, prequels...)
	result = append(result, RelationEntry{Anime: currentAnime, IsCurrent: true})
	result = append(result, sequels...)

	var finalErr error
	if err1 != nil {
		finalErr = err1
	} else if err2 != nil {
		finalErr = err2
	}

	return result, finalErr
}
