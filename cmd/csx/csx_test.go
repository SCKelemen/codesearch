package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/SCKelemen/codesearch"
)

func TestSearchAPIHandler(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	rootDir := t.TempDir()
	indexDir := filepath.Join(t.TempDir(), "index")
	filePath := writeTestFile(t, rootDir, "search_target.go", "package main\n\nfunc UniqueSearchTarget() string {\n\treturn \"needle\"\n}\n")
	writeTestFile(t, rootDir, "other.go", "package main\n\nfunc OtherFunction() {}\n")

	engine, err := openEngine(indexDir)
	if err != nil {
		t.Fatalf("open engine: %v", err)
	}
	defer func() {
		_ = engine.Close()
	}()

	if err := engine.Index(ctx, filePath); err != nil {
		t.Fatalf("index target file: %v", err)
	}
	if err := engine.Index(ctx, filepath.Join(rootDir, "other.go")); err != nil {
		t.Fatalf("index other file: %v", err)
	}

	server := httptest.NewServer(newSearchHandler(t, engine))
	defer server.Close()

	response, err := http.Get(server.URL + searchAPIPath + "?q=UniqueSearchTarget&mode=lexical&limit=5")
	if err != nil {
		t.Fatalf("get search response: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status code: got %d want %d", response.StatusCode, http.StatusOK)
	}
	if contentType := response.Header.Get("Content-Type"); contentType != "application/json; charset=utf-8" {
		t.Fatalf("unexpected content type: got %q", contentType)
	}

	var payload searchResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode search response: %v", err)
	}

	if payload.Query != "UniqueSearchTarget" {
		t.Fatalf("unexpected query: got %q", payload.Query)
	}
	if payload.Limit != 5 {
		t.Fatalf("unexpected limit: got %d want 5", payload.Limit)
	}
	if payload.Mode != "lexical" {
		t.Fatalf("unexpected mode: got %q want lexical", payload.Mode)
	}
	if payload.Source != "remote" {
		t.Fatalf("unexpected source: got %q want remote", payload.Source)
	}
	if len(payload.Results) != 1 {
		t.Fatalf("unexpected results length: got %d want 1", len(payload.Results))
	}

	result := payload.Results[0]
	if result.Path != filepath.ToSlash(filepath.Clean(filePath)) {
		t.Fatalf("unexpected result path: got %q want %q", result.Path, filepath.ToSlash(filepath.Clean(filePath)))
	}
	if result.Line != 3 {
		t.Fatalf("unexpected result line: got %d want 3", result.Line)
	}
	if result.Score <= 0 {
		t.Fatalf("expected positive score, got %f", result.Score)
	}
	if !strings.Contains(result.Snippet, "UniqueSearchTarget") {
		t.Fatalf("unexpected snippet: %q", result.Snippet)
	}
	if len(result.Matches) != 1 {
		t.Fatalf("unexpected match count: got %d want 1", len(result.Matches))
	}
	if result.Matches[0].Start >= result.Matches[0].End {
		t.Fatalf("invalid match range: %+v", result.Matches[0])
	}
}

func TestSearchAPIHandlerErrors(t *testing.T) {
	t.Parallel()

	engine := codesearch.New(
		codesearch.WithMemoryStore(),
		codesearch.WithEmbedder(deterministicEmbedder{dimensions: semanticDimensions}),
	)
	defer func() {
		_ = engine.Close()
	}()

	server := httptest.NewServer(newSearchHandler(t, engine))
	defer server.Close()

	t.Run("method not allowed", func(t *testing.T) {

		request, err := http.NewRequest(http.MethodPost, server.URL+searchAPIPath, nil)
		if err != nil {
			t.Fatalf("new request: %v", err)
		}
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatalf("do request: %v", err)
		}
		defer response.Body.Close()

		assertAPIError(t, response, http.StatusMethodNotAllowed, "method not allowed")
	})

	t.Run("missing query", func(t *testing.T) {

		response, err := http.Get(server.URL + searchAPIPath)
		if err != nil {
			t.Fatalf("get response: %v", err)
		}
		defer response.Body.Close()

		assertAPIError(t, response, http.StatusBadRequest, "query must not be empty")
	})

	t.Run("invalid mode", func(t *testing.T) {

		response, err := http.Get(server.URL + searchAPIPath + "?q=test&mode=invalid")
		if err != nil {
			t.Fatalf("get response: %v", err)
		}
		defer response.Body.Close()

		assertAPIError(t, response, http.StatusBadRequest, "unknown search mode \"invalid\"")
	})
}

