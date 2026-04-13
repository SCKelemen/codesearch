package codesearchv1

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
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
		lower := strings.ToLower(input)
		switch {
		case strings.Contains(lower, "semantic"), strings.Contains(lower, "approx"), strings.Contains(lower, "vector"):
			vectors[i] = []float32{1, 0}
		default:
			vectors[i] = []float32{0, 1}
		}
	}
	return vectors, nil
}

func (testEmbedder) Dimensions() int { return 2 }
func (testEmbedder) Model() string   { return "test" }

func TestSearch_EmptyQuery(t *testing.T) {
	t.Parallel()

	engine := codesearch.New(codesearch.WithMemoryStore())
	client, _ := newTestClient(t, engine)

	_, err := client.Search(context.Background(), connect.NewRequest(&codesearchpb.SearchRequest{}))
	assertConnectErrorContains(t, err, connect.CodeInvalidArgument, "query must not be empty")
}

func TestSearch_Integration(t *testing.T) {
	t.Parallel()

	engine := codesearch.New(codesearch.WithMemoryStore())
	indexTestFiles(t, engine, map[string]string{
		"src/main.go":   "package main\n\nfunc SearchTarget() string {\n\treturn \"needle\"\n}\n",
		"src/helper.ts": "export function helper(): string {\n\treturn 'value'\n}\n",
	})

	client, _ := newTestClient(t, engine)
	response := performSearchRequest(t, client, &codesearchpb.SearchRequest{Query: "SearchTarget", Limit: 5})

	if response.GetQuery() != "SearchTarget" {
		t.Fatalf("response.Query = %q, want %q", response.GetQuery(), "SearchTarget")
	}
	if response.GetMode() != "hybrid" {
		t.Fatalf("response.Mode = %q, want %q", response.GetMode(), "hybrid")
	}
	if len(response.GetResults()) != 1 {
		t.Fatalf("len(response.Results) = %d, want 1", len(response.GetResults()))
	}
	result := response.GetResults()[0]
	if !strings.HasSuffix(result.GetPath(), "src/main.go") {
		t.Fatalf("result.Path = %q, want suffix %q", result.GetPath(), "src/main.go")
	}
	if result.GetLine() != 3 {
		t.Fatalf("result.Line = %d, want 3", result.GetLine())
	}
	if !strings.Contains(result.GetSnippet(), "SearchTarget") {
		t.Fatalf("result.Snippet = %q, want to contain %q", result.GetSnippet(), "SearchTarget")
	}
}

func TestSearch_LanguageFilter(t *testing.T) {
	t.Parallel()

	engine := codesearch.New(codesearch.WithMemoryStore())
	indexTestFiles(t, engine, map[string]string{
		"src/main.go":   "package main\nconst shared = \"needle\"\n",
		"src/module.py": "shared = 'needle'\n",
	})

	client, _ := newTestClient(t, engine)
	response := performSearchRequest(t, client, &codesearchpb.SearchRequest{
		Query:  "needle",
		Limit:  10,
		Filter: `language == "Go"`,
	})

	if len(response.GetResults()) != 1 {
		t.Fatalf("len(response.Results) = %d, want 1", len(response.GetResults()))
	}
	if !strings.HasSuffix(response.GetResults()[0].GetPath(), "src/main.go") {
		t.Fatalf("result.Path = %q, want suffix %q", response.GetResults()[0].GetPath(), "src/main.go")
	}
}

func TestSearch_FuzzyMode(t *testing.T) {
	t.Parallel()

	engine := codesearch.New(codesearch.WithMemoryStore(), codesearch.WithEmbedder(testEmbedder{}))
	indexTestFiles(t, engine, map[string]string{
		"src/semantic.txt": "semantic vector content",
		"src/noise.txt":    "ordinary unrelated text",
	})

	client, _ := newTestClient(t, engine)
	response := performSearchRequest(t, client, &codesearchpb.SearchRequest{Query: "approximate concept", Mode: "hybrid", Limit: 5})

	if len(response.GetResults()) == 0 {
		t.Fatal("expected semantic result, got none")
	}
	if !strings.HasSuffix(response.GetResults()[0].GetPath(), "src/semantic.txt") {
		t.Fatalf("result.Path = %q, want suffix %q", response.GetResults()[0].GetPath(), "src/semantic.txt")
	}
}

