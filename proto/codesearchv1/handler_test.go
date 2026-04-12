package codesearchv1

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SCKelemen/codesearch"
)

type testEmbedder struct{}

func (testEmbedder) Embed(_ context.Context, inputs []string) ([][]float32, error) {
	vectors := make([][]float32, len(inputs))
	for i, input := range inputs {
		vectors[i] = []float32{float32(len(input)), float32(i + 1)}
	}
	return vectors, nil
}

func (testEmbedder) Dimensions() int { return 2 }
func (testEmbedder) Model() string   { return "test" }

func TestSearchHandler(t *testing.T) {
	t.Parallel()

	engine := codesearch.New(codesearch.WithMemoryStore())
	indexTestFiles(t, engine, map[string]string{
		"src/main.go":   "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n",
		"src/app.ts":    "export function greet(): string {\n\treturn hi\n}\n",
		"src/module.py": "def greet():\n    return hi\n",
	})

	handler := NewCodeSearchHandler(engine)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, SearchProcedurePath, strings.NewReader(`{"query":"func","limit":10}`))
	request.Header.Set("Content-Type", "application/json")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response SearchResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}
	if response.Query != "func" {
		t.Fatalf("response.Query = %q, want %q", response.Query, "func")
	}
	if response.Limit != 10 {
		t.Fatalf("response.Limit = %d, want 10", response.Limit)
	}
	if response.Mode != "hybrid" {
		t.Fatalf("response.Mode = %q, want %q", response.Mode, "hybrid")
	}
	if len(response.Results) < 2 {
		t.Fatalf("len(response.Results) = %d, want at least 2", len(response.Results))
	}

	paths := make(map[string]SearchResult, len(response.Results))
	for _, result := range response.Results {
		paths[result.Path] = result
	}

	goResult, ok := findResultBySuffix(response.Results, "src/main.go")
	if !ok {
		t.Fatalf("expected Go result in %#v", response.Results)
	}
	if goResult.Line != 3 {
		t.Fatalf("Go result line = %d, want 3", goResult.Line)
	}
	if !strings.Contains(strings.ToLower(goResult.Snippet), "func") {
		t.Fatalf("Go snippet = %q, want to contain %q", goResult.Snippet, "func")
	}
	if len(goResult.Matches) == 0 {
		t.Fatalf("Go result matches = %#v, want non-empty", goResult.Matches)
	}

	tsResult, ok := findResultBySuffix(response.Results, "src/app.ts")
	if !ok {
		t.Fatalf("expected TypeScript result in %#v", response.Results)
	}
	if !strings.Contains(strings.ToLower(tsResult.Snippet), "function") {
		t.Fatalf("TypeScript snippet = %q, want to contain %q", tsResult.Snippet, "function")
	}

	if _, ok := findResultBySuffix(response.Results, "src/module.py"); ok {
		t.Fatalf("did not expect Python result for query %q in %#v", "func", response.Results)
	}
}

func TestSearchHandlerErrors(t *testing.T) {
	t.Parallel()

	engine := codesearch.New(codesearch.WithMemoryStore())
	indexTestFiles(t, engine, map[string]string{
		"src/main.go": "package main\nfunc main() {}\n",
	})

	handler := NewCodeSearchHandler(engine)
	tests := []struct {
		name       string
		method     string
		path       string
		body       string
		wantStatus int
		wantError  string
	}{
		{
			name:       "method not allowed",
			method:     http.MethodGet,
			path:       SearchProcedurePath,
			wantStatus: http.StatusMethodNotAllowed,
			wantError:  "method not allowed",
		},
		{
			name:       "empty query",
			method:     http.MethodPost,
			path:       SearchProcedurePath,
			body:       `{}`,
			wantStatus: http.StatusBadRequest,
			wantError:  "query must not be empty",
		},
		{
			name:       "invalid mode",
			method:     http.MethodPost,
			path:       SearchProcedurePath,
			body:       `{"query":"func","mode":"invalid"}`,
			wantStatus: http.StatusBadRequest,
			wantError:  `unknown search mode "invalid"`,
		},
		{
			name:       "invalid json",
			method:     http.MethodPost,
			path:       SearchProcedurePath,
			body:       `{"query":`,
			wantStatus: http.StatusBadRequest,
			wantError:  "decode json body",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			recorder := httptest.NewRecorder()
			var body *strings.Reader
			if test.body == "" {
				body = strings.NewReader("")
			} else {
				body = strings.NewReader(test.body)
			}
			request := httptest.NewRequest(test.method, test.path, body)
			request.Header.Set("Content-Type", "application/json")

			handler.ServeHTTP(recorder, request)

			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, test.wantStatus, recorder.Body.String())
			}
			assertErrorResponseContains(t, recorder.Body.Bytes(), test.wantError)
		})
	}
}