func TestNormalizeMode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name    string
		raw     string
		want    string
		wantErr string
	}{
		{name: "default hybrid", raw: "", want: "hybrid"},
		{name: "hybrid trimmed", raw: "  HyBrId  ", want: "hybrid"},
		{name: "lexical", raw: "lexical", want: "lexical_only"},
		{name: "lexical alias", raw: "lexical_only", want: "lexical_only"},
		{name: "semantic", raw: "semantic", want: "semantic_only"},
		{name: "semantic alias", raw: "semantic_only", want: "semantic_only"},
		{name: "invalid", raw: "vector", wantErr: "unknown search mode \"vector\""},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizeMode(testCase.raw)
			if testCase.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error %q, got nil", testCase.wantErr)
				}
				if err.Error() != testCase.wantErr {
					t.Fatalf("unexpected error: got %q want %q", err.Error(), testCase.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalize mode: %v", err)
			}
			if string(got) != testCase.want {
				t.Fatalf("unexpected mode: got %q want %q", got, testCase.want)
			}
		})
	}
}

func TestIndexCommandLogic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	rootDir := t.TempDir()
	outputDir := filepath.Join(rootDir, "index")
	goFileOne := writeTestFile(t, rootDir, filepath.Join("pkg", "alpha.go"), "package pkg\n\nfunc AlphaSearchToken() string { return \"alpha\" }\n")
	goFileTwo := writeTestFile(t, rootDir, filepath.Join("pkg", "beta.go"), "package pkg\n\nfunc BetaSearchToken() string { return \"beta\" }\n")
	writeTestFile(t, rootDir, "README.md", "AlphaSearchToken should not be indexed by the Go-only filter.\n")
	writeBinaryFile(t, filepath.Join(rootDir, "pkg", "binary.dat"), []byte{0x00, 0x01, 0x02, 0x03})
	writeTestFile(t, outputDir, "stale.txt", "this file lives inside the output directory and must be skipped\n")

	inputs, err := collectIndexInputs(rootDir, outputDir, parseLanguageFilter("go"))
	if err != nil {
		t.Fatalf("collect index inputs: %v", err)
	}

	wantInputs := []string{goFileOne, goFileTwo}
	if len(inputs) != len(wantInputs) {
		t.Fatalf("unexpected number of inputs: got %d want %d (%v)", len(inputs), len(wantInputs), inputs)
	}
	for i := range wantInputs {
		if inputs[i] != wantInputs[i] {
			t.Fatalf("unexpected input at %d: got %q want %q", i, inputs[i], wantInputs[i])
		}
	}

	engine, err := openEngine(outputDir)
	if err != nil {
		t.Fatalf("open engine: %v", err)
	}
	defer func() {
		_ = engine.Close()
	}()

	for _, input := range inputs {
		if err := engine.Index(ctx, input, codesearch.WithEmbeddings(false)); err != nil {
			t.Fatalf("index %s: %v", input, err)
		}
	}

	results, err := engine.Search(ctx, "AlphaSearchToken")
	if err != nil {
		t.Fatalf("search indexed content: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("unexpected result count: got %d want 1", len(results))
	}
	if results[0].Path != filepath.ToSlash(filepath.Clean(goFileOne)) {
		t.Fatalf("unexpected result path: got %q want %q", results[0].Path, filepath.ToSlash(filepath.Clean(goFileOne)))
	}
	if !strings.Contains(results[0].Snippet, "AlphaSearchToken") {
		t.Fatalf("unexpected snippet: %q", results[0].Snippet)
	}
}

