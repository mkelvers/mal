package watchorder

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

const defaultUserAgent = "anime-relations-scraper/1.0 (+https://github.com/mkelvers/anime-relations)"

var idPattern = regexp.MustCompile(`/id/(\d+)`)

var ErrInvalidWatchOrderURL = errors.New("invalid watch order url")

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

func fetchDocument(ctx context.Context, httpClient *http.Client, url string) (*goquery.Document, error) {
	client := httpClient
	if client == nil {
		client = http.DefaultClient
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	request.Header.Set("User-Agent", defaultUserAgent)

	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", response.StatusCode)
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

func FetchWatchOrder(ctx context.Context, httpClient *http.Client, url string) (WatchOrderResult, error) {
	rootID, err := parseRootID(url)
	if err != nil {
		return WatchOrderResult{}, err
	}

	doc, err := fetchDocument(ctx, httpClient, url)
	if err != nil {
		return WatchOrderResult{}, err
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
