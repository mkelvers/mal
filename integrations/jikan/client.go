package jikan

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"mal/internal/db"
)

type Client struct {
	httpClient  *http.Client
	baseURL     string
	db          database.Querier
	retrySignal chan struct{}
	mu          sync.Mutex
	lastReqTime time.Time
}

func NewClient(db database.Querier) *Client {
	return &Client{
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		baseURL:     "https://api.jikan.moe/v4",
		db:          db,
		retrySignal: make(chan struct{}, 1),
	}
}

type APIError struct {
	StatusCode int
	URL        string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("jikan api returned status %d", e.StatusCode)
}

func IsNotFoundError(err error) bool {
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusNotFound
	}

	return false
}

func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	var apiErr *APIError
	if errors.As(err, &apiErr) {
		return isRetryableStatus(apiErr.StatusCode)
	}

	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	return false
}

func isRetryableStatus(statusCode int) bool {
	if statusCode == http.StatusTooManyRequests {
		return true
	}

	return statusCode >= 500 && statusCode <= 504
}

func retryDelay(attempt int) time.Duration {
	base := 500 * time.Millisecond
	delay := base * time.Duration(1<<attempt)
	if delay > 8*time.Second {
		return 8 * time.Second
	}

	return delay
}

func parseRetryAfter(value string) (time.Duration, bool) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, false
	}

	seconds, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, false
	}

	if seconds <= 0 {
		return 0, false
	}

	return time.Duration(seconds) * time.Second, true
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("request canceled while retrying jikan request: %w", ctx.Err())
	}
}

func truncateErrorMessage(message string) string {
	if len(message) <= 400 {
		return message
	}

	return message[:400]
}

func (c *Client) notifyRetryWorker() {
	select {
	case c.retrySignal <- struct{}{}:
	default:
	}
}

func (c *Client) RetrySignal() <-chan struct{} {
	return c.retrySignal
}

func (c *Client) EnqueueAnimeFetchRetry(parentCtx context.Context, animeID int, cause error) {
	if animeID <= 0 || !IsRetryableError(cause) {
		return
	}

	ctx, cancel := context.WithTimeout(parentCtx, 2*time.Second)
	defer cancel()

	err := c.db.EnqueueAnimeFetchRetry(ctx, database.EnqueueAnimeFetchRetryParams{
		AnimeID:   int64(animeID),
		LastError: truncateErrorMessage(cause.Error()),
	})
	if err != nil {
		return
	}

	c.notifyRetryWorker()
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

func (c *Client) getStaleCache(parentCtx context.Context, key string, out any) bool {
	ctx, cancel := context.WithTimeout(parentCtx, 2*time.Second)
	defer cancel()

	data, err := c.db.GetJikanCacheStale(ctx, key)
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

type cacheResult struct {
	data     any
	hasStale bool
}

func (c *Client) getWithCache(ctx context.Context, cacheKey string, ttl time.Duration, url string, out any) error {
	if c.getCache(ctx, cacheKey, out) {
		return nil
	}

	var stale any
	hasStale := c.getStaleCache(ctx, cacheKey, &stale)

	if err := c.fetchWithRetry(ctx, url, out); err != nil {
		if hasStale {
			staleBytes, marshalErr := json.Marshal(stale)
			if marshalErr == nil {
				unmarshalErr := json.Unmarshal(staleBytes, out)
				if unmarshalErr == nil {
					return nil
				}
			}
			log.Printf("jikan: stale cache unmarshal failed, falling back to error: %v", err)
		}
		return err
	}

	c.setCache(ctx, cacheKey, out, ttl)
	return nil
}

func (c *Client) fetchWithRetry(ctx context.Context, urlStr string, out any) error {
	maxRetries := 5
	for attempt := range maxRetries {
		if err := c.waitRateLimit(ctx); err != nil {
			return err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, urlStr, nil)
		if err != nil {
			return fmt.Errorf("failed to create jikan request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			if attempt < maxRetries-1 && IsRetryableError(err) {
				if retryErr := waitForRetry(ctx, retryDelay(attempt)); retryErr != nil {
					return retryErr
				}
				continue
			}

			return fmt.Errorf("jikan api error: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			apiErr := &APIError{StatusCode: resp.StatusCode, URL: urlStr}
			retryable := isRetryableStatus(resp.StatusCode)

			retryAfter := time.Duration(0)
			if parsed, ok := parseRetryAfter(resp.Header.Get("Retry-After")); ok {
				retryAfter = parsed
			}

			resp.Body.Close()

			if retryable && attempt < maxRetries-1 {
				delay := retryDelay(attempt)
				if retryAfter > delay {
					delay = retryAfter
				}

				if retryErr := waitForRetry(ctx, delay); retryErr != nil {
					return retryErr
				}

				continue
			}

			return apiErr
		}

		err = json.NewDecoder(resp.Body).Decode(out)
		resp.Body.Close()
		if err == nil {
			return nil
		}

		if attempt < maxRetries-1 {
			if retryErr := waitForRetry(ctx, retryDelay(attempt)); retryErr != nil {
				return retryErr
			}
			continue
		}

		return fmt.Errorf("failed to decode jikan response: %w", err)
	}

	return fmt.Errorf("max retries exceeded for %s", urlStr)
}
