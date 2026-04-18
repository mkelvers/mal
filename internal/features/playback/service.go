package playback

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mal/internal/database"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"mal/internal/jikan"
)

type Service struct {
	allAnimeClient *allAnimeClient
	jikanClient    *jikan.Client
	httpClient     *http.Client
	db             database.Querier
}

type sourceScore struct {
	source        StreamSource
	total         int
	typeScore     int
	providerScore int
	qualityScore  int
	refererScore  int
}

func NewService(jikanClient *jikan.Client, db database.Querier) *Service {
	return &Service{
		allAnimeClient: newAllAnimeClient(),
		jikanClient:    jikanClient,
		httpClient:     &http.Client{Timeout: 12 * time.Second},
		db:             db,
	}
}

func (s *Service) BuildWatchPageData(ctx context.Context, malID int, title string, episode string, mode string, userID string) (WatchPageData, error) {
	if malID <= 0 {
		return WatchPageData{}, errors.New("invalid mal id")
	}

	normalizedMode := normalizeMode(mode)
	if normalizedMode == "" {
		normalizedMode = "dub"
	}

	normalizedEpisode := strings.TrimSpace(episode)
	if normalizedEpisode == "" {
		normalizedEpisode = "1"
	}

	showID, resolvedTitle, err := s.resolveShow(ctx, malID, title)
	if err != nil {
		return WatchPageData{}, err
	}

	modeSources := make(map[string]ModeSource)
	for _, sourceMode := range []string{"dub", "sub"} {
		resolved, resolveErr := s.resolveModeSource(ctx, showID, normalizedEpisode, sourceMode, "best")
		if resolveErr != nil {
			continue
		}

		if strings.ToLower(resolved.Type) == "embed" {
			continue
		}

		modeSources[sourceMode] = ModeSource{
			URL:       resolved.URL,
			Referer:   resolved.Referer,
			Subtitles: toSubtitleItems(resolved),
		}
	}

	if len(modeSources) == 0 {
		return WatchPageData{}, errors.New("no direct playable sources available")
	}

	availableModes := availableModes(modeSources)
	initialMode := selectInitialMode(normalizedMode, modeSources)

	episodes := s.fetchEpisodeList(ctx, malID)
	if len(episodes) == 0 {
		episodeNumbers := s.fetchModeEpisodes(ctx, showID, initialMode)
		episodes = fallbackEpisodeList(episodeNumbers)
	}

	segments := s.fetchSkipSegments(ctx, malID, normalizedEpisode)

	currentStatus := ""
	startTimeSeconds := 0.0
	if userID != "" && s.db != nil {
		entry, err := s.db.GetWatchListEntry(ctx, database.GetWatchListEntryParams{
			UserID:  userID,
			AnimeID: int64(malID),
		})
		if err == nil {
			currentStatus = entry.Status
			if entry.CurrentEpisode.Valid && strconv.FormatInt(entry.CurrentEpisode.Int64, 10) == normalizedEpisode && entry.CurrentTimeSeconds > 0 {
				startTimeSeconds = entry.CurrentTimeSeconds
			}
		}
	}

	watchTitle := strings.TrimSpace(resolvedTitle)
	if watchTitle == "" {
		watchTitle = strings.TrimSpace(title)
	}
	if watchTitle == "" {
		watchTitle = fmt.Sprintf("MAL #%d", malID)
	}

	return WatchPageData{
		MalID:            malID,
		Title:            watchTitle,
		CurrentEpisode:   normalizedEpisode,
		StartTimeSeconds: startTimeSeconds,
		CurrentStatus:    currentStatus,
		InitialMode:      initialMode,
		AvailableModes:   availableModes,
		ModeSources:      modeSources,
		Episodes:         episodes,
		Segments:         segments,
	}, nil
}

func (s *Service) resolveShow(ctx context.Context, malID int, title string) (string, string, error) {
	malText := strconv.Itoa(malID)
	modeCandidates := []string{"sub", "dub"}
	for _, mode := range modeCandidates {
		results, err := s.allAnimeClient.Search(ctx, title, mode)
		if err != nil {
			continue
		}

		for _, result := range results {
			if strings.TrimSpace(result.MalID) == malText && strings.TrimSpace(result.ID) != "" {
				return result.ID, result.Name, nil
			}
		}
	}

	if strings.TrimSpace(title) != "" {
		for _, mode := range modeCandidates {
			results, err := s.allAnimeClient.Search(ctx, title, mode)
			if err != nil || len(results) == 0 {
				continue
			}

			best := results[0]
			if strings.TrimSpace(best.ID) != "" {
				return best.ID, best.Name, nil
			}
		}
	}

	return "", "", errors.New("unable to resolve allanime show")
}

