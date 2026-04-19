package playback

import (
	"bytes"
	"errors"
	"sort"
	"strconv"
	"strings"
)

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
