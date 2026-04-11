package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"mal/internal/watchorder"
)

const defaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/135.0.0.0 Safari/537.36"

var idPattern = regexp.MustCompile(`/id/(\d+)`)

type seedPayload struct {
	IDs []int `json:"ids"`
}

type outputPayload struct {
	Data map[string][]watchorder.WatchOrderEntry `json:"data"`
}

func parseRootID(url string) (int, error) {
	match := idPattern.FindStringSubmatch(url)
	if len(match) != 2 {
		return 0, fmt.Errorf("invalid watch-order url: %s", url)
	}

	id, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, fmt.Errorf("invalid watch-order id in url %s: %w", url, err)
	}

	return id, nil
}

func fetchDocument(ctx context.Context, client *http.Client, url string) (*goquery.Document, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", defaultUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Referer", "https://chiaki.site/")
	req.Header.Set("Cache-Control", "no-cache")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d", resp.StatusCode)
	}

	return goquery.NewDocumentFromReader(resp.Body)
}

func parseRows(doc *goquery.Document) []watchorder.WatchOrderEntry {
	entries := make([]watchorder.WatchOrderEntry, 0)

	doc.Find("tr[data-id]").Each(func(_ int, selection *goquery.Selection) {
		rawID, ok := selection.Attr("data-id")
		if !ok {
			return
		}

		id, err := strconv.Atoi(strings.TrimSpace(rawID))
		if err != nil {
			return
		}

		typeLabel := ""
		rawTypeID, hasType := selection.Attr("data-type")
		if hasType {
			typeID := strings.TrimSpace(rawTypeID)
			typeLabel = mapTypeByID(doc, typeID)
		}

		title := strings.TrimSpace(selection.Find(".wo_title").First().Text())
		titleAlt := strings.TrimSpace(selection.Find(".uk-text-small").First().Text())

		entries = append(entries, watchorder.WatchOrderEntry{
			ID:       id,
			Type:     typeLabel,
			Title:    title,
			TitleAlt: titleAlt,
		})
	})

	return entries
}

func mapTypeByID(doc *goquery.Document, typeID string) string {
	label := ""
	doc.Find("#wo_type_filter label").EachWithBreak(func(_ int, selection *goquery.Selection) bool {
		input := selection.Find("input[type='checkbox']")
		value, ok := input.Attr("value")
		if ok && strings.TrimSpace(value) == typeID {
			label = strings.TrimSpace(selection.Text())
			return false
		}
		return true
	})

	return label
}

func parseIDList(value string) ([]int, error) {
	if strings.TrimSpace(value) == "" {
		return []int{}, nil
	}

	parts := strings.Split(value, ",")
	ids := make([]int, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}

		id, err := strconv.Atoi(trimmed)
		if err != nil {
			return nil, fmt.Errorf("invalid id %q: %w", trimmed, err)
		}
		ids = append(ids, id)
	}

	return ids, nil
}

func loadSeedIDs(path string) ([]int, error) {
	if strings.TrimSpace(path) == "" {
		return []int{}, nil
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	payload := seedPayload{}
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, err
	}

	return payload.IDs, nil
}

func sortAndUnique(ids []int) []int {
	seen := make(map[int]bool)
	unique := make([]int, 0, len(ids))
	for _, id := range ids {
		if id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		unique = append(unique, id)
	}

	sort.Ints(unique)
	return unique
}

func main() {
	outputPath := flag.String("out", "data/watch_order.json", "output json file path")
	seedPath := flag.String("seed", "tmp/watch_order_seed_ids.json", "seed json file path with {\"ids\": [...]} ")
	idList := flag.String("ids", "", "comma-separated MAL ids")
	flag.Parse()

	idsFromFlag, err := parseIDList(*idList)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	idsFromSeed, err := loadSeedIDs(*seedPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to load seed ids: %v\n", err)
		os.Exit(1)
	}

	allIDs := sortAndUnique(append(idsFromSeed, idsFromFlag...))
	if len(allIDs) == 0 {
		fmt.Fprintln(os.Stderr, "error: no ids provided (use -seed and/or -ids)")
		os.Exit(1)
	}

	httpClient := &http.Client{Timeout: 12 * time.Second}
	ctx := context.Background()

	data := make(map[string][]watchorder.WatchOrderEntry, len(allIDs))
	for _, id := range allIDs {
		url := fmt.Sprintf("https://chiaki.site/?/tools/watch_order/id/%d", id)
		if _, err := parseRootID(url); err != nil {
			continue
		}

		doc, err := fetchDocument(ctx, httpClient, url)
		if err != nil {
			continue
		}

		if doc.Find("#wo_list").Length() == 0 {
			continue
		}

		data[strconv.Itoa(id)] = parseRows(doc)
	}

	encoded, err := json.Marshal(outputPayload{Data: data})
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to encode output: %v\n", err)
		os.Exit(1)
	}

	outputDirectory := filepath.Dir(*outputPath)
	if err := os.MkdirAll(outputDirectory, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to create data directory: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(*outputPath, encoded, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to write output %q: %v\n", *outputPath, err)
		os.Exit(1)
	}

	fmt.Printf("wrote watch-order dataset for %d ids to %s\n", len(data), *outputPath)
}