func TestIndexStatusHandler(t *testing.T) {
	t.Parallel()

	engine := codesearch.New(
		codesearch.WithMemoryStore(),
		codesearch.WithEmbedder(testEmbedder{}),
	)
	files := map[string]string{
		"src/main.go":   "package main\n\nfunc main() {}\n",
		"src/app.ts":    "export function greet(): string {\n\treturn hi\n}\n",
		"src/module.py": "def greet():\n    return hi\n",
	}
	indexTestFiles(t, engine, files)

	handler := NewCodeSearchHandler(engine)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, IndexStatusProcedurePath, bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response IndexStatusResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	if response.FileCount != int32(len(files)) {
		t.Fatalf("response.FileCount = %d, want %d", response.FileCount, len(files))
	}
	if response.EmbeddingCount != int32(len(files)) {
		t.Fatalf("response.EmbeddingCount = %d, want %d", response.EmbeddingCount, len(files))
	}
	if len(response.Languages) != len(files) {
		t.Fatalf("len(response.Languages) = %d, want %d; map = %#v", len(response.Languages), len(files), response.Languages)
	}

	for path := range files {
		document, err := engine.Documents.Lookup(context.Background(), path)
		if err != nil {
			t.Fatalf("Lookup(%q) returned error: %v", path, err)
		}
		if document == nil {
			t.Fatalf("Lookup(%q) returned nil document", path)
		}
		if response.Languages[document.Language] != 1 {
			t.Fatalf("response.Languages[%q] = %d, want 1; map = %#v", document.Language, response.Languages[document.Language], response.Languages)
		}
	}
	if response.TotalBytes <= 0 {
		t.Fatalf("response.TotalBytes = %d, want > 0", response.TotalBytes)
	}
	if response.IndexBytes != response.TotalBytes {
		t.Fatalf("response.IndexBytes = %d, want %d", response.IndexBytes, response.TotalBytes)
	}
}

func TestSearchHandlerFilter(t *testing.T) {
	t.Parallel()

	engine := codesearch.New(codesearch.WithMemoryStore())
	indexTestFiles(t, engine, map[string]string{
		"src/main.go": "package main\n\nfunc main() {}\n",
		"src/app.ts":  "export function greet(): string {\n\treturn hi\n}\n",
	})

	handler := NewCodeSearchHandler(engine)

	goResponse := performSearchRequest(t, handler, SearchRequest{
		Query:  "func",
		Limit:  10,
		Filter: "go",
	})
	if len(goResponse.Results) == 0 {
		t.Fatal("expected Go-filtered results, got none")
	}
	for _, result := range goResponse.Results {
		if filepath.Ext(result.Path) != ".go" {
			t.Fatalf("Go filter returned non-Go path %q in %#v", result.Path, goResponse.Results)
		}
	}

	tsResponse := performSearchRequest(t, handler, SearchRequest{
		Query:  "func",
		Limit:  10,
		Filter: "typescript",
	})
	if len(tsResponse.Results) == 0 {
		t.Fatal("expected TypeScript-filtered results, got none")
	}
	for _, result := range tsResponse.Results {
		if filepath.Ext(result.Path) != ".ts" {
			t.Fatalf("TypeScript filter returned non-TypeScript path %q in %#v", result.Path, tsResponse.Results)
		}
	}
}

func TestNotFoundPath(t *testing.T) {
	t.Parallel()

	handler := NewCodeSearchHandler(codesearch.New(codesearch.WithMemoryStore()))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/unknown", bytes.NewBufferString(`{}`))
	request.Header.Set("Content-Type", "application/json")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusNotFound, recorder.Body.String())
	}
	assertErrorResponseContains(t, recorder.Body.Bytes(), "not found")
}

func indexTestFiles(t *testing.T, engine *codesearch.Engine, files map[string]string) {
	t.Helper()

	for path, content := range files {
		if err := engine.IndexFile(context.Background(), path, []byte(content)); err != nil {
			t.Fatalf("IndexFile(%q) returned error: %v", path, err)
		}
	}
}

func performSearchRequest(t *testing.T, handler *CodeSearchHandler, requestBody SearchRequest) SearchResponse {
	t.Helper()

	payload, err := json.Marshal(requestBody)
	if err != nil {
		t.Fatalf("json.Marshal returned error: %v", err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, SearchProcedurePath, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
	}

	var response SearchResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("json.Unmarshal returned error: %v", err)
	}
	return response
}

func findResultBySuffix(results []SearchResult, suffix string) (SearchResult, bool) {
	for _, result := range results {
		if strings.HasSuffix(result.Path, suffix) {
			return result, true
		}
	}
	return SearchResult{}, false
}

func assertErrorResponseContains(t *testing.T, body []byte, want string) {
	t.Helper()

	var response map[string]string
	if err := json.Unmarshal(body, &response); err != nil {
		t.Fatalf("response is not valid JSON: %v; body = %s", err, string(body))
	}
	if !strings.Contains(response["error"], want) {
		t.Fatalf("error = %q, want to contain %q", response["error"], want)
	}
}
