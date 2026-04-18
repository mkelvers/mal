package playback

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	previewFrameWidth      = 160
	previewFrameHeight     = 90
	previewGridColumns     = 10
	previewGridRows        = 10
	previewFrameInterval   = 10
	previewFrameLimit      = previewGridColumns * previewGridRows
	previewGenerationLimit = 120 * time.Second
	previewFailureTTL      = 2 * time.Minute
	previewManifestName    = "map.json"
	spriteFileName         = "sprite.jpg"
)

type ffprobeFormat struct {
	Duration string `json:"duration"`
}

type ffprobeResult struct {
	Format ffprobeFormat `json:"format"`
}

func newPreviewRootDir() string {
	return filepath.Join(os.TempDir(), "mal-preview-cache")
}

func (s *Service) EnsurePreviewMap(ctx context.Context, req PreviewRequest) (PreviewMap, string, error) {
	if strings.TrimSpace(req.Source) == "" {
		return PreviewMap{}, "", fmt.Errorf("missing preview source")
	}

	parsedSource, err := url.Parse(req.Source)
	if err != nil {
		return PreviewMap{}, "", fmt.Errorf("invalid preview source")
	}
	if parsedSource.Scheme != "http" && parsedSource.Scheme != "https" {
		return PreviewMap{}, "", fmt.Errorf("invalid preview source scheme")
	}

	normalizedEpisode := strings.TrimSpace(req.Episode)
	if normalizedEpisode == "" {
		normalizedEpisode = "1"
	}

	normalizedMode := normalizeMode(req.Mode)
	if normalizedMode == "" {
		normalizedMode = "dub"
	}

	previewHash := hashPreviewIdentity(req.MalID, normalizedEpisode, normalizedMode, req.Source, req.Referer)
	previewKey := fmt.Sprintf("%d-%s", req.MalID, previewHash)
	if s.previewFailureActive(previewKey) {
		return PreviewMap{}, "", fmt.Errorf("preview temporarily disabled")
	}
	previewDir := filepath.Join(s.previewRoot, previewKey)
	manifestPath := filepath.Join(previewDir, previewManifestName)
	spritePath := filepath.Join(previewDir, spriteFileName)

	if mapData, err := readPreviewManifest(manifestPath); err == nil {
		if mapData.Duration > 0 && fileExists(spritePath) {
			return mapData, previewKey, nil
		}
	}

	lock := s.previewLock(previewKey)
	lock.Lock()
	defer lock.Unlock()

	if mapData, err := readPreviewManifest(manifestPath); err == nil {
		if mapData.Duration > 0 && fileExists(spritePath) {
			return mapData, previewKey, nil
		}
	}

	if err := os.MkdirAll(previewDir, 0o755); err != nil {
		return PreviewMap{}, "", fmt.Errorf("create preview cache dir: %w", err)
	}

	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return PreviewMap{}, "", fmt.Errorf("ffmpeg not found in PATH")
	}

	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		return PreviewMap{}, "", fmt.Errorf("ffprobe not found in PATH")
	}

	ctxWithTimeout, cancel := context.WithTimeout(ctx, previewGenerationLimit)
	defer cancel()

	duration, err := probePreviewDuration(ctxWithTimeout, ffprobePath, req.Source, req.Referer)
	if err != nil {
		if req.Duration > 0 {
			duration = req.Duration
		} else {
			return PreviewMap{}, "", fmt.Errorf("probe preview duration: %w", err)
		}
	}

	if duration <= 0 {
		return PreviewMap{}, "", fmt.Errorf("invalid duration for preview generation")
	}

	interval := selectPreviewInterval(duration)

	if err := generatePreviewSprite(ctxWithTimeout, ffmpegPath, req.Source, req.Referer, spritePath, interval); err != nil {
		if shouldBackoffPreview(err) {
			s.markPreviewFailure(previewKey)
		}
		return PreviewMap{}, "", err
	}
	s.clearPreviewFailure(previewKey)

	mapData := buildPreviewMap(duration, interval)
	if err := writePreviewManifest(manifestPath, mapData); err != nil {
		return PreviewMap{}, "", fmt.Errorf("write preview manifest: %w", err)
	}

	return mapData, previewKey, nil
}

func (s *Service) PreviewSpritePath(previewKey string) string {
	trimmed := strings.TrimSpace(previewKey)
	if trimmed == "" {
		return ""
	}

	safeKey := sanitizePreviewKey(trimmed)
	if safeKey == "" {
		return ""
	}

	return filepath.Join(s.previewRoot, safeKey, spriteFileName)
}

func (s *Service) previewLock(key string) *sync.Mutex {
	s.previewMu.Lock()
	defer s.previewMu.Unlock()

	lock, exists := s.previewLocks[key]
	if exists {
		return lock
	}

	newLock := &sync.Mutex{}
	s.previewLocks[key] = newLock
	return newLock
}

func (s *Service) previewFailureActive(key string) bool {
	s.previewFailMu.Lock()
	defer s.previewFailMu.Unlock()

	expiresAt, ok := s.previewFailTTL[key]
	if !ok {
		return false
	}

	if time.Now().After(expiresAt) {
		delete(s.previewFailTTL, key)
		return false
	}

	return true
}

func (s *Service) markPreviewFailure(key string) {
	s.previewFailMu.Lock()
	defer s.previewFailMu.Unlock()

	s.previewFailTTL[key] = time.Now().Add(previewFailureTTL)
}

func (s *Service) clearPreviewFailure(key string) {
	s.previewFailMu.Lock()
	defer s.previewFailMu.Unlock()

	delete(s.previewFailTTL, key)
}

