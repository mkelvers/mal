package playback

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func (s *Service) fetchSkipSegments(ctx context.Context, malID int, episode string) []SkipSegment {
	if malID <= 0 || strings.TrimSpace(episode) == "" {
		return nil
	}

	endpoint := fmt.Sprintf("https://api.aniskip.com/v1/skip-times/%s/%s?types=op&types=ed", url.PathEscape(strconv.Itoa(malID)), url.PathEscape(episode))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil
	}

	type resultItem struct {
		SkipType string `json:"skip_type"`
		Interval struct {
			StartTime float64 `json:"start_time"`
			EndTime   float64 `json:"end_time"`
		} `json:"interval"`
	}
	type apiResponse struct {
		Found  bool         `json:"found"`
		Result []resultItem `json:"results"`
	}

	var parsed apiResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil
	}

	segments := make([]SkipSegment, 0, len(parsed.Result))
	for _, item := range parsed.Result {
		if item.Interval.EndTime <= item.Interval.StartTime {
			continue
		}

		t := strings.ToLower(item.SkipType)
		if t != "op" && t != "ed" {
			continue
		}

		segments = append(segments, SkipSegment{
			Type:  t,
			Start: item.Interval.StartTime,
			End:   item.Interval.EndTime,
		})
	}

	return segments
}