func (s *Service) resolveModeSource(ctx context.Context, showID string, episode string, mode string, quality string) (StreamSource, error) {
	sources, err := s.allAnimeClient.GetEpisodeSources(ctx, showID, episode, mode)
	if err != nil {
		return StreamSource{}, err
	}

	ranked, err := rankSources(sources, quality)
	if err != nil {
		return StreamSource{}, err
	}

	selected, _, err := s.choosePlaybackSource(ctx, ranked)
	if err != nil {
		return StreamSource{}, err
	}

	return selected, nil
}

func (s *Service) choosePlaybackSource(ctx context.Context, ranked []sourceScore) (StreamSource, string, error) {
	if len(ranked) == 0 {
		return StreamSource{}, "", errors.New("no ranked sources available")
	}

	embedCandidates := make([]StreamSource, 0)
	for _, candidate := range ranked {
		source := candidate.source
		sourceType := strings.ToLower(source.Type)

		switch sourceType {
		case "mp4", "m3u8":
			return source, "direct-media", nil
		case "embed":
			embedCandidates = append(embedCandidates, source)
		default:
			playable, contentType := s.probeDirectMedia(ctx, source)
			if playable {
				return normalizeSourceTypeFromProbe(source, contentType), "probed-media", nil
			}
		}
	}

	for _, embed := range embedCandidates {
		if s.probeEmbedSource(ctx, embed) {
			return embed, "embed-probed", nil
		}
	}

	if len(embedCandidates) > 0 {
		return embedCandidates[0], "embed-fallback", nil
	}

	return ranked[0].source, "ranked-fallback", nil
}

func (s *Service) probeDirectMedia(ctx context.Context, source StreamSource) (bool, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return false, ""
	}

	if source.Referer != "" {
		req.Header.Set("Referer", source.Referer)
	}
	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Range", "bytes=0-4095")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false, ""
	}
	defer resp.Body.Close()

	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(contentType, "video/") || strings.Contains(contentType, "mpegurl") {
		return true, contentType
	}

	prefix, err := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if err == nil {
		if isLikelyM3U8(prefix) {
			return true, "application/vnd.apple.mpegurl"
		}
		if isLikelyMP4(prefix) {
			return true, "video/mp4"
		}
	}

	finalURL := ""
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = strings.ToLower(resp.Request.URL.String())
	}

	if strings.Contains(finalURL, ".mp4") || strings.Contains(finalURL, ".m3u8") {
		return true, contentType
	}

	return false, contentType
}

func (s *Service) probeEmbedSource(ctx context.Context, source StreamSource) bool {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, source.URL, nil)
	if err != nil {
		return false
	}

	if source.Referer != "" {
		req.Header.Set("Referer", source.Referer)
	}
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return false
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
	if err != nil {
		return false
	}

	content := strings.ToLower(string(body))
	markers := []string{
		"file was deleted",
		"file has been deleted",
		"video was deleted",
		"video has been deleted",
		"video unavailable",
		"file not found",
		"this file does not exist",
		"resource unavailable",
	}
	for _, marker := range markers {
		if strings.Contains(content, marker) {
			return false
		}
	}

	return true
}

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

func (s *Service) fetchEpisodeList(ctx context.Context, malID int) []EpisodeListItem {
	if malID <= 0 {
		return nil
	}

	items := make([]EpisodeListItem, 0)
	for page := 1; page <= 20; page++ {
		result, err := s.jikanClient.GetEpisodes(ctx, malID, page)
		if err != nil {
			return items
		}

		for _, episode := range result.Data {
			if episode.MalID <= 0 {
				continue
			}

			items = append(items, EpisodeListItem{
				Number: strconv.Itoa(episode.MalID),
				Title:  strings.TrimSpace(episode.Title),
				Filler: episode.Filler,
				Recap:  episode.Recap,
				Order:  episode.MalID,
			})
		}

		if !result.Pagination.HasNextPage {
			break
		}
	}

	return items
}

