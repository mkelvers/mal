package jikan

import (
	"fmt"
	"time"
)

// GetRelationsData fetches the raw relationships for an anime
func (c *Client) GetRelationsData(id int) (JikanRelationsResponse, error) {
	cacheKey := fmt.Sprintf("relations:%d", id)
	var cached JikanRelationsResponse
	if c.getCache(cacheKey, &cached) {
		return cached, nil
	}

	var result JikanRelationsResponse
	reqURL := fmt.Sprintf("%s/anime/%d/relations", c.baseURL, id)
	if err := c.fetchWithRetry(reqURL, &result); err != nil {
		return JikanRelationsResponse{}, err
	}

	c.setCache(cacheKey, result, time.Hour*24)
	return result, nil
}

// findFirstAnimeRelation extracts the first related anime ID for a specific relation type
func findFirstAnimeRelation(res JikanRelationsResponse, relType string) *int {
	for _, group := range res.Data {
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

// fetchChain recursively builds the relational chain (Prequels or Sequels)
func (c *Client) fetchChain(startID int, direction string, visited map[int]bool) ([]RelationEntry, error) {
	rels, err := c.GetRelationsData(startID)
	if err != nil {
		return nil, err
	}

	nextIDPtr := findFirstAnimeRelation(rels, direction)
	if nextIDPtr == nil {
		return nil, nil // normal end of chain
	}

	nextID := *nextIDPtr
	if visited[nextID] { // prevent loops
		return nil, nil
	}
	visited[nextID] = true

	anime, err := c.GetAnimeByID(nextID)
	if err != nil {
		return nil, err
	}

	entry := RelationEntry{Anime: anime, IsCurrent: false}
	rest, err := c.fetchChain(nextID, direction, visited)
	if err != nil {
		return nil, err
	}

	if direction == "Prequel" {
		return append(rest, entry), nil
	}
	return append([]RelationEntry{entry}, rest...), nil
}

// GetFullRelations resolves the full Prequel/Sequel chronological chain synchronously
func (c *Client) GetFullRelations(id int) ([]RelationEntry, error) {
	currentAnime, err := c.GetAnimeByID(id)
	if err != nil {
		return nil, err
	}

	visited := map[int]bool{id: true}

	prequels, err1 := c.fetchChain(id, "Prequel", visited)

	visitedSeq := make(map[int]bool)
	for k, v := range visited {
		visitedSeq[k] = v
	}

	sequels, err2 := c.fetchChain(id, "Sequel", visitedSeq)

	// If both chains errored and it wasn't just "no relations", we should probably error out
	// But it's safer to just return what we have and the error so the UI can decide
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
