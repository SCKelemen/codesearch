package codesearchv1

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/SCKelemen/codesearch"
	codesearchpb "github.com/SCKelemen/codesearch/gen/codesearch/v1"
	codesearchv1connect "github.com/SCKelemen/codesearch/gen/codesearch/v1/codesearchv1connect"
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

	client, _ := newTestClient(t, engine)
	response := performSearchRequest(t, client, &codesearchpb.SearchRequest{Query: "func", Limit: 10})

	if response.GetQuery() != "func" {
		t.Fatalf("response.Query = %q, want %q", response.GetQuery(), "func")
	}
	if response.GetLimit() != 10 {
		t.Fatalf("response.Limit = %d, want 10", response.GetLimit())
	}
	if response.GetMode() != "hybrid" {
		t.Fatalf("response.Mode = %q, want %q", response.GetMode(), "hybrid")
	}
	if len(response.GetResults()) < 2 {
		t.Fatalf("len(response.Results) = %d, want at least 2", len(response.GetResults()))
	}

	goResult, ok := findResultBySuffix(response.GetResults(), "src/main.go")
	if !ok {
		t.Fatalf("expected Go result in %#v", response.GetResults())
	}
	if goResult.GetLine() != 3 {
		t.Fatalf("Go result line = %d, want 3", goResult.GetLine())
	}
	if !strings.Contains(strings.ToLower(goResult.GetSnippet()), "func") {
		t.Fatalf("Go snippet = %q, want to contain %q", goResult.GetSnippet(), "func")
	}
	if len(goResult.GetMatches()) == 0 {
		t.Fatalf("Go result matches = %#v, want non-empty", goResult.GetMatches())
	}

	tsResult, ok := findResultBySuffix(response.GetResults(), "src/app.ts")
	if !ok {
		t.Fatalf("expected TypeScript result in %#v", response.GetResults())
	}
	if !strings.Contains(strings.ToLower(tsResult.GetSnippet()), "function") {
		t.Fatalf("TypeScript snippet = %q, want to contain %q", tsResult.GetSnippet(), "function")
	}

	if _, ok := findResultBySuffix(response.GetResults(), "src/module.py"); ok {
		t.Fatalf("did not expect Python result for query %q in %#v", "func", response.GetResults())
	}
}

func TestSearchHandlerErrors(t *testing.T) {
	t.Parallel()

	engine := codesearch.New(codesearch.WithMemoryStore())
	indexTestFiles(t, engine, map[string]string{
		"src/main.go": "package main\nfunc main() {}\n",
	})

	client, _ := newTestClient(t, engine)
	tests := []struct {
		name      string
		request   *codesearchpb.SearchRequest
		wantCode  connect.Code
		wantError string
	}{
		{
			name:      "empty query",
			request:   &codesearchpb.SearchRequest{},
			wantCode:  connect.CodeInvalidArgument,
			wantError: "query must not be empty",
		},
		{
			name:      "invalid mode",
			request:   &codesearchpb.SearchRequest{Query: "func", Mode: "invalid"},
			wantCode:  connect.CodeInvalidArgument,
			wantError: `unknown search mode "invalid"`,
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			_, err := client.Search(context.Background(), connect.NewRequest(test.request))
			assertConnectErrorContains(t, err, test.wantCode, test.wantError)
		})
	}
}

func TestSearchHandlerRejectsMalformedJSON(t *testing.T) {
	t.Parallel()

	engine := codesearch.New(codesearch.WithMemoryStore())
	_, server := newTestClient(t, engine)

	request, err := http.NewRequest(http.MethodPost, server.URL+codesearchv1connect.CodeSearchServiceSearchProcedure, strings.NewReader(`{"query":`))
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Connect-Protocol-Version", "1")

	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusBadRequest, string(body))
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

	client, _ := newTestClient(t, engine)
	response, err := client.IndexStatus(context.Background(), connect.NewRequest(&codesearchpb.IndexStatusRequest{}))
	if err != nil {
		t.Fatalf("IndexStatus() error = %v", err)
	}

	if response.Msg.GetFileCount() != int32(len(files)) {
		t.Fatalf("response.FileCount = %d, want %d", response.Msg.GetFileCount(), len(files))
	}
	if response.Msg.GetEmbeddingCount() != int32(len(files)) {
		t.Fatalf("response.EmbeddingCount = %d, want %d", response.Msg.GetEmbeddingCount(), len(files))
	}
	if len(response.Msg.GetLanguages()) != len(files) {
		t.Fatalf("len(response.Languages) = %d, want %d; map = %#v", len(response.Msg.GetLanguages()), len(files), response.Msg.GetLanguages())
	}

	for path := range files {
		document, err := engine.Documents.Lookup(context.Background(), path)
		if err != nil {
			t.Fatalf("Lookup(%q) returned error: %v", path, err)
		}
		if document == nil {
			t.Fatalf("Lookup(%q) returned nil document", path)
		}
		if response.Msg.GetLanguages()[document.Language] != 1 {
			t.Fatalf("response.Languages[%q] = %d, want 1; map = %#v", document.Language, response.Msg.GetLanguages()[document.Language], response.Msg.GetLanguages())
		}
	}
	if response.Msg.GetTotalBytes() <= 0 {
		t.Fatalf("response.TotalBytes = %d, want > 0", response.Msg.GetTotalBytes())
	}
	if response.Msg.GetIndexBytes() != response.Msg.GetTotalBytes() {
		t.Fatalf("response.IndexBytes = %d, want %d", response.Msg.GetIndexBytes(), response.Msg.GetTotalBytes())
	}
}

