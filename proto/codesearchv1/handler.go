package codesearchv1

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"

	"github.com/SCKelemen/codesearch"
	"github.com/SCKelemen/codesearch/hybrid"
	"github.com/SCKelemen/codesearch/linguist"
	"github.com/SCKelemen/codesearch/store"
)

const (
	SearchProcedurePath      = "/codesearch.v1.CodeSearchService/Search"
	IndexStatusProcedurePath = "/codesearch.v1.CodeSearchService/IndexStatus"
	defaultSearchLimit       = 20
)

type CodeSearchHandler struct {
	engine *codesearch.Engine
}

func NewCodeSearchHandler(engine *codesearch.Engine) *CodeSearchHandler {
	return &CodeSearchHandler{engine: engine}
}

func (h *CodeSearchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	switch r.URL.Path {
	case SearchProcedurePath:
		h.serveSearch(w, r)
	case IndexStatusProcedurePath:
		h.serveIndexStatus(w, r)
	default:
		writeError(w, http.StatusNotFound, "not found")
	}
}

func (h *CodeSearchHandler) serveSearch(w http.ResponseWriter, r *http.Request) {
	var request SearchRequest
	if err := decodeJSONBody(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if strings.TrimSpace(request.Query) == "" {
		writeError(w, http.StatusBadRequest, "query must not be empty")
		return
	}

	mode, err := normalizeMode(request.Mode)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	limit := int(request.Limit)
	if limit <= 0 {
		limit = defaultSearchLimit
	}

	results, err := h.engine.Search(r.Context(), request.Query, codesearch.WithLimit(limit), codesearch.WithMode(mode))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	response := SearchResponse{
		Query:   request.Query,
		Limit:   int32(limit),
		Mode:    modeLabel(mode),
		Results: make([]SearchResult, 0, len(results)),
	}
	filter := parseFilter(request.Filter)
	for _, result := range results {
		if !matchFilter(result.Path, filter) {
			continue
		}
		entry := SearchResult{
			Path:    result.Path,
			Line:    int32(result.Line),
			Score:   result.Score,
			Snippet: result.Snippet,
		}
		if len(result.Matches) != 0 {
			entry.Matches = make([]MatchRange, 0, len(result.Matches))
			for _, match := range result.Matches {
				entry.Matches = append(entry.Matches, MatchRange{Start: int32(match.Start), End: int32(match.End)})
			}
		}
		response.Results = append(response.Results, entry)
	}
	if err := writeJSON(w, http.StatusOK, response); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func (h *CodeSearchHandler) serveIndexStatus(w http.ResponseWriter, r *http.Request) {
	var request IndexStatusRequest
	if err := decodeJSONBody(r, &request); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	response, err := collectIndexStatus(r.Context(), h.engine)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := writeJSON(w, http.StatusOK, response); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func collectIndexStatus(ctx context.Context, engine *codesearch.Engine) (IndexStatusResponse, error) {
	response := IndexStatusResponse{Languages: make(map[string]int32)}
	cursor := ""
	for {
		documents, next, err := engine.Documents.List(ctx, store.WithLimit(512), store.WithCursor(cursor))
		if err != nil {
			return response, err
		}
		for _, document := range documents {
			response.FileCount++
			response.TotalBytes += document.Size
			response.IndexBytes += document.Size
			language := document.Language
			if language == "" {
				language = languageForPath(document.Path)
			}
			if language == "" {
				language = "unknown"
			}
			response.Languages[language]++
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
			return response, err
		}
		response.EmbeddingCount += int32(len(vectors))
		if next == "" {
			break
		}
		vectorCursor = next
	}

	return response, nil
}

func decodeJSONBody(r *http.Request, target any) error {
	if r.Body == nil {
		return nil
	}
	defer r.Body.Close()
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode json body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	return json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, statusCode int, message string) {
	_ = writeJSON(w, statusCode, map[string]string{"error": message})
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

func parseFilter(raw string) map[string]struct{} {
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

func matchFilter(path string, filters map[string]struct{}) bool {
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
