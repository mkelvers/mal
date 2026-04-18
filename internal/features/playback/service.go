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
	"sync"
	"time"
)

const (
	showResolutionCacheTTL = 12 * time.Hour
	playbackDataCacheTTL   = 2 * time.Minute
	providerProbeTimeout   = 3 * time.Second
)

type Service struct {
	allAnimeClient *allAnimeClient
	httpClient     *http.Client
	db             database.Querier

	cacheMu           sync.RWMutex
	showResolution    map[int]showResolutionCacheItem
	playbackDataCache map[string]playbackDataCacheItem
}

type sourceScore struct {
	source        StreamSource
	total         int
	typeScore     int
	providerScore int
	qualityScore  int
	refererScore  int
}

type showResolutionCacheItem struct {
	ShowID    string
	Title     string
	ExpiresAt time.Time
}

type playbackDataCacheItem struct {
	Data      playbackBaseData
	ExpiresAt time.Time
}

type playbackBaseData struct {
	Title          string
	AvailableModes []string
	ModeSources    map[string]ModeSource
	Segments       []SkipSegment
}

type modeSourceResult struct {
	Mode   string
	Source ModeSource
	OK     bool
}

type searchModeResult struct {
	Mode    string
	Results []searchResult
	Err     error
}

type directProbeResult struct {
	Playable    bool
	ContentType string
}

type userPlaybackState struct {
	CurrentStatus    string
	StartTimeSeconds float64
}

