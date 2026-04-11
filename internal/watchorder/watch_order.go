package watchorder

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36"

var idPattern = regexp.MustCompile(`/id/(\d+)`)
var malLinkPattern = regexp.MustCompile(`myanimelist\.net/anime/(\d+)`)

var ErrInvalidWatchOrderURL = errors.New("invalid watch order url")
var ErrWatchOrderMarkupNotFound = errors.New("watch order markup not found")

type HTTPStatusError struct {
	StatusCode  int
	URL         string
	Server      string
	CFRay       string
	Location    string
	ContentType string
	BodyPreview string
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf(
		"unexpected status code: %d (url=%s server=%s cf_ray=%s location=%s content_type=%s body=%q)",
		e.StatusCode,
		e.URL,
		e.Server,
		e.CFRay,
		e.Location,
		e.ContentType,
		e.BodyPreview,
	)
}

type WatchOrderEntry struct {
	ID       int    `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	TitleAlt string `json:"title_alt,omitempty"`
}

type WatchOrderResult struct {
	ID         int               `json:"id"`
	WatchOrder []WatchOrderEntry `json:"watch_order"`
}

type watchOrderRow struct {
	id               int
	typeID           int
	title            string
	alternativeTitle string
}

func parseRootID(url string) (int, error) {
	match := idPattern.FindStringSubmatch(url)
	if len(match) != 2 {
		return 0, ErrInvalidWatchOrderURL
	}

	id, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, ErrInvalidWatchOrderURL
	}

	return id, nil
}

func addCommonHeaders(request *http.Request) {
	request.Header.Set("User-Agent", defaultUserAgent)
	request.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	request.Header.Set("Accept-Language", "en-US,en;q=0.9")
	request.Header.Set("Referer", "https://chiaki.site/")
	request.Header.Set("Cache-Control", "no-cache")
}

func fetchDocument(ctx context.Context, httpClient *http.Client, url string) (*goquery.Document, error) {
	client := httpClient
	if client == nil {
		client = http.DefaultClient
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	addCommonHeaders(request)

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return nil, &HTTPStatusError{
			StatusCode:  response.StatusCode,
			URL:         url,
			Server:      strings.TrimSpace(response.Header.Get("Server")),
			CFRay:       strings.TrimSpace(response.Header.Get("CF-Ray")),
			Location:    strings.TrimSpace(response.Header.Get("Location")),
			ContentType: strings.TrimSpace(response.Header.Get("Content-Type")),
			BodyPreview: strings.Join(strings.Fields(strings.TrimSpace(string(body))), " "),
		}
	}

	document, err := goquery.NewDocumentFromReader(response.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse html: %w", err)
	}

	return document, nil
}

func extractTypeLabelsByID(doc *goquery.Document) map[int]string {
	typeLabels := make(map[int]string)

	doc.Find("#wo_type_filter label").Each(func(_ int, selection *goquery.Selection) {
		input := selection.Find("input[type='checkbox']")
		rawID, exists := input.Attr("value")
		if !exists {
			return
		}

		typeID, err := strconv.Atoi(strings.TrimSpace(rawID))
		if err != nil {
			return
		}

		label := strings.TrimSpace(selection.Text())
		if label == "" {
			return
		}

		typeLabels[typeID] = label
	})

	return typeLabels
}

func parseAttrInt(selection *goquery.Selection, attrName string) (int, bool) {
	rawValue, exists := selection.Attr(attrName)
	if !exists {
		return 0, false
	}

	value, err := strconv.Atoi(strings.TrimSpace(rawValue))
	if err != nil {
		return 0, false
	}

	return value, true
}

func extractRows(doc *goquery.Document) []watchOrderRow {
	rows := make([]watchOrderRow, 0)

	doc.Find("tr[data-id]").Each(func(_ int, selection *goquery.Selection) {
		id, ok := parseAttrInt(selection, "data-id")
		if !ok {
			return
		}

		typeID, ok := parseAttrInt(selection, "data-type")
		if !ok {
			return
		}

		title := strings.TrimSpace(selection.Find(".wo_title").First().Text())
		alternativeTitle := strings.TrimSpace(selection.Find(".uk-text-small").First().Text())

		rows = append(rows, watchOrderRow{
			id:               id,
			typeID:           typeID,
			title:            title,
			alternativeTitle: alternativeTitle,
		})
	})

	return rows
}

func hasWatchOrderTable(doc *goquery.Document) bool {
	return doc.Find("#wo_list").Length() > 0
}

func shouldTryProxy(err error) bool {
	var statusError *HTTPStatusError
	if errors.As(err, &statusError) {
		return statusError.StatusCode == http.StatusForbidden || statusError.StatusCode == http.StatusTooManyRequests || statusError.StatusCode == http.StatusServiceUnavailable
	}

	return false
}

func toJinaProxyURL(url string) string {
	trimmed := strings.TrimPrefix(strings.TrimPrefix(url, "https://"), "http://")
	return "https://r.jina.ai/http://" + trimmed
}

func fetchProxyText(ctx context.Context, httpClient *http.Client, url string) (string, error) {
	client := httpClient
	if client == nil {
		client = http.DefaultClient
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, toJinaProxyURL(url), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create proxy request: %w", err)
	}

	addCommonHeaders(request)

	response, err := client.Do(request)
	if err != nil {
		return "", fmt.Errorf("proxy request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("proxy status %d", response.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
	if err != nil {
		return "", fmt.Errorf("failed to read proxy response: %w", err)
	}

	return string(body), nil
}

func parseJinaEntries(text string) []WatchOrderEntry {
	lines := strings.Split(text, "\n")
	entries := make([]WatchOrderEntry, 0)
	seen := make(map[int]bool)

	for index, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}

		if !strings.Contains(trimmed, "myanimelist.net/anime/") || !strings.Contains(trimmed, "|") {
			continue
		}

		idMatch := malLinkPattern.FindStringSubmatch(trimmed)
		if len(idMatch) != 2 {
			continue
		}

		id, err := strconv.Atoi(idMatch[1])
		if err != nil || seen[id] {
			continue
		}

		parts := strings.Split(trimmed, "|")
		if len(parts) < 2 {
			continue
		}

		typeName := strings.TrimSpace(parts[1])
		if typeName == "" {
			continue
		}

		title, titleAlt := titleFromContext(lines, index)
		entries = append(entries, WatchOrderEntry{
			ID:       id,
			Type:     typeName,
			Title:    title,
			TitleAlt: titleAlt,
		})
		seen[id] = true
	}

	return entries
}

func isNoiseTitleLine(value string) bool {
	lower := strings.ToLower(strings.TrimSpace(value))
	if lower == "" {
		return true
	}

	if strings.HasPrefix(lower, "title:") || strings.HasPrefix(lower, "url source:") || strings.HasPrefix(lower, "markdown content:") {
		return true
	}

	if strings.Contains(lower, "/ watch order") {
		return true
	}

	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return true
	}

	return false
}

func titleFromContext(lines []string, metaIndex int) (string, string) {
	collected := make([]string, 0, 2)

	for idx := metaIndex - 1; idx >= 0 && len(collected) < 2; idx-- {
		candidate := strings.TrimSpace(lines[idx])
		if candidate == "" {
			continue
		}

		if isNoiseTitleLine(candidate) {
			continue
		}

		if strings.Contains(candidate, "myanimelist.net/anime/") {
			continue
		}

		collected = append(collected, candidate)
	}

	if len(collected) == 0 {
		return "", ""
	}

	if len(collected) == 1 {
		return collected[0], ""
	}

	return collected[1], collected[0]
}

func fetchViaProxy(ctx context.Context, httpClient *http.Client, url string, rootID int) (WatchOrderResult, error) {
	proxyText, err := fetchProxyText(ctx, httpClient, url)
	if err != nil {
		return WatchOrderResult{}, err
	}

	entries := parseJinaEntries(proxyText)
	if len(entries) == 0 {
		return WatchOrderResult{}, ErrWatchOrderMarkupNotFound
	}

	return WatchOrderResult{ID: rootID, WatchOrder: entries}, nil
}

func FetchWatchOrder(ctx context.Context, httpClient *http.Client, url string) (WatchOrderResult, error) {
	rootID, err := parseRootID(url)
	if err != nil {
		return WatchOrderResult{}, err
	}

	doc, err := fetchDocument(ctx, httpClient, url)
	if err != nil {
		if shouldTryProxy(err) {
			return fetchViaProxy(ctx, httpClient, url, rootID)
		}
		return WatchOrderResult{}, err
	}

	if !hasWatchOrderTable(doc) {
		return fetchViaProxy(ctx, httpClient, url, rootID)
	}

	rows := extractRows(doc)
	if len(rows) == 0 {
		return WatchOrderResult{ID: rootID, WatchOrder: []WatchOrderEntry{}}, nil
	}

	typeByID := extractTypeLabelsByID(doc)

	entries := make([]WatchOrderEntry, 0, len(rows))
	for _, row := range rows {
		typeName := strings.TrimSpace(typeByID[row.typeID])

		entries = append(entries, WatchOrderEntry{
			ID:       row.id,
			Type:     typeName,
			Title:    row.title,
			TitleAlt: row.alternativeTitle,
		})
	}

	return WatchOrderResult{ID: rootID, WatchOrder: entries}, nil
}
