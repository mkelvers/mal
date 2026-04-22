package playback

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	allAnimeBaseURL    = "https://api.allanime.day"
	allAnimeReferer    = "https://allmanga.to"
	defaultUserAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64; rv:109.0) Gecko/20100101 Firefox/121.0"
	allAnimeAESKey     = "ALLANIME_AES_KEY"
	aniCliRawSourceURL = "https://raw.githubusercontent.com/pystardust/ani-cli/master/ani-cli"
	aniCliKeyRegex     = `allanime_key="\$\(printf '%s' '([^']+)'`
	consensusThreshold = 2
)

var (
	aesKeys          = []string{"Xot36i3lK3:v1", "SimtVuagFbGR2K7P"}
	cachedKey        string
	cachedKeyFetched time.Time
	keyCacheDuration = 1 * time.Hour
	forkSources      = []string{
		"https://raw.githubusercontent.com/pystardust/ani-cli/master/ani-cli",
		"https://raw.githubusercontent.com/justfoolingaround/ani-cli/master/ani-cli",
		"https://raw.githubusercontent.com/justfoolingaround/ani-cli-mpv/master/ani-cli",
		"https://raw.githubusercontent.com/An1sora/ani-cli/master/ani-cli",
		"https://raw.githubusercontent.com/sdaqo/ani-cli/master/ani-cli",
	}
)

type searchResult struct {
	ID    string
	MalID string
	Name  string
}

type allAnimeClient struct {
	httpClient *http.Client
	extractor  *providerExtractor
}

func newAllAnimeClient() *allAnimeClient {
	return &allAnimeClient{
		httpClient: &http.Client{Timeout: 12 * time.Second},
		extractor:  newProviderExtractor(),
	}
}

func (c *allAnimeClient) graphqlRequest(ctx context.Context, query string, variables map[string]any) (map[string]any, error) {
	payload := map[string]any{
		"query":     query,
		"variables": variables,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal graphql payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, allAnimeBaseURL+"/api", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create graphql request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", allAnimeReferer)
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("execute graphql request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("read graphql response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("graphql status %d", resp.StatusCode)
	}

	var parsed map[string]any
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode graphql response: %w", err)
	}

	if errs, ok := parsed["errors"].([]any); ok && len(errs) > 0 {
		return nil, fmt.Errorf("graphql error: %v", errs[0])
	}

	return parsed, nil
}

func (c *allAnimeClient) Search(ctx context.Context, query string, mode string) ([]searchResult, error) {
	graphqlQuery := `query($search: SearchInput, $limit: Int, $page: Int, $translationType: VaildTranslationTypeEnumType, $countryOrigin: VaildCountryOriginEnumType) {
		shows(search: $search, limit: $limit, page: $page, translationType: $translationType, countryOrigin: $countryOrigin) {
			edges {
				_id
				malId
				name
			}
		}
	}`

	variables := map[string]any{
		"search": map[string]any{
			"allowAdult":   false,
			"allowUnknown": false,
			"query":        query,
		},
		"limit":           40,
		"page":            1,
		"translationType": mode,
		"countryOrigin":   "ALL",
	}

	result, err := c.graphqlRequest(ctx, graphqlQuery, variables)
	if err != nil {
		return nil, err
	}

	data, ok := result["data"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid search response")
	}

	shows, ok := data["shows"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid shows payload")
	}

	edges, ok := shows["edges"].([]any)
	if !ok {
		return nil, fmt.Errorf("invalid search edges")
	}

	out := make([]searchResult, 0, len(edges))
	for _, edge := range edges {
		item, ok := edge.(map[string]any)
		if !ok {
			continue
		}

		id, _ := item["_id"].(string)
		malID, _ := item["malId"].(string)
		name, _ := item["name"].(string)
		if unquoted, err := strconv.Unquote("\"" + name + "\""); err == nil {
			name = unquoted
		}
		name = strings.TrimSpace(name)

		if id == "" {
			continue
		}

		out = append(out, searchResult{ID: id, MalID: malID, Name: name})
	}

	return out, nil
}

