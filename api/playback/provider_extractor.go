package playback

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type providerExtractor struct {
	httpClient *http.Client
	baseURL    string
	referer    string
}

func newProviderExtractor() *providerExtractor {
	return &providerExtractor{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		baseURL:    allAnimeBaseURL,
		referer:    allAnimeReferer,
	}
}

func (e *providerExtractor) ExtractVideoLinks(ctx context.Context, providerPath string) ([]StreamSource, error) {
	endpoint := e.baseURL + providerPath

	var resp *http.Response
	var err error

	for attempt := range 3 {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(time.Duration(attempt) * 2 * time.Second):
			}
		}

		resp, err = doProxiedRequest(ctx, e.httpClient, endpoint, e.referer)
		if err == nil {
			break
		}

		if attempt == 2 {
			return nil, fmt.Errorf("fetch provider response: %w", err)
		}
	}

	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read provider response: %w", err)
	}

	return e.parseProviderResponse(ctx, string(body))
}

func (e *providerExtractor) parseProviderResponse(ctx context.Context, response string) ([]StreamSource, error) {
	sources := make([]StreamSource, 0)
	providerReferer := e.referer

	refererPattern := regexp.MustCompile(`"Referer":"([^"]+)"`)
	if match := refererPattern.FindStringSubmatch(response); len(match) >= 2 {
		providerReferer = strings.ReplaceAll(match[1], `\/`, "/")
	}
	if providerReferer == "" {
		providerReferer = e.referer
	}

	linkPattern := regexp.MustCompile(`"link":"([^"]+)","resolutionStr":"([^"]+)"`)
	for _, match := range linkPattern.FindAllStringSubmatch(response, -1) {
		if len(match) < 3 {
			continue
		}

		link := strings.ReplaceAll(match[1], `\/`, "/")
		quality := strings.TrimSpace(match[2])
		sourceType := detectStreamType(link)
		if sourceType == "unknown" {
			sourceType = detectEmbedType(link)
		}

		sources = append(sources, StreamSource{
			URL:      link,
			Quality:  quality,
			Provider: "wixmp",
			Type:     sourceType,
			Referer:  providerReferer,
		})
	}

	hlsPattern := regexp.MustCompile(`"url":"([^"]+)","hardsub_lang":"en-US"`)
	for _, match := range hlsPattern.FindAllStringSubmatch(response, -1) {
		if len(match) < 2 {
			continue
		}

		playlistURL := strings.ReplaceAll(match[1], `\/`, "/")
		if strings.Contains(playlistURL, "master.m3u8") {
			parsed, err := e.parseM3U8(ctx, playlistURL, providerReferer)
			if err == nil {
				sources = append(sources, parsed...)
			}
			continue
		}

		sources = append(sources, StreamSource{
			URL:      playlistURL,
			Quality:  "auto",
			Provider: "hls",
			Type:     "m3u8",
			Referer:  providerReferer,
		})
	}

	subtitlePattern := regexp.MustCompile(`"subtitles":\[(.*?)\]`)
	if subtitleMatch := subtitlePattern.FindStringSubmatch(response); len(subtitleMatch) >= 2 {
		subtitles := make([]Subtitle, 0)
		subtitleEntryPattern := regexp.MustCompile(`"lang":"([^"]+)".*?"src":"([^"]+)"`)
		for _, entry := range subtitleEntryPattern.FindAllStringSubmatch(subtitleMatch[1], -1) {
			if len(entry) < 3 {
				continue
			}

			subtitles = append(subtitles, Subtitle{
				Lang: strings.TrimSpace(entry[1]),
				URL:  strings.ReplaceAll(entry[2], `\/`, "/"),
			})
		}

		if len(subtitles) > 0 {
			for idx := range sources {
				sources[idx].Subtitles = subtitles
			}
		}
	}

	return sources, nil
}

func (e *providerExtractor) parseM3U8(ctx context.Context, masterURL string, referer string) ([]StreamSource, error) {
	resp, err := doProxiedRequest(ctx, e.httpClient, masterURL, referer)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(body), "\n")
	baseURL := masterURL
	if idx := strings.LastIndex(masterURL, "/"); idx >= 0 {
		baseURL = masterURL[:idx+1]
	}

	currentBandwidth := 0
	sources := make([]StreamSource, 0)
	bwPattern := regexp.MustCompile(`BANDWIDTH=(\d+)`)

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#EXT-X-STREAM-INF") {
			match := bwPattern.FindStringSubmatch(trimmed)
			if len(match) >= 2 {
				value, convErr := strconv.Atoi(match[1])
				if convErr == nil {
					currentBandwidth = value
				}
			}
			continue
		}

		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}

		streamURL := trimmed
		if !strings.HasPrefix(streamURL, "http://") && !strings.HasPrefix(streamURL, "https://") {
			streamURL = baseURL + streamURL
		}

		quality := "auto"
		kbps := currentBandwidth / 1000
		switch {
		case kbps >= 8000:
			quality = "1080p"
		case kbps >= 5000:
			quality = "720p"
		case kbps >= 2500:
			quality = "480p"
		case kbps > 0:
			quality = "360p"
		}

		sources = append(sources, StreamSource{
			URL:      streamURL,
			Quality:  quality,
			Provider: "hls",
			Type:     "m3u8",
			Referer:  referer,
		})
	}

	return sources, nil
}