func TestSearchHandlerFilter(t *testing.T) {
	t.Parallel()

	engine := codesearch.New(codesearch.WithMemoryStore())
	indexTestFiles(t, engine, map[string]string{
		"src/main.go": "package main\n\nfunc main() {}\n",
		"src/app.ts":  "export function greet(): string {\n\treturn hi\n}\n",
	})

	client, _ := newTestClient(t, engine)

	goResponse := performSearchRequest(t, client, &codesearchpb.SearchRequest{
		Query:  "func",
		Limit:  10,
		Filter: `file_extension == ".go"`,
	})
	if len(goResponse.GetResults()) == 0 {
		t.Fatal("expected Go-filtered results, got none")
	}
	for _, result := range goResponse.GetResults() {
		if filepath.Ext(result.GetPath()) != ".go" {
			t.Fatalf("Go filter returned non-Go path %q in %#v", result.GetPath(), goResponse.GetResults())
		}
	}

	tsResponse := performSearchRequest(t, client, &codesearchpb.SearchRequest{
		Query:  "func",
		Limit:  10,
		Filter: `file_extension == ".ts"`,
	})
	if len(tsResponse.GetResults()) == 0 {
		t.Fatal("expected TypeScript-filtered results, got none")
	}
	for _, result := range tsResponse.GetResults() {
		if filepath.Ext(result.GetPath()) != ".ts" {
			t.Fatalf("TypeScript filter returned non-TypeScript path %q in %#v", result.GetPath(), tsResponse.GetResults())
		}
	}
}

func TestSearchSymbolsHandler(t *testing.T) {
	t.Parallel()

	engine := codesearch.New(codesearch.WithMemoryStore())
	indexTestFiles(t, engine, map[string]string{
		"src/main.go":   "package main\n\nfunc main() {}\nfunc helper() {}\n",
		"src/widget.ts": "export class Widget {}\nexport function greet(): string {\n\treturn hi\n}\n",
	})

	client, _ := newTestClient(t, engine)
	response, err := client.SearchSymbols(context.Background(), connect.NewRequest(&codesearchpb.SearchSymbolsRequest{
		Name:     "main",
		Kind:     "function",
		Language: "go",
		Limit:    10,
	}))
	if err != nil {
		t.Fatalf("SearchSymbols() error = %v", err)
	}

	if len(response.Msg.GetResults()) != 1 {
		t.Fatalf("len(response.Results) = %d, want 1", len(response.Msg.GetResults()))
	}
	result := response.Msg.GetResults()[0]
	if result.GetName() != "main" {
		t.Fatalf("result.Name = %q, want %q", result.GetName(), "main")
	}
	if result.GetKind() != "function" {
		t.Fatalf("result.Kind = %q, want %q", result.GetKind(), "function")
	}
	if !strings.HasSuffix(result.GetPath(), "src/main.go") {
		t.Fatalf("result.Path = %q, want suffix %q", result.GetPath(), "src/main.go")
	}
	if result.GetRange().GetStartLine() <= 0 {
		t.Fatalf("result.Range.StartLine = %d, want > 0", result.GetRange().GetStartLine())
	}
}

func TestNotFoundPath(t *testing.T) {
	t.Parallel()

	engine := codesearch.New(codesearch.WithMemoryStore())
	_, server := newTestClient(t, engine)

	response, err := server.Client().Post(server.URL+"/unknown", "application/json", bytes.NewBufferString(`{}`))
	if err != nil {
		t.Fatalf("Post() error = %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, http.StatusNotFound, string(body))
	}
}

func indexTestFiles(t *testing.T, engine *codesearch.Engine, files map[string]string) {
	t.Helper()

	for path, content := range files {
		if err := engine.IndexFile(context.Background(), path, []byte(content)); err != nil {
			t.Fatalf("IndexFile(%q) returned error: %v", path, err)
		}
	}
}

func newTestClient(t *testing.T, engine *codesearch.Engine) (codesearchv1connect.CodeSearchServiceClient, *httptest.Server) {
	t.Helper()

	service := NewService(engine)
	connectPath, handler := codesearchv1connect.NewCodeSearchServiceHandler(service)
	mux := http.NewServeMux()
	mux.Handle(connectPath, handler)

	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)

	client := codesearchv1connect.NewCodeSearchServiceClient(server.Client(), server.URL)
	return client, server
}

func performSearchRequest(t *testing.T, client codesearchv1connect.CodeSearchServiceClient, requestBody *codesearchpb.SearchRequest) *codesearchpb.SearchResponse {
	t.Helper()

	response, err := client.Search(context.Background(), connect.NewRequest(requestBody))
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	return response.Msg
}

func findResultBySuffix(results []*codesearchpb.SearchResult, suffix string) (*codesearchpb.SearchResult, bool) {
	for _, result := range results {
		if strings.HasSuffix(result.GetPath(), suffix) {
			return result, true
		}
	}
	return nil, false
}

func assertConnectErrorContains(t *testing.T, err error, wantCode connect.Code, want string) {
	t.Helper()

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	var connectErr *connect.Error
	if !errors.As(err, &connectErr) {
		t.Fatalf("error = %T, want *connect.Error: %v", err, err)
	}
	if connectErr.Code() != wantCode {
		t.Fatalf("error code = %v, want %v", connectErr.Code(), wantCode)
	}
	if !strings.Contains(connectErr.Message(), want) {
		t.Fatalf("error message = %q, want to contain %q", connectErr.Message(), want)
	}
}