func (c *allAnimeClient) GetEpisodes(ctx context.Context, showID string, mode string) ([]string, error) {
	graphqlQuery := `query($showId: String!) {
		show(_id: $showId) {
			availableEpisodesDetail
		}
	}`

	result, err := c.graphqlRequest(ctx, graphqlQuery, map[string]any{"showId": showID})
	if err != nil {
		return nil, err
	}

	data, ok := result["data"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid episode response")
	}

	show, ok := data["show"].(map[string]any)
	if !ok || show == nil {
		return nil, fmt.Errorf("show not found")
	}

	detail, ok := show["availableEpisodesDetail"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid episodes detail")
	}

	rawList, ok := detail[mode].([]any)
	if !ok {
		return nil, fmt.Errorf("no episodes for mode %s", mode)
	}

	episodes := make([]string, 0, len(rawList))
	for _, item := range rawList {
		episode, ok := item.(string)
		if !ok {
			continue
		}

		episode = strings.TrimSpace(episode)
		if episode == "" {
			continue
		}

		episodes = append(episodes, episode)
	}

	return episodes, nil
}

func buildStreamSource(url, sourceType, provider string) StreamSource {
	return StreamSource{
		URL:      url,
		Provider: provider,
		Type:     sourceType,
		Referer:  allAnimeReferer,
	}
}

func (c *allAnimeClient) GetEpisodeSources(ctx context.Context, showID string, episode string, mode string) ([]StreamSource, error) {
	graphqlQuery := `query($showId: String!, $translationType: VaildTranslationTypeEnumType!, $episodeString: String!) {
		episode(showId: $showId, translationType: $translationType, episodeString: $episodeString) {
			sourceUrls
		}
	}`

	variables := map[string]any{
		"showId":          showID,
		"translationType": mode,
		"episodeString":   episode,
	}

	result, err := c.graphqlRequest(ctx, graphqlQuery, variables)
	if err != nil {
		return nil, err
	}

	data, ok := result["data"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("invalid source response")
	}

	episodeData, err := extractEpisodeData(data)
	if err != nil {
		return nil, err
	}

	rawSourceURLs, ok := episodeData["sourceUrls"].([]any)
	if !ok || len(rawSourceURLs) == 0 {
		return nil, fmt.Errorf("no source urls")
	}

	references := buildSourceReferences(rawSourceURLs)
	if len(references) == 0 {
		return nil, fmt.Errorf("no source references")
	}

	out := make([]StreamSource, 0, len(references))
	for _, ref := range references {
		target := strings.TrimSpace(ref.URL)
		if target == "" {
			continue
		}

		if strings.HasPrefix(target, "http://") || strings.HasPrefix(target, "https://") {
			sourceType := detectStreamType(target)
			if sourceType == "unknown" {
				sourceType = detectEmbedType(target)
			}

			out = append(out, buildStreamSource(target, sourceType, ref.Name))
			continue
		}

		decoded := decodeSourceURL(target)
		if decoded == "" {
			continue
		}

		if strings.HasPrefix(decoded, "http://") || strings.HasPrefix(decoded, "https://") {
			sourceType := detectStreamType(decoded)
			if sourceType == "unknown" {
				sourceType = detectEmbedType(decoded)
			}

			out = append(out, buildStreamSource(decoded, sourceType, ref.Name))
			continue
		}

		if !strings.HasPrefix(decoded, "/") {
			decoded = "/" + decoded
		}

		extracted, err := c.extractor.ExtractVideoLinks(ctx, decoded)
		if err != nil {
			continue
		}

		out = append(out, extracted...)
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("no playable sources extracted")
	}

	return out, nil
}

type sourceReference struct {
	URL  string
	Name string
}

func buildSourceReferences(rawSourceURLs []any) []sourceReference {
	priorityOrder := []string{"default", "yt-mp4", "s-mp4", "luf-mp4"}
	prioritySet := map[string]struct{}{"default": {}, "yt-mp4": {}, "s-mp4": {}, "luf-mp4": {}}

	prioritized := make(map[string]sourceReference)
	fallback := make([]sourceReference, 0, len(rawSourceURLs))
	seen := make(map[string]struct{})

	for _, source := range rawSourceURLs {
		item, ok := source.(map[string]any)
		if !ok {
			continue
		}

		sourceURL, _ := item["sourceUrl"].(string)
		sourceName, _ := item["sourceName"].(string)
		sourceURL = strings.TrimSpace(sourceURL)
		sourceName = strings.TrimSpace(sourceName)
		if sourceURL == "" {
			continue
		}

		if _, exists := seen[sourceURL]; exists {
			continue
		}
		seen[sourceURL] = struct{}{}

		ref := sourceReference{URL: sourceURL, Name: sourceName}
		normalized := strings.ToLower(sourceName)
		if _, prioritizedProvider := prioritySet[normalized]; prioritizedProvider {
			if _, exists := prioritized[normalized]; !exists {
				prioritized[normalized] = ref
			}
			continue
		}

		fallback = append(fallback, ref)
	}

	ordered := make([]sourceReference, 0, len(prioritized)+len(fallback))
	for _, provider := range priorityOrder {
		if ref, ok := prioritized[provider]; ok {
			ordered = append(ordered, ref)
		}
	}

	ordered = append(ordered, fallback...)
	return ordered
}