func (s *Service) fetchModeEpisodes(ctx context.Context, showID string, mode string) []string {
	episodes, err := s.allAnimeClient.GetEpisodes(ctx, showID, mode)
	if err == nil && len(episodes) > 0 {
		return episodes
	}

	fallbackMode := "sub"
	if mode == "sub" {
		fallbackMode = "dub"
	}

	fallbackEpisodes, fallbackErr := s.allAnimeClient.GetEpisodes(ctx, showID, fallbackMode)
	if fallbackErr != nil {
		return nil
	}

	return fallbackEpisodes
}

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

		rewritten, rewriteErr := rewritePlaylist(string(body), targetURL, referer)
		if rewriteErr != nil {
			return 0, nil, nil, nil, fmt.Errorf("rewrite playlist failed: %w", rewriteErr)
		}

		headers := cloneHeaders(resp.Header)
		headers.Set("Content-Type", "application/vnd.apple.mpegurl")
		return resp.StatusCode, headers, []byte(rewritten), nil, nil
	}

	headers := cloneHeaders(resp.Header)
	return resp.StatusCode, headers, nil, resp.Body, nil
}

func fallbackEpisodeList(episodeNumbers []string) []EpisodeListItem {
	items := make([]EpisodeListItem, 0, len(episodeNumbers))
	for idx, number := range episodeNumbers {
		trimmed := strings.TrimSpace(number)
		if trimmed == "" {
			continue
		}

		items = append(items, EpisodeListItem{
			Number: trimmed,
			Title:  "",
			Filler: false,
			Recap:  false,
			Order:  idx + 1,
		})
	}

	return items
}

func normalizeMode(raw string) string {
	lower := strings.ToLower(strings.TrimSpace(raw))
	if lower == "sub" || lower == "dub" {
		return lower
	}

	return lower
}

func toSubtitleItems(source StreamSource) []SubtitleItem {
	items := make([]SubtitleItem, 0, len(source.Subtitles))
	for _, subtitle := range source.Subtitles {
		targetURL := strings.TrimSpace(subtitle.URL)
		if targetURL == "" {
			continue
		}

		items = append(items, SubtitleItem{
			Lang:    strings.TrimSpace(subtitle.Lang),
			URL:     targetURL,
			Referer: source.Referer,
		})
	}

	return items
}

func availableModes(modeSources map[string]ModeSource) []string {
	ordered := make([]string, 0, len(modeSources))
	if _, ok := modeSources["dub"]; ok {
		ordered = append(ordered, "dub")
	}
	if _, ok := modeSources["sub"]; ok {
		ordered = append(ordered, "sub")
	}

	extra := make([]string, 0)
	for mode := range modeSources {
		if mode == "dub" || mode == "sub" {
			continue
		}
		extra = append(extra, mode)
	}
	sort.Strings(extra)

	return append(ordered, extra...)
}

func selectInitialMode(requestedMode string, modeSources map[string]ModeSource) string {
	normalizedRequested := normalizeMode(requestedMode)
	if normalizedRequested != "" {
		if _, ok := modeSources[normalizedRequested]; ok {
			return normalizedRequested
		}
	}

	if _, ok := modeSources["dub"]; ok {
		return "dub"
	}
	if _, ok := modeSources["sub"]; ok {
		return "sub"
	}

	for mode := range modeSources {
		return mode
	}

	return "dub"
}

func rankSources(sources []StreamSource, quality string) ([]sourceScore, error) {
	filtered := make([]StreamSource, 0, len(sources))
	seen := make(map[string]struct{})

	for _, source := range sources {
		if source.URL == "" {
			continue
		}
		if _, exists := seen[source.URL]; exists {
			continue
		}
		seen[source.URL] = struct{}{}
		filtered = append(filtered, source)
	}

	if len(filtered) == 0 {
		return nil, errors.New("no playable sources available")
	}

	targetQuality := normalizeQuality(quality)
	scored := make([]sourceScore, 0, len(filtered))
	for _, source := range filtered {
		typeScore := sourceTypePriority(source.Type)
		providerScore := providerPriority(source.Provider)
		qualityScore := sourceQualityPriority(source.Quality, targetQuality)
		refererScore := 0
		if source.Referer != "" {
			refererScore = 20
		}

		total := typeScore + providerScore + qualityScore + refererScore
		scored = append(scored, sourceScore{
			source:        source,
			total:         total,
			typeScore:     typeScore,
			providerScore: providerScore,
			qualityScore:  qualityScore,
			refererScore:  refererScore,
		})
	}

	sort.SliceStable(scored, func(i int, j int) bool {
		return scored[i].total > scored[j].total
	})

	return scored, nil
}

func normalizeQuality(quality string) string {
	lower := strings.ToLower(strings.TrimSpace(quality))
	if lower == "" {
		return "best"
	}

	return lower
}

