package playback

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mal/internal/database"
	"net/http"
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
	sqlDB          *sql.DB
	db             database.Querier
	proxyTokens    *proxyTokenSigner
	proxyHostMu    sync.RWMutex
	proxyHostCache map[string]proxyHostCacheItem

	cacheMu           sync.RWMutex
	showResolution    map[int]showResolutionCacheItem
	playbackDataCache map[string]playbackDataCacheItem
}

type Config struct {
	ProxyTokenSecret string
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

type proxyHostCacheItem struct {
	Allowed   bool
	ExpiresAt time.Time
}

type userPlaybackState struct {
	CurrentStatus    string
	StartTimeSeconds float64
}

func NewService(db database.Querier, sqlDB *sql.DB, cfg Config) *Service {
	proxyTokens, err := newProxyTokenSigner(cfg.ProxyTokenSecret)
	if err != nil {
		panic(fmt.Sprintf("failed to initialize proxy token signer: %v", err))
	}

	return &Service{
		allAnimeClient:    newAllAnimeClient(),
		httpClient:        &http.Client{Timeout: 12 * time.Second},
		sqlDB:             sqlDB,
		db:                db,
		proxyTokens:       proxyTokens,
		proxyHostCache:    make(map[string]proxyHostCacheItem),
		showResolution:    make(map[int]showResolutionCacheItem),
		playbackDataCache: make(map[string]playbackDataCacheItem),
	}
}

func (s *Service) BuildWatchPageData(ctx context.Context, malID int, titleCandidates []string, episode string, mode string, userID string) (WatchPageData, error) {
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
		showID, resolvedTitle, err := s.resolveShowCached(ctx, malID, titleCandidates)
		if err != nil {
			return WatchPageData{}, err
		}

		modeSources, segments := s.fetchPlaybackSourcesAndSegments(ctx, showID, malID, normalizedEpisode)
		if len(modeSources) == 0 {
			return WatchPageData{}, errors.New("no direct playable sources available")
		}

		watchTitle := strings.TrimSpace(resolvedTitle)
		if watchTitle == "" {
			watchTitle = firstNonEmptyTitle(titleCandidates)
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

	clientModeSources, err := s.buildClientModeSources(baseData.ModeSources)
	if err != nil {
		return WatchPageData{}, err
	}

	if _, ok := clientModeSources[initialMode]; !ok {
		return WatchPageData{}, errors.New("stream mode unavailable")
	}

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
		ModeSources:      clientModeSources,
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

func (s *Service) resolveShowCached(ctx context.Context, malID int, titleCandidates []string) (string, string, error) {
	s.cacheMu.RLock()
	item, ok := s.showResolution[malID]
	s.cacheMu.RUnlock()

	now := time.Now()
	if ok && now.Before(item.ExpiresAt) && strings.TrimSpace(item.ShowID) != "" {
		return item.ShowID, item.Title, nil
	}

	showID, resolvedTitle, err := s.resolveShow(ctx, malID, titleCandidates)
	if err != nil {
		return "", "", err
	}

	s.cacheMu.Lock()
	s.showResolution[malID] = showResolutionCacheItem{
		ShowID:    showID,
		Title:     resolvedTitle,
		ExpiresAt: now.Add(showResolutionCacheTTL),
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
