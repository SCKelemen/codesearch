package main

import (
	"context"
	"crypto/sha1"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SCKelemen/codesearch"
	"github.com/SCKelemen/codesearch/hybrid"
	"github.com/SCKelemen/codesearch/linguist"
	"github.com/SCKelemen/codesearch/store"
)

const (
	defaultIndexDir    = "./index"
	defaultListenAddr  = ":8080"
	defaultSearchLimit = 20
	searchAPIPath      = "/api/search"
	semanticDimensions = 64
)

type searchRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit,omitempty"`
	Mode  string `json:"mode,omitempty"`
}

type searchResponse struct {
	Query   string         `json:"query"`
	Limit   int            `json:"limit"`
	Mode    string         `json:"mode"`
	Source  string         `json:"source,omitempty"`
	Results []searchResult `json:"results"`
}

type searchResult struct {
	Path    string       `json:"path"`
	Line    int          `json:"line,omitempty"`
	Score   float64      `json:"score"`
	Snippet string       `json:"snippet,omitempty"`
	Matches []matchRange `json:"matches,omitempty"`
}

type matchRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type indexStats struct {
	FileCount      int
	TotalBytes     int64
	IndexBytes     int64
	LastModified   time.Time
	EmbeddingCount int
	Languages      map[string]int
}

type deterministicEmbedder struct {
	dimensions int
}

func (e deterministicEmbedder) Embed(_ context.Context, inputs []string) ([][]float32, error) {
	vectors := make([][]float32, len(inputs))
	for i, input := range inputs {
		vectors[i] = embedTextDeterministic(input, e.dimensions)
	}
	return vectors, nil
}

func (e deterministicEmbedder) Dimensions() int {
	if e.dimensions <= 0 {
		return semanticDimensions
	}
	return e.dimensions
}

func (e deterministicEmbedder) Model() string {
	return "csx-hash-v1"
}

func openEngine(indexDir string) (*codesearch.Engine, error) {
	return codesearch.Open(indexDir, codesearch.WithEmbedder(deterministicEmbedder{dimensions: semanticDimensions}))
}

func normalizeMode(raw string) (hybrid.SearchMode, error) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", "hybrid":
		return hybrid.Hybrid, nil
	case "lexical", "lexical_only":
		return hybrid.LexicalOnly, nil
	case "semantic", "semantic_only":
		return hybrid.SemanticOnly, nil
	default:
		return "", fmt.Errorf("unknown search mode %q", raw)
	}
}

func modeLabel(mode hybrid.SearchMode) string {
	switch mode {
	case hybrid.LexicalOnly:
		return "lexical"
	case hybrid.SemanticOnly:
		return "semantic"
	default:
		return "hybrid"
	}
}

func buildSearchResponse(query string, limit int, mode hybrid.SearchMode, source string, results []codesearch.Result) searchResponse {
	response := searchResponse{
		Query:   query,
		Limit:   limit,
		Mode:    modeLabel(mode),
		Source:  source,
		Results: make([]searchResult, 0, len(results)),
	}
	for _, result := range results {
		entry := searchResult{
			Path:    result.Path,
			Line:    result.Line,
			Score:   result.Score,
			Snippet: result.Snippet,
		}
		if len(result.Matches) != 0 {
			entry.Matches = make([]matchRange, 0, len(result.Matches))
			for _, match := range result.Matches {
				entry.Matches = append(entry.Matches, matchRange{Start: match.Start, End: match.End})
			}
		}
		response.Results = append(response.Results, entry)
	}
	return response
}

func parseLanguageFilter(raw string) map[string]struct{} {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	filters := make(map[string]struct{})
	for _, part := range strings.Split(raw, ",") {
		token := normalizeLanguageToken(part)
		if token == "" {
			continue
		}
		if language := linguist.LookupByName(token); language != nil {
			filters[normalizeLanguageToken(language.Name)] = struct{}{}
			continue
		}
		filters[token] = struct{}{}
	}
	if len(filters) == 0 {
		return nil
	}
	return filters
}

func languageAllowed(path string, filters map[string]struct{}) bool {
	if len(filters) == 0 {
		return true
	}
	language := languageForPath(path)
	if language != "" {
		if _, ok := filters[normalizeLanguageToken(language)]; ok {
			return true
		}
	}
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext != "" {
		if _, ok := filters[normalizeLanguageToken(ext)]; ok {
			return true
		}
	}
	if alias := languageAlias(ext); alias != "" {
		_, ok := filters[alias]
		return ok
	}
	return false
}

func languageForPath(path string) string {
	language := linguist.LookupByExtension(filepath.Ext(path))
	if language == nil {
		return ""
	}
	return language.Name
}

func normalizeLanguageToken(value string) string {
	value = strings.TrimSpace(strings.ToLower(value))
	value = strings.TrimPrefix(value, ".")
	value = strings.ReplaceAll(value, "_", "-")
	value = strings.ReplaceAll(value, " ", "-")
	if alias := languageAlias(value); alias != "" {
		return alias
	}
	return value
}

func languageAlias(value string) string {
	switch value {
	case "golang":
		return "go"
	case "ts":
		return "typescript"
	case "js":
		return "javascript"
	case "py":
		return "python"
	case "rb":
		return "ruby"
	case "rs":
		return "rust"
	case "yml":
		return "yaml"
	case "md":
		return "markdown"
	default:
		return ""
	}
}