func TestStatusOutput(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	rootDir := t.TempDir()
	indexDir := filepath.Join(t.TempDir(), "index")
	firstFile := writeTestFile(t, rootDir, "first.go", "package main\n\nfunc FirstStatusToken() string {\n\treturn \"first\"\n}\n")
	secondFile := writeTestFile(t, rootDir, filepath.Join("nested", "second.go"), "package nested\n\nfunc SecondStatusToken() string {\n\treturn \"second\"\n}\n")

	engine, err := openEngine(indexDir)
	if err != nil {
		t.Fatalf("open engine: %v", err)
	}
	if err := engine.Index(ctx, firstFile); err != nil {
		t.Fatalf("index first file: %v", err)
	}
	if err := engine.Index(ctx, secondFile); err != nil {
		t.Fatalf("index second file: %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatalf("close engine: %v", err)
	}

	statusEngine, err := openEngine(indexDir)
	if err != nil {
		t.Fatalf("reopen engine: %v", err)
	}
	defer func() {
		_ = statusEngine.Close()
	}()

	stats, err := collectIndexStats(ctx, statusEngine, indexDir)
	if err != nil {
		t.Fatalf("collect index stats: %v", err)
	}

	if stats.FileCount != 2 {
		t.Fatalf("unexpected file count: got %d want 2", stats.FileCount)
	}
	if stats.EmbeddingCount != 2 {
		t.Fatalf("unexpected embedding count: got %d want 2", stats.EmbeddingCount)
	}
	if stats.TotalBytes <= 0 {
		t.Fatalf("expected positive total bytes, got %d", stats.TotalBytes)
	}
	if stats.IndexBytes <= 0 {
		t.Fatalf("expected positive index bytes, got %d", stats.IndexBytes)
	}
	if stats.LastModified.IsZero() {
		t.Fatal("expected non-zero last modified time")
	}
	language := languageForPath(firstFile)
	if language == "" {
		t.Fatal("expected language detection for Go file")
	}
	if stats.Languages[language] != 2 {
		t.Fatalf("unexpected language count for %q: got %d want 2", language, stats.Languages[language])
	}

	var output bytes.Buffer
	if err := renderStatus(&output, indexDir, stats); err != nil {
		t.Fatalf("render status: %v", err)
	}

	cleanOutput := stripANSI(output.String())
	for _, want := range []string{"Index status", "files:", "2", "embeddings:", "Languages", language} {
		if !strings.Contains(cleanOutput, want) {
			t.Fatalf("status output %q does not contain %q", cleanOutput, want)
		}
	}
}

func newSearchHandler(t *testing.T, engine *codesearch.Engine) http.Handler {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc(searchAPIPath, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeAPIError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		query := r.URL.Query().Get("q")
		if err := requireQuery(query); err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		mode, err := normalizeMode(r.URL.Query().Get("mode"))
		if err != nil {
			writeAPIError(w, http.StatusBadRequest, err.Error())
			return
		}
		limit := parseLimit(r.URL.Query().Get("limit"), defaultSearchLimit)
		results, err := engine.Search(r.Context(), query, codesearch.WithLimit(limit), codesearch.WithMode(mode))
		if err != nil {
			writeAPIError(w, http.StatusInternalServerError, err.Error())
			return
		}
		if err := writeJSON(w, http.StatusOK, buildSearchResponse(query, limit, mode, "remote", results)); err != nil {
			t.Fatalf("write json response: %v", err)
		}
	})
	return mux
}

func assertAPIError(t *testing.T, response *http.Response, wantStatus int, wantError string) {
	t.Helper()

	if response.StatusCode != wantStatus {
		t.Fatalf("unexpected status code: got %d want %d", response.StatusCode, wantStatus)
	}

	var payload map[string]string
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode error payload: %v", err)
	}
	if payload["error"] != wantError {
		t.Fatalf("unexpected error message: got %q want %q", payload["error"], wantError)
	}
}

func writeTestFile(t *testing.T, rootDir, relativePath, content string) string {
	t.Helper()

	path := filepath.Join(rootDir, relativePath)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file %s: %v", path, err)
	}
	return path
}

func writeBinaryFile(t *testing.T, path string, content []byte) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write binary file %s: %v", path, err)
	}
}

func stripANSI(value string) string {
	return regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(value, "")
}
