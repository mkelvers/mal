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

func (c *Client) waitRateLimit(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	// Jikan has a 3 req/sec limit AND a 60 req/min limit.
	// 400ms base delay keeps us safely under the 3/sec limit.
	nextAllowed := c.lastReqTime.Add(400 * time.Millisecond)
	if now.Before(nextAllowed) {
		timer := time.NewTimer(nextAllowed.Sub(now))
		defer timer.Stop()

		select {
		case <-timer.C:
		case <-ctx.Done():
			return fmt.Errorf("request canceled while waiting for rate limit: %w", ctx.Err())
		}
		c.lastReqTime = time.Now()
	} else {
		c.lastReqTime = now
	}

	return nil
}

func (c *Client) getCache(parentCtx context.Context, key string, out any) bool {
	ctx, cancel := context.WithTimeout(parentCtx, 2*time.Second)
	defer cancel()

	data, err := c.db.GetJikanCache(ctx, key)
	if err != nil {
		return false
	}

	err = json.Unmarshal([]byte(data), out)
	return err == nil
}

func (c *Client) setCache(parentCtx context.Context, key string, data any, ttl time.Duration) {
	ctx, cancel := context.WithTimeout(parentCtx, 2*time.Second)
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

func (c *Client) fetchWithRetry(ctx context.Context, urlStr string, out any) error {
	maxRetries := 5
	for range maxRetries {
		if err := c.waitRateLimit(ctx); err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create jikan request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("jikan api error: %w", err)
		}

		if resp.StatusCode == 429 {
			resp.Body.Close()
			// Jikan rate limit is hit (usually the 60 requests/minute limit)
			// Wait for 2 seconds before retrying to let the bucket refill slightly
			timer := time.NewTimer(2 * time.Second)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return fmt.Errorf("request canceled while retrying jikan request: %w", ctx.Err())
			}
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