func NewService(db database.Querier) *Service {
	return &Service{
		allAnimeClient:    newAllAnimeClient(),
		httpClient:        &http.Client{Timeout: 12 * time.Second},
		db:                db,
		showResolution:    make(map[int]showResolutionCacheItem),
		playbackDataCache: make(map[string]playbackDataCacheItem),
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

	userStateCh := s.fetchUserPlaybackStateAsync(ctx, userID, malID, normalizedEpisode)

	cacheKey := playbackDataCacheKey(malID, normalizedEpisode)
	baseData, cacheHit := s.getPlaybackBaseDataCache(cacheKey)
	if !cacheHit {
		showID, resolvedTitle, err := s.resolveShowCached(ctx, malID, title)
		if err != nil {
			return WatchPageData{}, err
		}

		modeSources, segments := s.fetchPlaybackSourcesAndSegments(ctx, showID, malID, normalizedEpisode)
		if len(modeSources) == 0 {
			return WatchPageData{}, errors.New("no direct playable sources available")
		}

		watchTitle := strings.TrimSpace(resolvedTitle)
		if watchTitle == "" {
			watchTitle = strings.TrimSpace(title)
		}
		if watchTitle == "" {
			watchTitle = fmt.Sprintf("MAL #%d", malID)
		}

		baseData = playbackBaseData{
			Title:          watchTitle,
			AvailableModes: availableModes(modeSources),
			ModeSources:    modeSources,
			Segments:       segments,
		}

		s.setPlaybackBaseDataCache(cacheKey, baseData)
	}

	initialMode := selectInitialMode(normalizedMode, baseData.ModeSources)

	userState := userPlaybackState{}
	if userStateCh != nil {
		userState = <-userStateCh
	}

	return WatchPageData{
		MalID:            malID,
		Title:            baseData.Title,
		CurrentEpisode:   normalizedEpisode,
		StartTimeSeconds: userState.StartTimeSeconds,
		CurrentStatus:    userState.CurrentStatus,
		InitialMode:      initialMode,
		AvailableModes:   cloneStringSlice(baseData.AvailableModes),
		ModeSources:      cloneModeSources(baseData.ModeSources),
		Segments:         cloneSegments(baseData.Segments),
	}, nil
}

func playbackDataCacheKey(malID int, episode string) string {
	return fmt.Sprintf("%d:%s", malID, episode)
}

func (s *Service) fetchUserPlaybackStateAsync(ctx context.Context, userID string, malID int, episode string) <-chan userPlaybackState {
	if userID == "" || s.db == nil {
		return nil
	}

	resultCh := make(chan userPlaybackState, 1)
	go func() {
		state := userPlaybackState{}

		entry, err := s.db.GetWatchListEntry(ctx, database.GetWatchListEntryParams{
			UserID:  userID,
			AnimeID: int64(malID),
		})
		if err == nil {
			state.CurrentStatus = entry.Status
			if entry.CurrentEpisode.Valid && strconv.FormatInt(entry.CurrentEpisode.Int64, 10) == episode && entry.CurrentTimeSeconds > 0 {
				state.StartTimeSeconds = entry.CurrentTimeSeconds
			}
		}

		if state.StartTimeSeconds <= 0 {
			continueEntry, continueErr := s.db.GetContinueWatchingEntry(ctx, database.GetContinueWatchingEntryParams{
				UserID:  userID,
				AnimeID: int64(malID),
			})
			if continueErr == nil && continueEntry.CurrentEpisode.Valid && strconv.FormatInt(continueEntry.CurrentEpisode.Int64, 10) == episode && continueEntry.CurrentTimeSeconds > 0 {
				state.StartTimeSeconds = continueEntry.CurrentTimeSeconds
			}
		}

		resultCh <- state
	}()

	return resultCh
}

func (s *Service) getPlaybackBaseDataCache(key string) (playbackBaseData, bool) {
	now := time.Now()

	s.cacheMu.RLock()
	item, ok := s.playbackDataCache[key]
	s.cacheMu.RUnlock()
	if !ok {
		return playbackBaseData{}, false
	}

	if now.After(item.ExpiresAt) {
		s.cacheMu.Lock()
		current, exists := s.playbackDataCache[key]
		if exists && time.Now().After(current.ExpiresAt) {
			delete(s.playbackDataCache, key)
		}
		s.cacheMu.Unlock()
		return playbackBaseData{}, false
	}

	return clonePlaybackBaseData(item.Data), true
}

func (s *Service) setPlaybackBaseDataCache(key string, data playbackBaseData) {
	s.cacheMu.Lock()
	s.playbackDataCache[key] = playbackDataCacheItem{
		Data:      clonePlaybackBaseData(data),
		ExpiresAt: time.Now().Add(playbackDataCacheTTL),
	}
	s.cacheMu.Unlock()
}

func (s *Service) resolveShowCached(ctx context.Context, malID int, title string) (string, string, error) {
	now := time.Now()

	s.cacheMu.RLock()
	item, ok := s.showResolution[malID]
	s.cacheMu.RUnlock()

	if ok && now.Before(item.ExpiresAt) && strings.TrimSpace(item.ShowID) != "" {
		return item.ShowID, item.Title, nil
	}

	showID, resolvedTitle, err := s.resolveShow(ctx, malID, title)
	if err != nil {
		return "", "", err
	}

	s.cacheMu.Lock()
	s.showResolution[malID] = showResolutionCacheItem{
		ShowID:    showID,
		Title:     resolvedTitle,
		ExpiresAt: time.Now().Add(showResolutionCacheTTL),
	}
	s.cacheMu.Unlock()

	return showID, resolvedTitle, nil
}

func (s *Service) fetchPlaybackSourcesAndSegments(ctx context.Context, showID string, malID int, episode string) (map[string]ModeSource, []SkipSegment) {
	modeCh := make(chan modeSourceResult, 2)
	probeCache := make(map[string]directProbeResult)
	probeCacheMu := sync.Mutex{}

	for _, mode := range []string{"dub", "sub"} {
		modeValue := mode
		go func() {
			resolved, err := s.resolveModeSourceWithCache(ctx, showID, episode, modeValue, "best", probeCache, &probeCacheMu)
			if err != nil {
				modeCh <- modeSourceResult{Mode: modeValue, OK: false}
				return
			}

			if strings.ToLower(resolved.Type) == "embed" {
				modeCh <- modeSourceResult{Mode: modeValue, OK: false}
				return
			}

			modeCh <- modeSourceResult{
				Mode: modeValue,
				Source: ModeSource{
					URL:       resolved.URL,
					Referer:   resolved.Referer,
					Subtitles: toSubtitleItems(resolved),
				},
				OK: true,
			}
		}()
	}

	segmentsCh := make(chan []SkipSegment, 1)
	go func() {
		segmentsCh <- s.fetchSkipSegments(ctx, malID, episode)
	}()

	modeSources := make(map[string]ModeSource)
	for range 2 {
		result := <-modeCh
		if !result.OK {
			continue
		}
		modeSources[result.Mode] = result.Source
	}

	segments := <-segmentsCh
	return modeSources, segments
}

func clonePlaybackBaseData(data playbackBaseData) playbackBaseData {
	return playbackBaseData{
		Title:          data.Title,
		AvailableModes: cloneStringSlice(data.AvailableModes),
		ModeSources:    cloneModeSources(data.ModeSources),
		Segments:       cloneSegments(data.Segments),
	}
}

func cloneStringSlice(items []string) []string {
	if len(items) == 0 {
		return nil
	}

	cloned := make([]string, len(items))
	copy(cloned, items)
	return cloned
}

func cloneModeSources(modeSources map[string]ModeSource) map[string]ModeSource {
	if len(modeSources) == 0 {
		return nil
	}

	cloned := make(map[string]ModeSource, len(modeSources))
	for mode, source := range modeSources {
		cloned[mode] = ModeSource{
			URL:       source.URL,
			Referer:   source.Referer,
			Subtitles: cloneSubtitleItems(source.Subtitles),
		}
	}

	return cloned
}

func cloneSubtitleItems(items []SubtitleItem) []SubtitleItem {
	if len(items) == 0 {
		return nil
	}

	cloned := make([]SubtitleItem, len(items))
	copy(cloned, items)
	return cloned
}

func cloneSegments(segments []SkipSegment) []SkipSegment {
	if len(segments) == 0 {
		return nil
	}

	cloned := make([]SkipSegment, len(segments))
	copy(cloned, segments)
	return cloned
}

func (s *Service) resolveShow(ctx context.Context, malID int, title string) (string, string, error) {
	malText := strconv.Itoa(malID)
	modeCandidates := []string{"sub", "dub"}

	resultsByMode := make(map[string][]searchResult, len(modeCandidates))
	searchCh := make(chan searchModeResult, len(modeCandidates))

	var wg sync.WaitGroup
	for _, mode := range modeCandidates {
		modeValue := mode
		wg.Add(1)
		go func() {
			defer wg.Done()
			results, err := s.allAnimeClient.Search(ctx, title, modeValue)
			searchCh <- searchModeResult{Mode: modeValue, Results: results, Err: err}
		}()
	}

	wg.Wait()
	close(searchCh)

	for result := range searchCh {
		if result.Err != nil {
			continue
		}
		resultsByMode[result.Mode] = result.Results
	}

	for _, mode := range modeCandidates {
		for _, result := range resultsByMode[mode] {
			if strings.TrimSpace(result.MalID) == malText && strings.TrimSpace(result.ID) != "" {
				return result.ID, result.Name, nil
			}
		}
	}

	if strings.TrimSpace(title) != "" {
		for _, mode := range modeCandidates {
			results := resultsByMode[mode]
			if len(results) == 0 {
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

func (s *Service) resolveModeSourceWithCache(
	ctx context.Context,
	showID string,
	episode string,
	mode string,
	quality string,
	probeCache map[string]directProbeResult,
	probeCacheMu *sync.Mutex,
) (StreamSource, error) {
	sources, err := s.allAnimeClient.GetEpisodeSources(ctx, showID, episode, mode)
	if err != nil {
		return StreamSource{}, err
	}

	ranked, err := rankSources(sources, quality)
	if err != nil {
		return StreamSource{}, err
	}

	selected, _, err := s.choosePlaybackSourceWithCache(ctx, ranked, probeCache, probeCacheMu)
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

func (s *Service) choosePlaybackSourceWithCache(
	ctx context.Context,
	ranked []sourceScore,
	probeCache map[string]directProbeResult,
	probeCacheMu *sync.Mutex,
) (StreamSource, string, error) {
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
			playable, contentType := s.probeDirectMediaCached(ctx, source, probeCache, probeCacheMu)
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

func (s *Service) probeDirectMediaCached(
	ctx context.Context,
	source StreamSource,
	probeCache map[string]directProbeResult,
	probeCacheMu *sync.Mutex,
) (bool, string) {
	cacheKey := strings.TrimSpace(source.URL)
	if cacheKey == "" {
		return s.probeDirectMedia(ctx, source)
	}

	probeCacheMu.Lock()
	cached, ok := probeCache[cacheKey]
	probeCacheMu.Unlock()
	if ok {
		return cached.Playable, cached.ContentType
	}

	playable, contentType := s.probeDirectMedia(ctx, source)

	probeCacheMu.Lock()
	probeCache[cacheKey] = directProbeResult{Playable: playable, ContentType: contentType}
	probeCacheMu.Unlock()

	return playable, contentType
}

func (s *Service) probeDirectMedia(ctx context.Context, source StreamSource) (bool, string) {
	probeCtx, cancel := context.WithTimeout(ctx, providerProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, source.URL, nil)
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
	probeCtx, cancel := context.WithTimeout(ctx, providerProbeTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(probeCtx, http.MethodGet, source.URL, nil)
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
