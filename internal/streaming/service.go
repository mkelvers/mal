package streaming

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/anacrolix/torrent/metainfo"
	"golang.org/x/net/proxy"

	"mal/internal/nyaa"
)

type Service struct {
	client         *torrent.Client
	nyaa           *nyaa.Client
	hls            *HLSTranscoder
	mu             sync.RWMutex
	activeTorrents map[string]*torrent.Torrent
	logger         *slog.Logger
}

type StreamInfo struct {
	InfoHash     string
	Name         string
	Size         int64
	Files        []FileInfo
	Progress     float64
	DownloadRate int64
	Peers        int
}

type FileInfo struct {
	Index int
	Path  string
	Size  int64
}

func NewService(logger *slog.Logger) (*Service, error) {
	cfg := torrent.NewDefaultClientConfig()
	cfg.ListenPort = 42069
	cfg.Seed = false
	cfg.NoUpload = true

	// Use temp directory for downloads
	cfg.DataDir = filepath.Join("/tmp", "mal-streams")

	// Configure SOCKS5 proxy if TORRENT_PROXY is set
	// Usage: export TORRENT_PROXY=socks5://127.0.0.1:1080
	// Start with: ssh -D 1080 -N user@your-vps
	if proxyURL := os.Getenv("TORRENT_PROXY"); proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("invalid TORRENT_PROXY url: %w", err)
		}

		dialer, err := proxy.FromURL(parsed, proxy.Direct)
		if err != nil {
			return nil, fmt.Errorf("failed to create proxy dialer: %w", err)
		}

		// Proxy HTTP requests (trackers, webseeds)
		cfg.HTTPProxy = func(*http.Request) (*url.URL, error) {
			return parsed, nil
		}

		// Proxy peer connections via DialContext
		if contextDialer, ok := dialer.(proxy.ContextDialer); ok {
			cfg.HTTPDialContext = contextDialer.DialContext
		}

		logger.Info("torrent proxy configured", "proxy", proxyURL)
	}

	client, err := torrent.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create torrent client: %w", err)
	}

	hls, err := NewHLSTranscoder(logger)
	if err != nil {
		logger.Warn("HLS transcoding unavailable", "error", err)
		// Continue without HLS - will fall back to direct streaming
	}

	return &Service{
		client:         client,
		nyaa:           nyaa.NewClient(),
		hls:            hls,
		activeTorrents: make(map[string]*torrent.Torrent),
		logger:         logger,
	}, nil
}

// SearchEpisode searches nyaa for torrents of a specific episode
func (s *Service) SearchEpisode(animeTitle string, episode int) ([]nyaa.Torrent, error) {
	return s.nyaa.SearchEpisode(animeTitle, episode)
}

// SearchAnime searches nyaa for torrents of an anime
func (s *Service) SearchAnime(query string) ([]nyaa.Torrent, error) {
	return s.nyaa.SearchAnime(query)
}

// AddMagnet adds a magnet link and returns stream info
func (s *Service) AddMagnet(ctx context.Context, magnetURI string) (*StreamInfo, error) {
	t, err := s.client.AddMagnet(magnetURI)
	if err != nil {
		return nil, fmt.Errorf("failed to add magnet: %w", err)
	}

	// Wait for metadata with timeout
	select {
	case <-t.GotInfo():
	case <-ctx.Done():
		t.Drop()
		return nil, ctx.Err()
	case <-time.After(60 * time.Second):
		t.Drop()
		return nil, fmt.Errorf("timeout waiting for torrent metadata")
	}

	infoHash := t.InfoHash().HexString()

	s.mu.Lock()
	s.activeTorrents[infoHash] = t
	s.mu.Unlock()

	return s.getStreamInfo(t), nil
}

// GetTorrent returns an active torrent by info hash
func (s *Service) GetTorrent(infoHash string) (*torrent.Torrent, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	t, ok := s.activeTorrents[infoHash]
	return t, ok
}

