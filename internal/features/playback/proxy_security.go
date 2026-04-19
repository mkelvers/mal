package playback

import (
	"bufio"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"
)

const (
	proxyStreamTokenTTL   = 2 * time.Hour
	proxySegmentTokenTTL  = 6 * time.Hour
	proxySubtitleTokenTTL = 6 * time.Hour
)
const proxyHostCheckTTL = 2 * time.Minute

type proxyScope string

const (
	proxyScopeStream   proxyScope = "stream"
	proxyScopeSegment  proxyScope = "segment"
	proxyScopeSubtitle proxyScope = "subtitle"
)

type proxyTokenPayload struct {
	TargetURL string `json:"u"`
	Referer   string `json:"r,omitempty"`
	Scope     string `json:"s"`
	ExpiresAt int64  `json:"exp"`
}

type proxyTokenSigner struct {
	secret []byte
}

func newProxyTokenSigner(secret string) (*proxyTokenSigner, error) {
	trimmed := strings.TrimSpace(secret)
	if trimmed == "" {
		return nil, errors.New("proxy token secret is required")
	}

	if len(trimmed) < 32 {
		return nil, errors.New("proxy token secret must be at least 32 characters")
	}

	return &proxyTokenSigner{secret: []byte(trimmed)}, nil
}

func (s *proxyTokenSigner) Sign(payload proxyTokenPayload) (string, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal proxy token payload: %w", err)
	}

	mac := hmac.New(sha256.New, s.secret)
	mac.Write(body)
	signature := mac.Sum(nil)

	encodedBody := base64.RawURLEncoding.EncodeToString(body)
	encodedSignature := base64.RawURLEncoding.EncodeToString(signature)
	return encodedBody + "." + encodedSignature, nil
}

func (s *proxyTokenSigner) Verify(token string) (proxyTokenPayload, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return proxyTokenPayload{}, errors.New("invalid proxy token format")
	}

	body, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return proxyTokenPayload{}, errors.New("invalid proxy token payload")
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return proxyTokenPayload{}, errors.New("invalid proxy token signature")
	}

	mac := hmac.New(sha256.New, s.secret)
	mac.Write(body)
	expected := mac.Sum(nil)
	if !hmac.Equal(signature, expected) {
		return proxyTokenPayload{}, errors.New("invalid proxy token signature")
	}

	var payload proxyTokenPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return proxyTokenPayload{}, errors.New("invalid proxy token payload")
	}

	if payload.ExpiresAt <= time.Now().Unix() {
		return proxyTokenPayload{}, errors.New("proxy token expired")
	}

	return payload, nil
}

func (s *Service) buildClientModeSources(modeSources map[string]ModeSource) (map[string]ModeSource, error) {
	clientModeSources := make(map[string]ModeSource, len(modeSources))

	for mode, source := range modeSources {
		streamToken, err := s.issueProxyToken(source.URL, source.Referer, proxyScopeStream)
		if err != nil {
			return nil, err
		}

		subtitles := make([]SubtitleItem, 0, len(source.Subtitles))
		for _, subtitle := range source.Subtitles {
			targetURL := strings.TrimSpace(subtitle.URL)
			if targetURL == "" {
				continue
			}

			token, err := s.issueProxyToken(targetURL, source.Referer, proxyScopeSubtitle)
			if err != nil {
				return nil, err
			}

			subtitles = append(subtitles, SubtitleItem{
				Lang:  subtitle.Lang,
				Token: token,
			})
		}

		clientModeSources[mode] = ModeSource{
			Token:     streamToken,
			Subtitles: subtitles,
		}
	}

	return clientModeSources, nil
}

func (s *Service) issueProxyToken(targetURL string, referer string, scope proxyScope) (string, error) {
	normalizedTarget, err := normalizeProxyURL(targetURL)
	if err != nil {
		return "", err
	}

	normalizedReferer := ""
	if strings.TrimSpace(referer) != "" {
		refererURL, refererErr := normalizeProxyURL(referer)
		if refererErr == nil {
			normalizedReferer = refererURL
		}
	}

	return s.proxyTokens.Sign(proxyTokenPayload{
		TargetURL: normalizedTarget,
		Referer:   normalizedReferer,
		Scope:     string(scope),
		ExpiresAt: time.Now().Add(proxyTokenTTL(scope)).Unix(),
	})
}