func TestSearch_ExactMode(t *testing.T) {
	t.Parallel()

	engine := codesearch.New(codesearch.WithMemoryStore())
	indexTestFiles(t, engine, map[string]string{"src/main.go": "package main\nconst exactValue = \"ExactValue\"\n"})

	client, _ := newTestClient(t, engine)
	response := performSearchRequest(t, client, &codesearchpb.SearchRequest{Query: "ExactValue", Mode: "lexical", Limit: 5})

	if response.GetMode() != "lexical" {
		t.Fatalf("response.Mode = %q, want %q", response.GetMode(), "lexical")
	}
	if len(response.GetResults()) != 1 {
		t.Fatalf("len(response.Results) = %d, want 1", len(response.GetResults()))
	}
}

func TestIndexStatus_Empty(t *testing.T) {
	t.Parallel()

	engine := codesearch.New(codesearch.WithMemoryStore())
	client, _ := newTestClient(t, engine)

	response, err := client.IndexStatus(context.Background(), connect.NewRequest(&codesearchpb.IndexStatusRequest{}))
	if err != nil {
		t.Fatalf("IndexStatus() error = %v", err)
	}
	if response.Msg.GetFileCount() != 0 {
		t.Fatalf("response.FileCount = %d, want 0", response.Msg.GetFileCount())
	}
	if response.Msg.GetEmbeddingCount() != 0 {
		t.Fatalf("response.EmbeddingCount = %d, want 0", response.Msg.GetEmbeddingCount())
	}
	if len(response.Msg.GetLanguages()) != 0 {
		t.Fatalf("len(response.Languages) = %d, want 0", len(response.Msg.GetLanguages()))
	}
}