func extractEpisodeData(data map[string]any) (map[string]any, error) {
	episodeData, ok := data["episode"].(map[string]any)
	if ok && episodeData != nil {
		return episodeData, nil
	}

	toBeParsed, ok := data["tobeparsed"].(string)
	if !ok || strings.TrimSpace(toBeParsed) == "" {
		return nil, fmt.Errorf("episode not found")
	}

	decoded, err := decryptTobeparsed(toBeParsed)
	if err != nil {
		return nil, fmt.Errorf("decode episode payload: %w", err)
	}

	var parsed map[string]any
	if err := json.Unmarshal(decoded, &parsed); err != nil {
		return nil, fmt.Errorf("parse decoded payload: %w", err)
	}

	episodeData, ok = parsed["episode"].(map[string]any)
	if !ok || episodeData == nil {
		return nil, fmt.Errorf("decoded payload missing episode")
	}

	return episodeData, nil
}

func decryptTobeparsed(encoded string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("base64 decode failed: %w", err)
	}

	if len(raw) < 29 {
		return nil, fmt.Errorf("encrypted payload too short")
	}

	version := raw[0]
	iv := raw[1:13]
	cipherText := raw[13 : len(raw)-16]

	for _, keyStr := range getAllKeys() {
		key := sha256.Sum256([]byte(keyStr))

		block, err := aes.NewCipher(key[:])
		if err != nil {
			continue
		}

		if version == 1 {
			plainText, err := tryDecryptCTR(block, iv, cipherText)
			if err == nil && json.Valid(plainText) {
				return plainText, nil
			}
		}

		gcm, err := cipher.NewGCM(block)
		if err == nil {
			tag := raw[len(raw)-16:]
			combined := append(append([]byte{}, cipherText...), tag...)
			plainText, openErr := gcm.Open(nil, iv, combined, nil)
			if openErr == nil && json.Valid(plainText) {
				return plainText, nil
			}
		}
	}

	return nil, fmt.Errorf("decryption failed")
}

func getAllKeys() []string {
	keys := make([]string, 0, len(aesKeys)+1)

	if cachedKey != "" && time.Since(cachedKeyFetched) < keyCacheDuration {
		keys = append(keys, cachedKey)
	}

	keys = append(keys, aesKeys...)
	return keys
}

func tryDecryptCTR(block cipher.Block, iv []byte, cipherText []byte) ([]byte, error) {
	ctrIV := append([]byte{}, iv...)
	ctrIV = append(ctrIV, 0x00, 0x00, 0x00, 0x02)
	ctr := cipher.NewCTR(block, ctrIV)
	plainText := make([]byte, len(cipherText))
	ctr.XORKeyStream(plainText, cipherText)
	return plainText, nil
}

func getAESKey() string {
	if envKey := os.Getenv(allAnimeAESKey); envKey != "" {
		return envKey
	}

	if cachedKey != "" && time.Since(cachedKeyFetched) < keyCacheDuration {
		return cachedKey
	}

	validatedKey := validateKeys()
	if validatedKey != "" {
		cachedKey = validatedKey
		cachedKeyFetched = time.Now()
		return cachedKey
	}

	if len(aesKeys) > 0 {
		return aesKeys[0]
	}

	return ""
}

