package jikan

import "fmt"

// GetRelationsData fetches the raw relationships for an anime
func (c *Client) GetRelationsData(id int) (JikanRelationsResponse, error) {
	if cached, ok := c.relationsCache.Get(id); ok {
		return cached, nil
	}

	var result JikanRelationsResponse
	reqURL := fmt.Sprintf("%s/anime/%d/relations", c.baseURL, id)
	if err := c.fetchWithRetry(reqURL, &result); err != nil {
		return JikanRelationsResponse{}, err
	}

	c.relationsCache.Add(id, result)
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
func (c *Client) fetchChain(startID int, direction string, visited map[int]bool) []RelationEntry {
	rels, err := c.GetRelationsData(startID)
	if err != nil {
		return nil
	}

	nextIDPtr := findFirstAnimeRelation(rels, direction)
	if nextIDPtr == nil {
		return nil
	}

	nextID := *nextIDPtr
	if visited[nextID] { // prevent loops
		return nil
	}
	visited[nextID] = true

	anime, err := c.GetAnimeByID(nextID)
	if err != nil {
		return nil
	}

	entry := RelationEntry{Anime: anime, IsCurrent: false}
	rest := c.fetchChain(nextID, direction, visited)

	if direction == "Prequel" {
		return append(rest, entry)
	}
	return append([]RelationEntry{entry}, rest...)
}

// GetFullRelations resolves the full Prequel/Sequel chronological chain synchronously
func (c *Client) GetFullRelations(id int) []RelationEntry {
	currentAnime, err := c.GetAnimeByID(id)
	if err != nil {
		return nil
	}

	visited := map[int]bool{id: true}

	prequels := c.fetchChain(id, "Prequel", visited)

	// Clone visited set for sequels so we don't block valid paths if there's weird branching
	visitedSeq := make(map[int]bool)
	for k, v := range visited {
		visitedSeq[k] = v
	}

	sequels := c.fetchChain(id, "Sequel", visitedSeq)

	var result []RelationEntry
	result = append(result, prequels...)
	result = append(result, RelationEntry{Anime: currentAnime, IsCurrent: true})
	result = append(result, sequels...)

	return result
}
