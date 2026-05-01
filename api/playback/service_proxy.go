package playback

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

func (s *Service) ProxyStream(ctx context.Context, targetURL string, referer string, rangeHeader string) (int, http.Header, []byte, io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return 0, nil, nil, nil, fmt.Errorf("invalid upstream url: %w", err)
	}

	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	if rangeHeader != "" {
		req.Header.Set("Range", rangeHeader)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return 0, nil, nil, nil, fmt.Errorf("upstream request failed: %w", err)
	}

	if isM3U8(targetURL, resp.Header.Get("Content-Type")) {
		defer resp.Body.Close()
		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
		if readErr != nil {
			return 0, nil, nil, nil, fmt.Errorf("read playlist failed: %w", readErr)
		}

		rewritten, rewriteErr := s.rewritePlaylistWithTokens(ctx, string(body), targetURL, referer)
		if rewriteErr != nil {
			return 0, nil, nil, nil, fmt.Errorf("rewrite playlist failed: %w", rewriteErr)
		}

		headers := cloneHeaders(resp.Header)
		headers.Del("Content-Length")
		headers.Del("Transfer-Encoding")
		headers.Set("Content-Type", "application/vnd.apple.mpegurl")
		headers.Set("Content-Length", strconv.Itoa(len(rewritten)))
		return resp.StatusCode, headers, []byte(rewritten), nil, nil
	}

	headers := cloneHeaders(resp.Header)
	// Some upstream servers send transfer-encoding chunked, we should let go's http server handle it
	headers.Del("Transfer-Encoding")
	return resp.StatusCode, headers, nil, resp.Body, nil
}

func isM3U8(targetURL string, contentType string) bool {
	if strings.Contains(strings.ToLower(targetURL), ".m3u8") {
		return true
	}
	lowerType := strings.ToLower(contentType)
	return strings.Contains(lowerType, "application/vnd.apple.mpegurl") || strings.Contains(lowerType, "application/x-mpegurl")
}

var hopHeaders = map[string]struct{}{
	"connection":          {},
	"keep-alive":          {},
	"proxy-authenticate":  {},
	"proxy-authorization": {},
	"te":                  {},
	"trailers":            {},
	"upgrade":             {},
}

func cloneHeaders(src http.Header) http.Header {
	dst := make(http.Header)
	for key, values := range src {
		if _, ok := hopHeaders[strings.ToLower(key)]; ok {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
	return dst
}