func validateKeys() string {
	fetchedKeys := fetchKeyFromForks()
	allKeys := append([]string{fetchedKeys}, aesKeys...)

	for _, keyStr := range allKeys {
		if keyStr == "" {
			continue
		}

		raw, err := base64.StdEncoding.DecodeString(getTestPayload())
		if err != nil {
			continue
		}

		if len(raw) < 29 {
			continue
		}

		version := raw[0]
		iv := raw[1:13]
		cipherText := raw[13 : len(raw)-16]

		key := sha256.Sum256([]byte(keyStr))
		block, err := aes.NewCipher(key[:])
		if err != nil {
			continue
		}

		var plainText []byte

		if version == 1 {
			plainText, _ = tryDecryptCTR(block, iv, cipherText)
		}

		if len(plainText) == 0 || !json.Valid(plainText) {
			gcm, err := cipher.NewGCM(block)
			if err != nil {
				continue
			}
			tag := raw[len(raw)-16:]
			combined := append(append([]byte{}, cipherText...), tag...)
			plainText, err = gcm.Open(nil, iv, combined, nil)
			if err != nil || !json.Valid(plainText) {
				continue
			}
		}

		var parsed map[string]any
		if err := json.Unmarshal(plainText, &parsed); err != nil {
			continue
		}

		episodeData, ok := parsed["episode"].(map[string]any)
		if !ok || episodeData == nil {
			continue
		}

		sourceUrls, ok := episodeData["sourceUrls"].([]any)
		if !ok || len(sourceUrls) == 0 {
			continue
		}

		return keyStr
	}

	return ""
}

var testPayloadCache string