func sourceTypePriority(sourceType string) int {
	switch strings.ToLower(sourceType) {
	case "mp4":
		return 500
	case "m3u8":
		return 450
	case "unknown":
		return 300
	case "embed":
		return 100
	default:
		return 200
	}
}

func providerPriority(provider string) int {
	switch strings.ToLower(provider) {
	case "s-mp4":
		return 120
	case "default":
		return 115
	case "luf-mp4":
		return 110
	case "vid-mp4":
		return 105
	case "yt-mp4":
		return 100
	case "mp4":
		return 95
	case "uv-mp4":
		return 90
	case "hls":
		return 80
	case "sw":
		return 40
	case "ok":
		return 35
	case "ss-hls":
		return 30
	default:
		return 60
	}
}

func sourceQualityPriority(sourceQuality string, targetQuality string) int {
	qualityValue := parseQualityValue(sourceQuality)

	switch targetQuality {
	case "best":
		return qualityValue
	case "worst":
		return -qualityValue
	default:
		if qualityMatches(sourceQuality, targetQuality) {
			return 2000 + qualityValue
		}

		return -300 + qualityValue
	}
}

func parseQualityValue(rawQuality string) int {
	lower := strings.ToLower(rawQuality)
	var digits strings.Builder

	for _, char := range lower {
		if char >= '0' && char <= '9' {
			digits.WriteRune(char)
			continue
		}
		if digits.Len() > 0 {
			break
		}
	}

	if digits.Len() > 0 {
		value, err := strconv.Atoi(digits.String())
		if err == nil {
			return value
		}
	}

	if lower == "auto" {
		return 240
	}

	return 0
}

func qualityMatches(sourceQuality string, targetQuality string) bool {
	sourceLower := strings.ToLower(sourceQuality)
	targetLower := strings.ToLower(targetQuality)

	if sourceLower == "" {
		return false
	}

	if strings.Contains(sourceLower, targetLower) {
		return true
	}

	sourceDigits := extractDigits(sourceLower)
	targetDigits := extractDigits(targetLower)

	return sourceDigits != "" && sourceDigits == targetDigits
}

func extractDigits(value string) string {
	var digits strings.Builder
	for _, char := range value {
		if char >= '0' && char <= '9' {
			digits.WriteRune(char)
			continue
		}
		if digits.Len() > 0 {
			break
		}
	}

	return digits.String()
}

func normalizeSourceTypeFromProbe(source StreamSource, contentType string) StreamSource {
	lower := strings.ToLower(contentType)
	if strings.Contains(lower, "video/mp4") {
		source.Type = "mp4"
		return source
	}

	if strings.Contains(lower, "mpegurl") {
		source.Type = "m3u8"
		return source
	}

	return source
}

func isLikelyMP4(payload []byte) bool {
	if len(payload) < 12 {
		return false
	}

	return bytes.Equal(payload[4:8], []byte("ftyp"))
}

func isLikelyM3U8(payload []byte) bool {
	trimmed := strings.TrimSpace(string(payload))
	return strings.HasPrefix(trimmed, "#EXTM3U")
}

func isM3U8(targetURL string, contentType string) bool {
	lowerURL := strings.ToLower(targetURL)
	lowerType := strings.ToLower(contentType)
	if strings.Contains(lowerURL, ".m3u8") {
		return true
	}

	return strings.Contains(lowerType, "application/vnd.apple.mpegurl") || strings.Contains(lowerType, "application/x-mpegurl")
}

func rewritePlaylist(content string, baseURL string, referer string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}

	var out strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			out.WriteString(line)
			out.WriteString("\n")
			continue
		}

		relativeURL, parseErr := url.Parse(trimmed)
		if parseErr != nil {
			out.WriteString(line)
			out.WriteString("\n")
			continue
		}

		absolute := base.ResolveReference(relativeURL).String()
		proxied := "/watch/proxy/segment?u=" + url.QueryEscape(absolute)
		if referer != "" {
			proxied += "&r=" + url.QueryEscape(referer)
		}

		out.WriteString(proxied)
		out.WriteString("\n")
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return out.String(), nil
}

func cloneHeaders(src http.Header) http.Header {
	dst := make(http.Header)
	for key, values := range src {
		lower := strings.ToLower(key)
		if lower == "connection" || lower == "transfer-encoding" || lower == "keep-alive" || lower == "proxy-authenticate" || lower == "proxy-authorization" || lower == "te" || lower == "trailers" || lower == "upgrade" {
			continue
		}

		for _, value := range values {
			dst.Add(key, value)
		}
	}

	return dst
}