var proxyTokenTTLs = map[proxyScope]time.Duration{
	proxyScopeStream:   proxyStreamTokenTTL,
	proxyScopeSegment:  proxySegmentTokenTTL,
	proxyScopeSubtitle: proxySubtitleTokenTTL,
}

func proxyTokenTTL(scope proxyScope) time.Duration {
	if ttl, ok := proxyTokenTTLs[scope]; ok {
		return ttl
	}
	return proxyStreamTokenTTL
}

func (s *Service) resolveProxyToken(ctx context.Context, token string, scope proxyScope) (string, string, error) {
	payload, err := s.proxyTokens.Verify(token)
	if err != nil {
		return "", "", err
	}

	if payload.Scope != string(scope) {
		return "", "", errors.New("proxy token scope mismatch")
	}

	normalizedTarget, err := normalizeProxyURL(payload.TargetURL)
	if err != nil {
		return "", "", err
	}

	if err := s.ensurePublicProxyTarget(ctx, normalizedTarget); err != nil {
		return "", "", err
	}

	normalizedReferer := ""
	if strings.TrimSpace(payload.Referer) != "" {
		refererURL, refererErr := normalizeProxyURL(payload.Referer)
		if refererErr == nil {
			if ensureErr := s.ensurePublicProxyTarget(ctx, refererURL); ensureErr == nil {
				normalizedReferer = refererURL
			}
		}
	}

	return normalizedTarget, normalizedReferer, nil
}

func normalizeProxyURL(rawURL string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return "", errors.New("invalid proxy target")
	}

	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("invalid proxy target scheme")
	}

	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" {
		return "", errors.New("invalid proxy target host")
	}

	if host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return "", errors.New("localhost targets are not allowed")
	}

	ip := net.ParseIP(host)
	if ip != nil && isBlockedProxyIP(ip) {
		return "", errors.New("private proxy targets are not allowed")
	}

	return parsed.String(), nil
}

func isBlockedProxyIP(ip net.IP) bool {
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsMulticast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsUnspecified()
}

func (s *Service) ensurePublicProxyTarget(ctx context.Context, rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return errors.New("invalid proxy target")
	}

	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return errors.New("invalid proxy target host")
	}

	if ip := net.ParseIP(host); ip != nil {
		if isBlockedProxyIP(ip) {
			return errors.New("private proxy targets are not allowed")
		}
		return nil
	}

	now := time.Now()
	s.proxyHostMu.RLock()
	cached, ok := s.proxyHostCache[host]
	s.proxyHostMu.RUnlock()
	if ok && now.Before(cached.ExpiresAt) {
		if cached.Allowed {
			return nil
		}
		return errors.New("private proxy targets are not allowed")
	}

	resolvedIPs, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil || len(resolvedIPs) == 0 {
		return errors.New("proxy target lookup failed")
	}

	allowed := true
	for _, resolved := range resolvedIPs {
		if isBlockedProxyIP(resolved.IP) {
			allowed = false
			break
		}
	}

	s.proxyHostMu.Lock()
	s.proxyHostCache[host] = proxyHostCacheItem{
		Allowed:   allowed,
		ExpiresAt: now.Add(proxyHostCheckTTL),
	}
	s.proxyHostMu.Unlock()

	if !allowed {
		return errors.New("private proxy targets are not allowed")
	}

	return nil
}

func (s *Service) rewritePlaylistWithTokens(ctx context.Context, content string, baseURL string, referer string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}

	var out strings.Builder
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}

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

		absoluteURL := base.ResolveReference(relativeURL).String()
		token, tokenErr := s.issueProxyToken(absoluteURL, referer, proxyScopeSegment)
		if tokenErr != nil {
			return "", tokenErr
		}

		proxied := "/watch/proxy/segment?token=" + url.QueryEscape(token)
		out.WriteString(proxied)
		out.WriteString("\n")
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return out.String(), nil
}
