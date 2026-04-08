package jikan

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"mal/internal/database"
)

type Client struct {
	httpClient  *http.Client
	baseURL     string
	db          database.Querier
	mu          sync.Mutex
	lastReqTime time.Time
}

func NewClient(db database.Querier) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		baseURL:    "https://api.jikan.moe/v4",
		db:         db,
	}
}

func (c *Client) waitRateLimit() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	nextAllowed := c.lastReqTime.Add(340 * time.Millisecond)
	if now.Before(nextAllowed) {
		time.Sleep(nextAllowed.Sub(now))
		c.lastReqTime = time.Now()
	} else {
		c.lastReqTime = now
	}
}

func (c *Client) getCache(key string, out interface{}) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	data, err := c.db.GetJikanCache(ctx, key)
	if err != nil {
		return false
	}

	err = json.Unmarshal([]byte(data), out)
	return err == nil
}

func (c *Client) setCache(key string, data interface{}, ttl time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	bytes, err := json.Marshal(data)
	if err != nil {
		return
	}

	_ = c.db.SetJikanCache(ctx, database.SetJikanCacheParams{
		Key:       key,
		Data:      string(bytes),
		ExpiresAt: time.Now().Add(ttl),
	})
}

// fetchWithRetry provides robust fetching respecting Jikan's strict 3 req/sec rate limit
func (c *Client) fetchWithRetry(urlStr string, out interface{}) error {
	maxRetries := 3
	for i := 0; i < maxRetries; i++ {
		c.waitRateLimit()

		resp, err := c.httpClient.Get(urlStr)
		if err != nil {
			return fmt.Errorf("jikan api error: %w", err)
		}

		if resp.StatusCode == 429 {
			resp.Body.Close()
			time.Sleep(800 * time.Millisecond) // Double delay on rate limit
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return fmt.Errorf("jikan api returned status %d", resp.StatusCode)
		}

		err = json.NewDecoder(resp.Body).Decode(out)
		resp.Body.Close()
		return err
	}
	return fmt.Errorf("max retries exceeded for %s", urlStr)
}