func TestIndexStatus_AfterIndex(t *testing.T) {
	t.Parallel()

	engine := codesearch.New(codesearch.WithMemoryStore(), codesearch.WithEmbedder(testEmbedder{}))
	files := map[string]string{
		"src/main.go":   "package main\n\nfunc main() {}\n",
		"src/module.py": "def main():\n    return True\n",
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
	if response.Msg.GetLanguages()["Go"] != 1 {
		t.Fatalf("response.Languages[Go] = %d, want 1", response.Msg.GetLanguages()["Go"])
	}
	if response.Msg.GetLanguages()["Python"] != 1 {
		t.Fatalf("response.Languages[Python] = %d, want 1", response.Msg.GetLanguages()["Python"])
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
	response, err := client.SearchSymbols(context.Background(), connect.NewRequest(&codesearchpb.SearchSymbolsRequest{Name: "main", Kind: "function", Language: "go", Limit: 10}))
	if err != nil {
		t.Fatalf("SearchSymbols() error = %v", err)
	}
	if len(response.Msg.GetResults()) != 1 {
		t.Fatalf("len(response.Results) = %d, want 1", len(response.Msg.GetResults()))
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

func TestNewCodeSearchHandler(t *testing.T) {
	t.Parallel()

	engine := codesearch.New(codesearch.WithMemoryStore())
	indexTestFiles(t, engine, map[string]string{"src/main.go": "package main\nfunc main() {}\n"})

	server := httptest.NewServer(NewCodeSearchHandler(engine))
	defer server.Close()

	client := codesearchv1connect.NewCodeSearchServiceClient(server.Client(), server.URL)
	response, err := client.Search(context.Background(), connect.NewRequest(&codesearchpb.SearchRequest{Query: "main", Limit: 5}))
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(response.Msg.GetResults()) == 0 {
		t.Fatal("expected search results, got none")
	}
}

func TestHelperConversions(t *testing.T) {
	t.Parallel()

	request := SearchRequest{Query: "needle", Limit: 5, Mode: "lexical", Filter: `language == "Go"`}
	if got := SearchRequestFromProto(request.ToProto()); got != request {
		t.Fatalf("SearchRequest round trip = %#v, want %#v", got, request)
	}

	response := SearchResponse{Query: "needle", Limit: 5, Mode: "lexical", Results: []SearchResult{{Path: "main.go", Line: 2, Score: 1.5, Snippet: "const needle = true", Matches: []MatchRange{{Start: 6, End: 12}}}}}
	if got := SearchResponseFromProto(response.ToProto()); got.Query != response.Query || len(got.Results) != 1 || got.Results[0].Path != response.Results[0].Path {
		t.Fatalf("SearchResponse round trip = %#v, want %#v", got, response)
	}

	symbolRequest := SearchSymbolsRequest{Name: "main", Kind: "function", Language: "go", Container: "pkg", Path: "main.go", Limit: 3}
	if got := SearchSymbolsRequestFromProto(symbolRequest.ToProto()); got != symbolRequest {
		t.Fatalf("SearchSymbolsRequest round trip = %#v, want %#v", got, symbolRequest)
	}

	symbolResponse := SearchSymbolsResponse{Results: []SymbolResult{{Name: "main", Kind: "function", Language: "go", Path: "main.go", Container: "pkg", Exported: true, Range: SourceRange{StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 4}}}}
	if got := SearchSymbolsResponseFromProto(symbolResponse.ToProto()); got.Results[0].Name != "main" {
		t.Fatalf("SearchSymbolsResponse round trip = %#v, want %#v", got, symbolResponse)
	}

	statusResponse := IndexStatusResponse{FileCount: 2, TotalBytes: 100, IndexBytes: 100, EmbeddingCount: 2, Languages: map[string]int32{"Go": 1}}
	if got := IndexStatusResponseFromProto(statusResponse.ToProto()); got.FileCount != statusResponse.FileCount || got.Languages["Go"] != 1 {
		t.Fatalf("IndexStatusResponse round trip = %#v, want %#v", got, statusResponse)
	}
	if got := IndexStatusRequestFromProto((&IndexStatusRequest{}).ToProto()); got != (IndexStatusRequest{}) {
		t.Fatalf("IndexStatusRequest round trip = %#v, want empty request", got)
	}
}

func TestHelperFunctions(t *testing.T) {
	t.Parallel()

	mode, err := normalizeMode(" lexical ")
	if err != nil {
		t.Fatalf("normalizeMode() error = %v", err)
	}
	if mode != "lexical_only" {
		t.Fatalf("normalizeMode() = %q, want %q", mode, "lexical_only")
	}
	if modeLabel(mode) != "lexical" {
		t.Fatalf("modeLabel() = %q, want %q", modeLabel(mode), "lexical")
	}
	if languageForPath("main.go") != "Go" {
		t.Fatalf("languageForPath() = %q, want %q", languageForPath("main.go"), "Go")
	}

	kind, err := parseSymbolKind("method")
	if err != nil {
		t.Fatalf("parseSymbolKind() error = %v", err)
	}
	if kind != 10 {
		t.Fatalf("parseSymbolKind() = %v, want %v", kind, 10)
	}
	if formatSymbolKind(kind) != "method" {
		t.Fatalf("formatSymbolKind() = %q, want %q", formatSymbolKind(kind), "method")
	}
	if _, err := parseSymbolKind("unknown-kind"); err == nil {
		t.Fatal("parseSymbolKind() error = nil, want error")
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
			t.Fatalf("IndexFile(%q) error = %v", path, err)
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

func TestSearchSymbolsWithKindFilter(t *testing.T) {
	t.Parallel()

	engine := codesearch.New(codesearch.WithMemoryStore())
	indexTestFiles(t, engine, map[string]string{
		"src/main.go": "package main\n\nfunc test() {}\nfunc helper() {}\n",
	})

	client, _ := newTestClient(t, engine)
	response, err := client.SearchSymbols(context.Background(), connect.NewRequest(&codesearchpb.SearchSymbolsRequest{
		Name:  "test",
		Kind:  "function",
		Limit: 10,
	}))
	if err != nil {
		t.Fatalf("SearchSymbols() error = %v", err)
	}
	if len(response.Msg.GetResults()) != 1 {
		t.Fatalf("len(response.Results) = %d, want 1", len(response.Msg.GetResults()))
	}
}

func TestSearchSymbolsInvalidKind(t *testing.T) {
	t.Parallel()

	engine := codesearch.New(codesearch.WithMemoryStore())
	client, _ := newTestClient(t, engine)

	_, err := client.SearchSymbols(context.Background(), connect.NewRequest(&codesearchpb.SearchSymbolsRequest{
		Name: "test",
		Kind: "invalid_kind_xyz",
	}))
	assertConnectErrorContains(t, err, connect.CodeInvalidArgument, "unknown symbol kind")
}