func collectIndexStats(ctx context.Context, engine *codesearch.Engine, indexDir string) (indexStats, error) {
	stats := indexStats{Languages: make(map[string]int)}
	cursor := ""
	for {
		documents, next, err := engine.Documents.List(ctx, store.WithLimit(512), store.WithCursor(cursor))
		if err != nil {
			return stats, err
		}
		for _, doc := range documents {
			stats.FileCount++
			stats.TotalBytes += doc.Size
			language := doc.Language
			if language == "" {
				language = languageForPath(doc.Path)
			}
			if language == "" {
				language = "unknown"
			}
			stats.Languages[language]++
			if doc.UpdatedAt.After(stats.LastModified) {
				stats.LastModified = doc.UpdatedAt
			}
		}
		if next == "" {
			break
		}
		cursor = next
	}

	vectorCursor := ""
	for {
		vectors, next, err := engine.Vectors.List(ctx, store.WithLimit(512), store.WithCursor(vectorCursor))
		if err != nil {
			return stats, err
		}
		stats.EmbeddingCount += len(vectors)
		if next == "" {
			break
		}
		vectorCursor = next
	}

	indexBytes, err := directorySize(indexDir)
	if err != nil {
		return stats, err
	}
	stats.IndexBytes = indexBytes
	return stats, nil
}

func directorySize(root string) (int64, error) {
	var total int64
	err := filepath.Walk(root, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info == nil || info.IsDir() {
			return nil
		}
		total += info.Size()
		return nil
	})
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	return total, nil
}

func normalizeAddress(address string) string {
	trimmed := strings.TrimSpace(address)
	if trimmed == "" {
		return "http://127.0.0.1:8080"
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "http://" + trimmed
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return strings.TrimRight(trimmed, "/")
	}
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return strings.TrimRight(parsed.String(), "/")
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) error {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func writeAPIError(w http.ResponseWriter, statusCode int, message string) {
	_ = writeJSON(w, statusCode, map[string]string{"error": message})
}

func httpSearch(ctx context.Context, client *http.Client, address string, request searchRequest) (searchResponse, error) {
	mode, err := normalizeMode(request.Mode)
	if err != nil {
		return searchResponse{}, err
	}
	limit := request.Limit
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	endpoint := normalizeAddress(address) + searchAPIPath
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return searchResponse{}, fmt.Errorf("parse remote address: %w", err)
	}
	queryValues := parsed.Query()
	queryValues.Set("q", request.Query)
	queryValues.Set("limit", strconv.Itoa(limit))
	queryValues.Set("mode", modeLabel(mode))
	parsed.RawQuery = queryValues.Encode()

	httpRequest, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed.String(), nil)
	if err != nil {
		return searchResponse{}, fmt.Errorf("build remote request: %w", err)
	}
	response, err := client.Do(httpRequest)
	if err != nil {
		return searchResponse{}, fmt.Errorf("remote search failed: %w", err)
	}
	defer response.Body.Close()

	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		message := strings.TrimSpace(string(body))
		if message == "" {
			message = response.Status
		}
		return searchResponse{}, fmt.Errorf("remote search failed: %s", message)
	}

	var payload searchResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return searchResponse{}, fmt.Errorf("decode remote response: %w", err)
	}
	return payload, nil
}

func sortLanguageCounts(languages map[string]int) []string {
	type entry struct {
		language string
		count    int
	}
	entries := make([]entry, 0, len(languages))
	width := 0
	for language, count := range languages {
		entries = append(entries, entry{language: language, count: count})
		if len(language) > width {
			width = len(language)
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].count != entries[j].count {
			return entries[i].count > entries[j].count
		}
		return entries[i].language < entries[j].language
	})
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		lines = append(lines, fmt.Sprintf("%-*s %d", width, entry.language, entry.count))
	}
	return lines
}

func humanBytes(size int64) string {
	if size < 1024 {
		return fmt.Sprintf("%d B", size)
	}
	units := []string{"KB", "MB", "GB", "TB"}
	value := float64(size)
	unit := "B"
	for _, candidate := range units {
		value /= 1024
		unit = candidate
		if value < 1024 {
			break
		}
	}
	formatted := fmt.Sprintf("%.1f %s", value, unit)
	formatted = strings.TrimSuffix(formatted, ".0 "+unit)
	if !strings.HasSuffix(formatted, unit) {
		formatted += " " + unit
	}
	return formatted
}

func embedTextDeterministic(input string, dimensions int) []float32 {
	if dimensions <= 0 {
		dimensions = semanticDimensions
	}
	vector := make([]float32, dimensions)
	for _, token := range tokenize(input) {
		sum := sha1.Sum([]byte(token))
		index := int(sum[0]) % dimensions
		sign := float32(1)
		if sum[1]%2 == 1 {
			sign = -1
		}
		weight := float32(len(token)) / 8
		if weight < 0.5 {
			weight = 0.5
		}
		vector[index] += sign * weight
	}
	normalizeFloat32(vector)
	return vector
}

func tokenize(input string) []string {
	fields := strings.FieldsFunc(strings.ToLower(input), func(r rune) bool {
		return !(r == '_' || r == '-' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z'))
	})
	if len(fields) == 0 && strings.TrimSpace(input) != "" {
		return []string{strings.ToLower(strings.TrimSpace(input))}
	}
	return fields
}

func normalizeFloat32(values []float32) {
	var sum float64
	for _, value := range values {
		sum += float64(value * value)
	}
	if sum == 0 {
		return
	}
	norm := float32(1 / math.Sqrt(sum))
	for i := range values {
		values[i] *= norm
	}
}

func parseLimit(raw string, fallback int) int {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}

func requireQuery(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return errors.New("query must not be empty")
	}
	return nil
}