func hashPreviewIdentity(malID int, episode string, mode string, source string, referer string) string {
	payload := fmt.Sprintf("%d|%s|%s|%s|%s", malID, episode, mode, source, referer)
	sum := sha1.Sum([]byte(payload))
	return hex.EncodeToString(sum[:8])
}

func sanitizePreviewKey(raw string) string {
	var builder strings.Builder
	for _, char := range raw {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' {
			builder.WriteRune(char)
		}
	}

	return builder.String()
}

func selectPreviewInterval(duration float64) float64 {
	if duration <= 0 {
		return previewFrameInterval
	}

	candidate := duration / float64(previewFrameLimit)
	if candidate < previewFrameInterval {
		return previewFrameInterval
	}

	return math.Ceil(candidate)
}

func computePreviewFrames(duration float64, interval float64) int {
	if duration <= 0 || interval <= 0 {
		return 1
	}

	frames := int(math.Ceil(duration / interval))
	if frames > previewFrameLimit {
		return previewFrameLimit
	}

	return frames
}

func buildPreviewMap(duration float64, interval float64) PreviewMap {
	frames := computePreviewFrames(duration, interval)
	cues := make([]PreviewCue, 0, frames)

	for idx := 0; idx < frames; idx++ {
		start := float64(idx) * interval
		end := start + interval
		if idx == frames-1 {
			end = duration
		}
		if end > duration {
			end = duration
		}

		column := idx % previewGridColumns
		row := idx / previewGridColumns
		x := column * previewFrameWidth
		y := row * previewFrameHeight

		cues = append(cues, PreviewCue{
			Start:  start,
			End:    end,
			Sprite: spriteFileName,
			X:      x,
			Y:      y,
			Width:  previewFrameWidth,
			Height: previewFrameHeight,
		})
	}

	return PreviewMap{
		Width:    previewFrameWidth,
		Height:   previewFrameHeight,
		Columns:  previewGridColumns,
		Rows:     previewGridRows,
		Interval: interval,
		Duration: duration,
		Cues:     cues,
	}
}

func readPreviewManifest(path string) (PreviewMap, error) {
	payload, err := os.ReadFile(path)
	if err != nil {
		return PreviewMap{}, err
	}

	var mapData PreviewMap
	if err := json.Unmarshal(payload, &mapData); err != nil {
		return PreviewMap{}, err
	}

	if mapData.Duration <= 0 || len(mapData.Cues) == 0 {
		return PreviewMap{}, fmt.Errorf("invalid preview manifest")
	}

	return mapData, nil
}

func writePreviewManifest(path string, mapData PreviewMap) error {
	payload, err := json.Marshal(mapData)
	if err != nil {
		return err
	}

	return os.WriteFile(path, payload, 0o644)
}

func probePreviewDuration(ctx context.Context, ffprobePath string, source string, referer string) (float64, error) {
	args := []string{
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "json",
	}

	headers := ffmpegHeaders(referer)
	if headers != "" {
		args = append(args, "-headers", headers)
	}

	args = append(args, source)

	cmd := exec.CommandContext(ctx, ffprobePath, args...)
	output, err := cmd.Output()
	if err != nil {
		return 0, err
	}

	var parsed ffprobeResult
	if err := json.Unmarshal(output, &parsed); err != nil {
		return 0, err
	}

	durationText := strings.TrimSpace(parsed.Format.Duration)
	if durationText == "" {
		return 0, fmt.Errorf("missing duration")
	}

	duration, err := strconv.ParseFloat(durationText, 64)
	if err != nil {
		return 0, err
	}

	if !isFinitePositive(duration) {
		return 0, fmt.Errorf("non-finite duration")
	}

	return duration, nil
}

func generatePreviewSprite(ctx context.Context, ffmpegPath string, source string, referer string, outputPath string, interval float64) error {
	filter := fmt.Sprintf("fps=1/%0.3f,scale=%d:%d,tile=%dx%d", interval, previewFrameWidth, previewFrameHeight, previewGridColumns, previewGridRows)
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-y",
	}

	headers := ffmpegHeaders(referer)
	if headers != "" {
		args = append(args, "-headers", headers)
	}

	args = append(args,
		"-i", source,
		"-vf", filter,
		"-frames:v", "1",
		"-q:v", "5",
		"-update", "1",
		outputPath,
	)

	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("create ffmpeg stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}

	errPayload, _ := io.ReadAll(io.LimitReader(stderr, 64*1024))
	waitErr := cmd.Wait()
	if waitErr != nil {
		message := strings.TrimSpace(string(errPayload))
		if message == "" {
			return fmt.Errorf("ffmpeg failed: %w", waitErr)
		}
		return fmt.Errorf("ffmpeg failed: %s", message)
	}

	if _, statErr := os.Stat(outputPath); statErr != nil {
		return fmt.Errorf("preview sprite not generated")
	}

	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func ffmpegHeaders(referer string) string {
	var builder strings.Builder
	builder.WriteString("User-Agent: ")
	builder.WriteString(defaultUserAgent)
	builder.WriteString("\r\n")

	trimmedReferer := strings.TrimSpace(referer)
	if trimmedReferer != "" {
		builder.WriteString("Referer: ")
		builder.WriteString(trimmedReferer)
		builder.WriteString("\r\n")
	}

	builder.WriteString("Connection: keep-alive\r\n")
	return builder.String()
}

func isFinitePositive(value float64) bool {
	if value <= 0 {
		return false
	}

	if math.IsInf(value, 0) {
		return false
	}

	if math.IsNaN(value) {
		return false
	}

	return true
}

func shouldBackoffPreview(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}

	errText := strings.ToLower(err.Error())
	if strings.Contains(errText, "signal: killed") {
		return true
	}
	if strings.Contains(errText, "context canceled") {
		return true
	}
	if strings.Contains(errText, "cannot allocate memory") {
		return true
	}

	return false
}
