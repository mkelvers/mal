package playback

import (
	"context"
	"net/http"
)

func doProxiedRequest(ctx context.Context, client *http.Client, url string, referer string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", defaultUserAgent)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}
