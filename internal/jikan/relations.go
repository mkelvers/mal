package jikan

// findFirstAnimeRelation extracts the first related anime ID for a specific relation type
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

// fetchChain recursively builds the relational chain (Prequels or Sequels)
func (c *Client) fetchChain(startID int, direction string, visited map[int]bool) ([]RelationEntry, error) {
	anime, err := c.GetAnimeByID(startID)
	if err != nil {
		return nil, err
	}

	nextIDPtr := findFirstAnimeRelation(anime.Relations, direction)
	if nextIDPtr == nil {
		return nil, nil // normal end of chain
	}

	nextID := *nextIDPtr
	if visited[nextID] { // prevent loops
		return nil, nil
	}
	visited[nextID] = true

	nextAnime, err := c.GetAnimeByID(nextID)
	if err != nil {
		return nil, err
	}

	entry := RelationEntry{Anime: nextAnime, IsCurrent: false}
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