func getTestPayload() string {
	if testPayloadCache != "" {
		return testPayloadCache
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	searchQuery := `query($search: SearchInput, $limit: Int, $page: Int, $translationType: VaildTranslationTypeEnumType, $countryOrigin: VaildCountryOriginEnumType) {
		searchResults(search: $search, limit: $limit, page: $page, translationType: $translationType, countryOrigin: $countryOrigin) {
			results {
				_id
			}
		}
	}`

	searchVariables := map[string]any{
		"search":          map[string]any{"query": "pokemon"},
		"limit":           1,
		"page":            1,
		"translationType": "SUB",
		"countryOrigin":   "JP",
	}

	searchBody, _ := json.Marshal(map[string]any{
		"query":     searchQuery,
		"variables": searchVariables,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, allAnimeBaseURL+"/api", bytes.NewReader(searchBody))
	if err != nil {
		return ""
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Referer", allAnimeReferer)
	req.Header.Set("User-Agent", defaultUserAgent)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil || resp.StatusCode != http.StatusOK {
		return ""
	}

	var searchResult struct {
		Data struct {
			SearchResults struct {
				Results []struct {
					ID string `json:"_id"`
				} `json:"results"`
			} `json:"searchResults"`
		} `json:"data"`
	}

	if err := json.Unmarshal(body, &searchResult); err != nil {
		return ""
	}

	if len(searchResult.Data.SearchResults.Results) == 0 {
		return ""
	}

	showID := searchResult.Data.SearchResults.Results[0].ID

	episodeQuery := `query($showId: String!, $translationType: VaildTranslationTypeEnumType!, $episodeString: String!) {
		episode(showId: $showId, translationType: $translationType, episodeString: $episodeString) {
			tobeparsed
		}
	}`

	episodeVariables := map[string]any{
		"showId":          showID,
		"translationType": "SUB",
		"episodeString":   "1",
	}

	episodeBody, _ := json.Marshal(map[string]any{
		"query":     episodeQuery,
		"variables": episodeVariables,
	})

	episodeReq, err := http.NewRequestWithContext(ctx, http.MethodPost, allAnimeBaseURL+"/api", bytes.NewReader(episodeBody))
	if err != nil {
		return ""
	}
	episodeReq.Header.Set("Content-Type", "application/json")
	episodeReq.Header.Set("Referer", allAnimeReferer)
	episodeReq.Header.Set("User-Agent", defaultUserAgent)

	episodeResp, err := http.DefaultClient.Do(episodeReq)
	if err != nil {
		return ""
	}
	defer episodeResp.Body.Close()

	episodeBodyBytes, err := io.ReadAll(episodeResp.Body)
	if err != nil || episodeResp.StatusCode != http.StatusOK {
		return ""
	}

	var episodeResult struct {
		Data struct {
			Episode struct {
				ToBeParsed string `json:"tobeparsed"`
			} `json:"episode"`
		} `json:"data"`
	}

	if err := json.Unmarshal(episodeBodyBytes, &episodeResult); err != nil {
		return ""
	}

	testPayloadCache = episodeResult.Data.Episode.ToBeParsed
	return testPayloadCache
}

func fetchKeyFromForks() string {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	type fetchResult struct {
		key  string
		err  error
		body string
	}

	results := make(chan fetchResult, len(forkSources))

	for _, source := range forkSources {
		go func(source string) {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
			if err != nil {
				results <- fetchResult{err: err}
				return
			}
			req.Header.Set("User-Agent", defaultUserAgent)

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				results <- fetchResult{err: err}
				return
			}
			defer resp.Body.Close()

			body, err := io.ReadAll(resp.Body)
			if err != nil || resp.StatusCode != 200 {
				results <- fetchResult{err: fmt.Errorf("bad response")}
				return
			}

			results <- fetchResult{body: string(body)}
		}(source)
	}

	keyCounts := make(map[string]int)
	deadline := time.After(12 * time.Second)
	for range forkSources {
		select {
		case r := <-results:
			if r.err != nil || r.body == "" {
				continue
			}
			if key := extractKey(r.body); key != "" {
				keyCounts[key]++
				if keyCounts[key] >= consensusThreshold {
					return key
				}
			}
		case <-deadline:
			goto checkConsensus
		}
	}

checkConsensus:
	for key, count := range keyCounts {
		if count >= consensusThreshold {
			return key
		}
	}

	for key := range keyCounts {
		return key
	}

	return ""
}

func extractKey(scriptContent string) string {
	re := regexp.MustCompile(aniCliKeyRegex)
	matches := re.FindStringSubmatch(scriptContent)
	if len(matches) < 2 {
		return ""
	}
	return matches[1]
}

func decodeSourceURL(encoded string) string {
	if encoded == "" {
		return ""
	}

	encoded = strings.TrimPrefix(encoded, "--")

	substitutions := map[string]string{
		"79": "A", "7a": "B", "7b": "C", "7c": "D", "7d": "E",
		"7e": "F", "7f": "G", "70": "H", "71": "I", "72": "J",
		"73": "K", "74": "L", "75": "M", "76": "N", "77": "O",
		"68": "P", "69": "Q", "6a": "R", "6b": "S", "6c": "T",
		"6d": "U", "6e": "V", "6f": "W", "60": "X", "61": "Y",
		"62": "Z",
		"59": "a", "5a": "b", "5b": "c", "5c": "d", "5d": "e",
		"5e": "f", "5f": "g", "50": "h", "51": "i", "52": "j",
		"53": "k", "54": "l", "55": "m", "56": "n", "57": "o",
		"48": "p", "49": "q", "4a": "r", "4b": "s", "4c": "t",
		"4d": "u", "4e": "v", "4f": "w", "40": "x", "41": "y",
		"42": "z",
		"08": "0", "09": "1", "0a": "2", "0b": "3", "0c": "4",
		"0d": "5", "0e": "6", "0f": "7", "00": "8", "01": "9",
		"15": "-", "16": ".", "67": "_", "46": "~", "02": ":",
		"17": "/", "07": "?", "1b": "#", "63": "[", "65": "]",
		"78": "@", "19": "!", "1c": "$", "1e": "&", "10": "(",
		"11": ")", "12": "*", "13": "+", "14": ",", "03": ";",
		"05": "=", "1d": "%",
	}

	var result strings.Builder
	for idx := 0; idx < len(encoded); {
		if idx+2 <= len(encoded) {
			pair := encoded[idx : idx+2]
			if sub, ok := substitutions[pair]; ok {
				result.WriteString(sub)
				idx += 2
				continue
			}
		}

		result.WriteByte(encoded[idx])
		idx++
	}

	decoded := result.String()
	if strings.Contains(decoded, "/clock") && !strings.Contains(decoded, "/clock.json") {
		decoded = strings.Replace(decoded, "/clock", "/clock.json", 1)
	}

	return decoded
}

func detectStreamType(sourceURL string) string {
	lower := strings.ToLower(sourceURL)
	if strings.Contains(lower, ".m3u8") || strings.Contains(lower, "master.m3u8") {
		return "m3u8"
	}

	if strings.Contains(lower, ".mp4") {
		return "mp4"
	}

	return "unknown"
}

func detectEmbedType(rawURL string) string {
	lower := strings.ToLower(rawURL)
	embedHosts := []string{"streamwish", "streamsb", "mp4upload", "ok.ru", "gogoplay", "streamlare"}
	for _, host := range embedHosts {
		if strings.Contains(lower, host) {
			return "embed"
		}
	}

	return "unknown"
}
