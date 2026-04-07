package streaming

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// HLSTranscoder manages on-demand HLS transcoding sessions
type HLSTranscoder struct {
	logger   *slog.Logger
	sessions map[string]*HLSSession
	mu       sync.RWMutex
	baseDir  string
}

// HLSSession represents an active transcoding session
type HLSSession struct {
	InfoHash   string
	OutputDir  string
	Playlist   string
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	ready      chan struct{}
	err        error
	lastAccess time.Time
	mu         sync.Mutex
}

// NewHLSTranscoder creates a new HLS transcoder
func NewHLSTranscoder(logger *slog.Logger) (*HLSTranscoder, error) {
	// Check ffmpeg is available
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return nil, fmt.Errorf("ffmpeg not found in PATH: %w", err)
	}

	baseDir := filepath.Join(os.TempDir(), "mal-hls")
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create HLS temp dir: %w", err)
	}

	t := &HLSTranscoder{
		logger:   logger,
		sessions: make(map[string]*HLSSession),
		baseDir:  baseDir,
	}

	// Start cleanup goroutine
	go t.cleanupLoop()

	return t, nil
}

// StartSessionWithReader starts transcoding from a reader (piped input) to HLS
func (t *HLSTranscoder) StartSessionWithReader(infoHash string, reader io.Reader) (*HLSSession, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Return existing session if available
	if session, ok := t.sessions[infoHash]; ok {
		session.mu.Lock()
		session.lastAccess = time.Now()
		session.mu.Unlock()
		return session, nil
	}

	// Create output directory
	outputDir := filepath.Join(t.baseDir, infoHash)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output dir: %w", err)
	}

	playlist := filepath.Join(outputDir, "stream.m3u8")

	ctx, cancel := context.WithCancel(context.Background())

	session := &HLSSession{
		InfoHash:   infoHash,
		OutputDir:  outputDir,
		Playlist:   playlist,
		cancel:     cancel,
		ready:      make(chan struct{}),
		lastAccess: time.Now(),
	}

	// Start ffmpeg with pipe input
	// Use browser-compatible H.264 baseline/main profile and AAC-LC
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", "pipe:0", // Read from stdin
		// Video: H.264 with browser-compatible settings
		"-c:v", "libx264",
		"-preset", "veryfast",
		"-crf", "23",
		"-profile:v", "main",
		"-level", "4.0",
		"-pix_fmt", "yuv420p",
		// Audio: AAC-LC stereo
		"-c:a", "aac",
		"-b:a", "128k",
		"-ac", "2",
		"-ar", "44100",
		// HLS output
		"-f", "hls",
		"-hls_time", "4",
		"-hls_list_size", "0",
		"-hls_flags", "append_list+independent_segments",
		"-hls_segment_type", "mpegts",
		"-hls_segment_filename", filepath.Join(outputDir, "segment_%03d.ts"),
		"-start_number", "0",
		playlist,
	)

	// Pipe the reader to ffmpeg's stdin
	cmd.Stdin = reader
	session.cmd = cmd

	// Capture stderr for debugging
	cmd.Stderr = &ffmpegLogger{logger: t.logger, infoHash: infoHash}

	if err := cmd.Start(); err != nil {
		cancel()
		os.RemoveAll(outputDir)
		return nil, fmt.Errorf("failed to start ffmpeg: %w", err)
	}

	t.sessions[infoHash] = session

	// Wait for playlist to be created
	go func() {
		defer close(session.ready)

		for i := 0; i < 120; i++ { // Wait up to 60 seconds
			if _, err := os.Stat(playlist); err == nil {
				// Check if at least one segment exists
				segments, _ := filepath.Glob(filepath.Join(outputDir, "segment_*.ts"))
				if len(segments) > 0 {
					t.logger.Info("HLS segments ready", "hash", infoHash, "segments", len(segments))
					return
				}
			}
			time.Sleep(500 * time.Millisecond)
		}

		session.err = fmt.Errorf("timeout waiting for HLS segments")
		cancel()
	}()

	// Monitor process
	go func() {
		err := cmd.Wait()
		if err != nil && ctx.Err() == nil {
			t.logger.Error("ffmpeg exited with error", "hash", infoHash, "error", err)
		} else {
			t.logger.Info("ffmpeg finished", "hash", infoHash)
		}
	}()

	return session, nil
}

// GetSession returns an existing session
func (t *HLSTranscoder) GetSession(infoHash string) (*HLSSession, bool) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	session, ok := t.sessions[infoHash]
	if ok {
		session.mu.Lock()
		session.lastAccess = time.Now()
		session.mu.Unlock()
	}
	return session, ok
}

// StopSession stops a transcoding session
func (t *HLSTranscoder) StopSession(infoHash string) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if session, ok := t.sessions[infoHash]; ok {
		session.cancel()
		os.RemoveAll(session.OutputDir)
		delete(t.sessions, infoHash)
	}
}

// WaitReady waits for the session to have segments ready
func (s *HLSSession) WaitReady(ctx context.Context) error {
	select {
	case <-s.ready:
		return s.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// cleanupLoop removes stale sessions
func (t *HLSTranscoder) cleanupLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		t.mu.Lock()
		now := time.Now()
		for hash, session := range t.sessions {
			session.mu.Lock()
			if now.Sub(session.lastAccess) > 10*time.Minute {
				session.cancel()
				os.RemoveAll(session.OutputDir)
				delete(t.sessions, hash)
				t.logger.Info("cleaned up stale HLS session", "hash", hash)
			}
			session.mu.Unlock()
		}
		t.mu.Unlock()
	}
}

// Shutdown stops all sessions
func (t *HLSTranscoder) Shutdown() {
	t.mu.Lock()
	defer t.mu.Unlock()

	for hash, session := range t.sessions {
		session.cancel()
		os.RemoveAll(session.OutputDir)
		delete(t.sessions, hash)
	}

	os.RemoveAll(t.baseDir)
}

// ffmpegLogger logs ffmpeg stderr output
type ffmpegLogger struct {
	logger   *slog.Logger
	infoHash string
}

func (l *ffmpegLogger) Write(p []byte) (n int, err error) {
	l.logger.Debug("ffmpeg", "hash", l.infoHash, "output", string(p))
	return len(p), nil
}