// StreamFile streams a specific file from a torrent
func (s *Service) StreamFile(w http.ResponseWriter, r *http.Request, infoHash string, fileIdx int) error {
	t, ok := s.GetTorrent(infoHash)
	if !ok {
		return fmt.Errorf("torrent not found: %s", infoHash)
	}

	files := t.Files()
	if fileIdx < 0 || fileIdx >= len(files) {
		return fmt.Errorf("invalid file index: %d", fileIdx)
	}

	file := files[fileIdx]
	reader := file.NewReader()
	reader.SetReadahead(file.Length() / 100) // 1% readahead
	reader.SetResponsive()

	// Determine content type
	contentType := "application/octet-stream"
	ext := strings.ToLower(filepath.Ext(file.Path()))
	switch ext {
	case ".mp4":
		contentType = "video/mp4"
	case ".mkv":
		contentType = "video/x-matroska"
	case ".webm":
		contentType = "video/webm"
	case ".avi":
		contentType = "video/x-msvideo"
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Accept-Ranges", "bytes")

	// Handle range requests for seeking
	http.ServeContent(w, r, file.Path(), time.Time{}, reader)
	return nil
}

// StreamVideo finds and streams the main video file from a torrent
func (s *Service) StreamVideo(w http.ResponseWriter, r *http.Request, infoHash string) error {
	t, ok := s.GetTorrent(infoHash)
	if !ok {
		return fmt.Errorf("torrent not found: %s", infoHash)
	}

	// Find the largest video file
	var bestFile *torrent.File
	var bestIdx int
	for i, f := range t.Files() {
		if isVideoFile(f.Path()) {
			if bestFile == nil || f.Length() > bestFile.Length() {
				bestFile = f
				bestIdx = i
			}
		}
	}

	if bestFile == nil {
		return fmt.Errorf("no video file found in torrent")
	}

	return s.StreamFile(w, r, infoHash, bestIdx)
}

// GetStreamInfo returns info about an active torrent
func (s *Service) GetStreamInfo(infoHash string) (*StreamInfo, error) {
	t, ok := s.GetTorrent(infoHash)
	if !ok {
		return nil, fmt.Errorf("torrent not found: %s", infoHash)
	}
	return s.getStreamInfo(t), nil
}

func (s *Service) getStreamInfo(t *torrent.Torrent) *StreamInfo {
	info := &StreamInfo{
		InfoHash: t.InfoHash().HexString(),
		Name:     t.Name(),
		Peers:    t.Stats().ActivePeers,
	}

	var totalLength, completed int64
	for i, f := range t.Files() {
		info.Files = append(info.Files, FileInfo{
			Index: i,
			Path:  f.Path(),
			Size:  f.Length(),
		})
		totalLength += f.Length()
		completed += f.BytesCompleted()
	}

	info.Size = totalLength
	if totalLength > 0 {
		info.Progress = float64(completed) / float64(totalLength) * 100
	}

	stats := t.Stats()
	info.DownloadRate = stats.ConnStats.BytesReadData.Int64()

	return info
}

// DropTorrent removes a torrent from the client
func (s *Service) DropTorrent(infoHash string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if t, ok := s.activeTorrents[infoHash]; ok {
		t.Drop()
		delete(s.activeTorrents, infoHash)
	}
}

// Close shuts down the torrent client
func (s *Service) Close() {
	s.mu.Lock()
	for _, t := range s.activeTorrents {
		t.Drop()
	}
	s.activeTorrents = nil
	s.mu.Unlock()

	if s.hls != nil {
		s.hls.Shutdown()
	}

	s.client.Close()
}

func isVideoFile(path string) bool {
	videoExts := []string{".mp4", ".mkv", ".avi", ".mov", ".webm", ".wmv", ".flv"}
	lower := strings.ToLower(path)
	for _, ext := range videoExts {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// ParseMagnetHash extracts info hash from a magnet URI
func ParseMagnetHash(magnetURI string) (string, error) {
	spec, err := torrent.TorrentSpecFromMagnetUri(magnetURI)
	if err != nil {
		return "", err
	}
	return spec.InfoHash.HexString(), nil
}

// MagnetFromHash creates a minimal magnet URI from an info hash
func MagnetFromHash(infoHash string) (string, error) {
	var ih metainfo.Hash
	if err := ih.FromHexString(infoHash); err != nil {
		return "", err
	}
	return fmt.Sprintf("magnet:?xt=urn:btih:%s", ih.HexString()), nil
}

// GetVideoFilePath returns the filesystem path to the main video file
func (s *Service) GetVideoFilePath(infoHash string) (string, error) {
	t, ok := s.GetTorrent(infoHash)
	if !ok {
		return "", fmt.Errorf("torrent not found: %s", infoHash)
	}

	// Find the largest video file
	var bestFile *torrent.File
	for _, f := range t.Files() {
		if isVideoFile(f.Path()) {
			if bestFile == nil || f.Length() > bestFile.Length() {
				bestFile = f
			}
		}
	}

	if bestFile == nil {
		return "", fmt.Errorf("no video file found in torrent")
	}

	// Return path relative to data dir
	return filepath.Join("/tmp", "mal-streams", bestFile.Path()), nil
}

// StartHLS starts HLS transcoding for a torrent
func (s *Service) StartHLS(ctx context.Context, infoHash string) (*HLSSession, error) {
	if s.hls == nil {
		return nil, fmt.Errorf("HLS transcoding not available (ffmpeg not found)")
	}

	// Check if session already exists
	if session, ok := s.hls.GetSession(infoHash); ok {
		return session, nil
	}

	// Get torrent and video file
	t, ok := s.GetTorrent(infoHash)
	if !ok {
		return nil, fmt.Errorf("torrent not found: %s", infoHash)
	}

	// Find the largest video file
	var videoFile *torrent.File
	for _, f := range t.Files() {
		if isVideoFile(f.Path()) {
			if videoFile == nil || f.Length() > videoFile.Length() {
				videoFile = f
			}
		}
	}

	if videoFile == nil {
		return nil, fmt.Errorf("no video file found in torrent")
	}

	// Prioritize downloading the beginning of the file for ffmpeg
	videoFile.Download()
	reader := videoFile.NewReader()
	reader.SetReadahead(10 * 1024 * 1024) // 10MB readahead
	reader.SetResponsive()

	// Wait for at least 2MB to be available before starting ffmpeg
	minBytes := int64(2 * 1024 * 1024)
	s.logger.Info("waiting for initial data", "hash", infoHash, "need", minBytes)

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	timeout := time.After(60 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-timeout:
			return nil, fmt.Errorf("timeout waiting for video data")
		case <-ticker.C:
			completed := videoFile.BytesCompleted()
			if completed >= minBytes {
				s.logger.Info("got enough data, starting transcoding", "hash", infoHash, "bytes", completed)
				goto ready
			}
			s.logger.Debug("waiting for data", "hash", infoHash, "completed", completed, "need", minBytes)
		}
	}

ready:
	// Start transcoding with the reader piped to ffmpeg
	session, err := s.hls.StartSessionWithReader(infoHash, reader)
	if err != nil {
		return nil, err
	}

	// Wait for first segments to be ready
	waitCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	if err := session.WaitReady(waitCtx); err != nil {
		s.hls.StopSession(infoHash)
		return nil, fmt.Errorf("HLS not ready: %w", err)
	}

	return session, nil
}

// GetHLSSession returns an existing HLS session
func (s *Service) GetHLSSession(infoHash string) (*HLSSession, bool) {
	if s.hls == nil {
		return nil, false
	}
	return s.hls.GetSession(infoHash)
}

// HasHLS returns whether HLS transcoding is available
func (s *Service) HasHLS() bool {
	return s.hls != nil
}
