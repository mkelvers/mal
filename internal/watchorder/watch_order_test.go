package watchorder

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func testServer(body string) *httptest.Server {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(body))
	})

	return httptest.NewServer(handler)
}

func testHTMLWithMetadata() string {
	return `
<!doctype html>
<html>
  <body>
    <div id="wo_type_filter">
      <label><input type="checkbox" value="1" checked> TV</label>
      <label><input type="checkbox" value="3" checked> Movie</label>
    </div>
    <table id="wo_list">
      <tr data-id="442" data-anilist-id="442" data-type="3">
        <td>
          <span class="wo_title">Naruto Movie 1</span>
          <span class="uk-text-small">Naruto the Movie 1</span>
        </td>
      </tr>
    </table>
  </body>
</html>`
}

func testHTMLEmptyRows() string {
	return `
<!doctype html>
<html>
  <body>
    <div id="wo_type_filter">
      <label><input type="checkbox" value="1" checked> TV</label>
      <label><input type="checkbox" value="3" checked> Movie</label>
    </div>
    <table id="wo_list"></table>
  </body>
</html>`
}

func TestFetchWatchOrder_OutputShape(t *testing.T) {
	server := testServer(testHTMLWithMetadata())
	defer server.Close()

	url := server.URL + "/?/tools/watch_order/id/442"
	result, err := FetchWatchOrder(context.Background(), &http.Client{Timeout: time.Second}, url)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.ID != 442 {
		t.Fatalf("expected root id 442, got %d", result.ID)
	}

	if len(result.WatchOrder) != 1 {
		t.Fatalf("expected 1 watch_order entry, got %d", len(result.WatchOrder))
	}

	entry := result.WatchOrder[0]
	if entry.ID != 442 {
		t.Fatalf("expected entry id 442, got %d", entry.ID)
	}
	if entry.Type != "Movie" {
		t.Fatalf("expected type Movie, got %q", entry.Type)
	}
	if entry.Title != "Naruto Movie 1" {
		t.Fatalf("expected title Naruto Movie 1, got %q", entry.Title)
	}
	if entry.TitleAlt != "Naruto the Movie 1" {
		t.Fatalf("expected title_alt Naruto the Movie 1, got %q", entry.TitleAlt)
	}
}

func TestFetchWatchOrder_NoRowsReturnsEmpty(t *testing.T) {
	server := testServer(testHTMLEmptyRows())
	defer server.Close()

	url := server.URL + "/?/tools/watch_order/id/1535"
	result, err := FetchWatchOrder(context.Background(), &http.Client{Timeout: time.Second}, url)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if result.ID != 1535 {
		t.Fatalf("expected root id 1535, got %d", result.ID)
	}

	if len(result.WatchOrder) != 0 {
		t.Fatalf("expected no entries, got %d", len(result.WatchOrder))
	}
}

func TestFetchWatchOrder_MissingMarkupFallsBackToProxy(t *testing.T) {
	proxyPayload := `Title: Jujutsu Kaisen / Watch Order
URL Source: https://chiaki.site/?/tools/watch_order/id/40748

Markdown Content:
Jujutsu Kaisen

 Oct 3, 2020 – Mar 27, 2021 | TV | 24ep × 23min. | ★8.51 | [](https://myanimelist.net/anime/40748)
Jujutsu Kaisen 0 Movie

Jujutsu Kaisen 0

 Dec 24, 2021 | Movie | 1ep × 1hr. 44min. | ★8.36 | [](https://myanimelist.net/anime/48561)
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/http/") {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(proxyPayload))
			return
		}

		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("blocked"))
	}))
	defer server.Close()

	transport := http.DefaultTransport
	testClient := &http.Client{
		Timeout: time.Second,
		Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			if strings.HasPrefix(request.URL.Host, "r.jina.ai") {
				proxyURL := server.URL + "/http/" + strings.TrimPrefix(request.URL.Path, "/")
				proxyRequest, err := http.NewRequestWithContext(request.Context(), request.Method, proxyURL, nil)
				if err != nil {
					return nil, err
				}
				return transport.RoundTrip(proxyRequest)
			}

			blockedURL := server.URL + request.URL.Path
			blockedRequest, err := http.NewRequestWithContext(request.Context(), request.Method, blockedURL, nil)
			if err != nil {
				return nil, err
			}
			return transport.RoundTrip(blockedRequest)
		}),
	}

	result, err := FetchWatchOrder(context.Background(), testClient, "https://chiaki.site/?/tools/watch_order/id/40748")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(result.WatchOrder) != 2 {
		t.Fatalf("expected 2 proxy entries, got %d", len(result.WatchOrder))
	}

	if result.WatchOrder[0].ID != 40748 || result.WatchOrder[0].Type != "TV" {
		t.Fatalf("unexpected first entry: %+v", result.WatchOrder[0])
	}

	if result.WatchOrder[1].ID != 48561 || result.WatchOrder[1].Type != "Movie" {
		t.Fatalf("unexpected second entry: %+v", result.WatchOrder[1])
	}
}

func TestFetchWatchOrder_HTTPStatusErrorIncludesContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "cloudflare")
		w.Header().Set("CF-Ray", "abc123")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("<html><body>access denied</body></html>"))
	}))
	defer server.Close()

	url := server.URL + "/?/tools/watch_order/id/1"
	_, err := fetchDocument(context.Background(), &http.Client{Timeout: time.Second}, url)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}

	var statusError *HTTPStatusError
	if !errors.As(err, &statusError) {
		t.Fatalf("expected HTTPStatusError, got %T", err)
	}

	if statusError.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", statusError.StatusCode)
	}
	if statusError.CFRay != "abc123" {
		t.Fatalf("expected cf-ray abc123, got %q", statusError.CFRay)
	}
	if !strings.Contains(statusError.BodyPreview, "access denied") {
		t.Fatalf("expected body preview to include access denied, got %q", statusError.BodyPreview)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
