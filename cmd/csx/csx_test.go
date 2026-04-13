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

	"github.com/SCKelemen/clix"
	"github.com/SCKelemen/codesearch"
	codesearchpb "github.com/SCKelemen/codesearch/gen/codesearch/v1"
	codesearchv1connect "github.com/SCKelemen/codesearch/gen/codesearch/v1/codesearchv1connect"
	"github.com/SCKelemen/codesearch/lsp/lsifgen"
	"github.com/SCKelemen/codesearch/proto/codesearchv1"
)

type promptFunc func(context.Context, ...clix.PromptOption) (string, error)

func (f promptFunc) Prompt(ctx context.Context, opts ...clix.PromptOption) (string, error) {
	return f(ctx, opts...)
}

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
	defer func() { _ = engine.Close() }()

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

	var payload searchResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode search response: %v", err)
	}
	if payload.Mode != "lexical" {
		t.Fatalf("payload.Mode = %q, want %q", payload.Mode, "lexical")
	}
	if len(payload.Results) != 1 {
		t.Fatalf("len(payload.Results) = %d, want 1", len(payload.Results))
	}
}

func TestSearchAPIHandlerErrors(t *testing.T) {
	t.Parallel()

	engine := codesearch.New(codesearch.WithMemoryStore(), codesearch.WithEmbedder(deterministicEmbedder{dimensions: semanticDimensions}))
	defer func() { _ = engine.Close() }()

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

func TestSearchModeFlag(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	rootDir := t.TempDir()
	indexDir := filepath.Join(t.TempDir(), "index")
	filePath := writeTestFile(t, rootDir, "main.go", "package main\n\nfunc FlagSearchTarget() string {\n\treturn \"needle\"\n}\n")

	engine, err := openEngine(indexDir)
	if err != nil {
		t.Fatalf("open engine: %v", err)
	}
	if err := engine.Index(ctx, filePath); err != nil {
		t.Fatalf("index file: %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatalf("close engine: %v", err)
	}

	app := newApp()
	var out bytes.Buffer
	app.Out = &out
	app.Err = &bytes.Buffer{}

	if err := app.Run(ctx, []string{"search", "FlagSearchTarget", "--index", indexDir, "--mode", "lexical", "--json"}); err != nil {
		t.Fatalf("app.Run() error = %v", err)
	}

	var payload searchResponse
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal() error = %v; output = %s", err, out.String())
	}
	if payload.Mode != "lexical" {
		t.Fatalf("payload.Mode = %q, want %q", payload.Mode, "lexical")
	}
	if payload.Query != "FlagSearchTarget" {
		t.Fatalf("payload.Query = %q, want %q", payload.Query, "FlagSearchTarget")
	}
	if len(payload.Results) != 1 {
		t.Fatalf("len(payload.Results) = %d, want 1", len(payload.Results))
	}
}

func TestLSPSetup_Disabled(t *testing.T) {
	t.Parallel()

	if mux := setupLSP(context.Background(), t.TempDir(), false); mux != nil {
		t.Fatalf("setupLSP() = %#v, want nil", mux)
	}
}

func TestLSIFCommand_NoArgs(t *testing.T) {
	t.Parallel()

	app := newApp()
	app.Out = &bytes.Buffer{}
	app.Err = &bytes.Buffer{}

	prompted := false
	missingPath := filepath.Join(t.TempDir(), "missing.go")
	app.Prompter = promptFunc(func(context.Context, ...clix.PromptOption) (string, error) {
		prompted = true
		return missingPath, nil
	})

	err := app.Run(context.Background(), []string{"lsif"})
	if err == nil {
		t.Fatal("app.Run() error = nil, want error")
	}
	if !prompted {
		t.Fatal("expected prompter to be called for the missing path argument")
	}
	if !strings.Contains(err.Error(), missingPath) {
		t.Fatalf("error = %q, want to contain %q", err.Error(), missingPath)
	}
}

func TestIndexCommandLogic(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	rootDir := t.TempDir()
	outputDir := filepath.Join(rootDir, "index")
	goFileOne := writeTestFile(t, rootDir, filepath.Join("pkg", "alpha.go"), "package pkg\n\nfunc AlphaSearchToken() string { return \"alpha\" }\n")
	writeTestFile(t, rootDir, filepath.Join("pkg", "beta.go"), "package pkg\n\nfunc BetaSearchToken() string { return \"beta\" }\n")
	writeTestFile(t, rootDir, "README.md", "AlphaSearchToken should not be indexed by the Go-only filter.\n")
	writeBinaryFile(t, filepath.Join(rootDir, "pkg", "binary.dat"), []byte{0x00, 0x01, 0x02, 0x03})
	writeTestFile(t, outputDir, "stale.txt", "this file lives inside the output directory and must be skipped\n")

	inputs, err := collectIndexInputs(rootDir, outputDir, parseLanguageFilter("go"))
	if err != nil {
		t.Fatalf("collect index inputs: %v", err)
	}
	if len(inputs) != 2 {
		t.Fatalf("len(inputs) = %d, want 2", len(inputs))
	}

	engine, err := openEngine(outputDir)
	if err != nil {
		t.Fatalf("open engine: %v", err)
	}
	defer func() { _ = engine.Close() }()

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
		t.Fatalf("result.Path = %q, want %q", results[0].Path, filepath.ToSlash(filepath.Clean(goFileOne)))
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
	defer func() { _ = statusEngine.Close() }()

	stats, err := collectIndexStats(ctx, statusEngine, indexDir)
	if err != nil {
		t.Fatalf("collect index stats: %v", err)
	}
	if stats.FileCount != 2 {
		t.Fatalf("stats.FileCount = %d, want 2", stats.FileCount)
	}

	var output bytes.Buffer
	if err := renderStatus(&output, indexDir, stats); err != nil {
		t.Fatalf("render status: %v", err)
	}
	cleanOutput := stripANSI(output.String())
	for _, want := range []string{"Index status", "files:", "embeddings:", "Languages"} {
		if !strings.Contains(cleanOutput, want) {
			t.Fatalf("status output %q does not contain %q", cleanOutput, want)
		}
	}
}

func TestRemoteSearchHelpers(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	engine := codesearch.New(codesearch.WithMemoryStore())
	if err := engine.IndexFile(ctx, "remote.go", []byte("package main\n\nfunc RemoteNeedle() {}\n")); err != nil {
		t.Fatalf("IndexFile() error = %v", err)
	}

	service := codesearchv1.NewService(engine)
	connectPath, connectHandler := codesearchv1connect.NewCodeSearchServiceHandler(service)
	connectMux := http.NewServeMux()
	connectMux.Handle(connectPath, connectHandler)
	connectServer := httptest.NewServer(connectMux)
	defer connectServer.Close()

	connectResponse, err := httpConnectSearch(ctx, connectServer.Client(), connectServer.URL, searchRequest{Query: "RemoteNeedle", Mode: "lexical", Limit: 3})
	if err != nil {
		t.Fatalf("httpConnectSearch() error = %v", err)
	}
	if connectResponse.Source != "remote" {
		t.Fatalf("connectResponse.Source = %q, want %q", connectResponse.Source, "remote")
	}
	if len(connectResponse.Results) != 1 {
		t.Fatalf("len(connectResponse.Results) = %d, want 1", len(connectResponse.Results))
	}

	jsonServer := httptest.NewServer(newSearchHandler(t, engine))
	defer jsonServer.Close()

	jsonResponse, err := httpSearch(ctx, jsonServer.Client(), jsonServer.URL, searchRequest{Query: "RemoteNeedle", Mode: "lexical", Limit: 3})
	if err != nil {
		t.Fatalf("httpSearch() error = %v", err)
	}
	if len(jsonResponse.Results) != 1 {
		t.Fatalf("len(jsonResponse.Results) = %d, want 1", len(jsonResponse.Results))
	}
}

func TestRenderAndConversionHelpers(t *testing.T) {
	t.Parallel()

	response := searchResponse{Query: "needle", Limit: 2, Mode: "lexical", Source: "local", Results: []searchResult{{Path: "/tmp/project/main.go", Line: 3, Score: 1.25, Snippet: "const needle = true", Matches: []matchRange{{Start: 6, End: 12}}}}}

	var jsonOutput bytes.Buffer
	if err := renderSearchJSON(&jsonOutput, response); err != nil {
		t.Fatalf("renderSearchJSON() error = %v", err)
	}
	var decoded searchResponse
	if err := json.Unmarshal(jsonOutput.Bytes(), &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if decoded.Query != response.Query || len(decoded.Results) != 1 {
		t.Fatalf("decoded response = %#v, want %#v", decoded, response)
	}

	var textOutput bytes.Buffer
	if err := renderSearchText(&textOutput, response); err != nil {
		t.Fatalf("renderSearchText() error = %v", err)
	}
	clean := stripANSI(textOutput.String())
	for _, want := range []string{"Search results", "needle", "local", "main.go"} {
		if !strings.Contains(clean, want) {
			t.Fatalf("rendered text %q does not contain %q", clean, want)
		}
	}

	var emptyOutput bytes.Buffer
	if err := renderSearchText(&emptyOutput, searchResponse{Query: "missing", Mode: "lexical"}); err != nil {
		t.Fatalf("renderSearchText(empty) error = %v", err)
	}
	if !strings.Contains(stripANSI(emptyOutput.String()), "no matches found") {
		t.Fatalf("empty output = %q, want no matches message", stripANSI(emptyOutput.String()))
	}

	ui := newCLIUI(&bytes.Buffer{})
	highlighted := stripANSI(highlightSnippet(ui, "const needle = true", []matchRange{{Start: 6, End: 12}}))
	if !strings.Contains(highlighted, "needle") {
		t.Fatalf("highlightSnippet() = %q, want to contain %q", highlighted, "needle")
	}
	if clamp(5, 0, 3) != 3 {
		t.Fatalf("clamp() = %d, want 3", clamp(5, 0, 3))
	}

	protoResponse := &codesearchpb.SearchResponse{Query: "remote", Limit: 1, Mode: "lexical", Results: []*codesearchpb.SearchResult{{Path: "remote.go", Line: 2, Score: 1, Snippet: "func remote()", Matches: []*codesearchpb.MatchRange{{Start: 5, End: 11}}}}}
	converted := searchResponseFromProto(protoResponse)
	if converted.Source != "remote" || len(converted.Results) != 1 {
		t.Fatalf("searchResponseFromProto() = %#v", converted)
	}

	if got := normalizeAddress("127.0.0.1:8080"); got != "http://127.0.0.1:8080" {
		t.Fatalf("normalizeAddress() = %q, want %q", got, "http://127.0.0.1:8080")
	}
	if got := readRemoteError(strings.NewReader(`{"error":"bad request"}`)); got != "bad request" {
		t.Fatalf("readRemoteError(JSON) = %q, want %q", got, "bad request")
	}
	if got := readRemoteError(strings.NewReader("plain failure")); got != "plain failure" {
		t.Fatalf("readRemoteError(text) = %q, want %q", got, "plain failure")
	}
}

func TestLSIFHelpers(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	goFile := writeTestFile(t, rootDir, "main.go", "package main\n\nfunc main() {}\n")
	writeTestFile(t, rootDir, "notes.txt", "ignored\n")

	inputs, err := collectLSIFInputs(rootDir)
	if err != nil {
		t.Fatalf("collectLSIFInputs() error = %v", err)
	}
	if len(inputs) != 1 || inputs[0] != goFile {
		t.Fatalf("collectLSIFInputs() = %v, want [%q]", inputs, goFile)
	}

	sources, err := loadSources(inputs)
	if err != nil {
		t.Fatalf("loadSources() error = %v", err)
	}
	if !strings.Contains(sources[goFile], "package main") {
		t.Fatalf("loadSources() content = %q, want Go source", sources[goFile])
	}
	if got := formatLSIFStats(nil); got != "LSIF: indexed 0 documents, 0 symbols, 0 references" {
		t.Fatalf("formatLSIFStats(nil) = %q", got)
	}
	if got := formatLSIFStats(&lsifgen.Stats{Documents: 1, Symbols: 2, References: 3}); got != "LSIF: indexed 1 documents, 2 symbols, 3 references" {
		t.Fatalf("formatLSIFStats(stats) = %q", got)
	}
}

func TestDeterministicEmbedderDefaults(t *testing.T) {
	t.Parallel()

	var embedder deterministicEmbedder
	if embedder.Dimensions() != semanticDimensions {
		t.Fatalf("Dimensions() = %d, want %d", embedder.Dimensions(), semanticDimensions)
	}
	if embedder.Model() == "" {
		t.Fatal("Model() returned an empty string")
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
		if err := writeJSON(w, http.StatusOK, buildSearchResponse(query, limit, mode, "remote", "", results)); err != nil {
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
