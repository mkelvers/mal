package playback

import (
	"context"
	"testing"

	"mal/internal/database"
)

func TestNormalizeProxyURLRejectsLocalhost(t *testing.T) {
	t.Parallel()

	_, err := normalizeProxyURL("http://localhost:8080/private")
	if err == nil {
		t.Fatal("expected localhost URL to be rejected")
	}
}

func TestNormalizeProxyURLRejectsPrivateIP(t *testing.T) {
	t.Parallel()

	_, err := normalizeProxyURL("http://192.168.1.10/stream")
	if err == nil {
		t.Fatal("expected private IP URL to be rejected")
	}
}

func TestProxyTokenScopeValidation(t *testing.T) {
	t.Parallel()

	service := NewService(&fakeProxyQuerier{}, nil, Config{ProxyTokenSecret: "0123456789abcdef0123456789abcdef"})
	token, err := service.issueProxyToken("https://example.com/playlist.m3u8", "", proxyScopeStream)
	if err != nil {
		t.Fatalf("failed to issue token: %v", err)
	}

	_, _, err = service.resolveProxyToken(context.Background(), token, proxyScopeSegment)
	if err == nil {
		t.Fatal("expected scope mismatch error")
	}
}

type fakeProxyQuerier struct {
	database.Querier
}
