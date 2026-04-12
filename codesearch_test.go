package codesearch

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SCKelemen/codesearch/hybrid"
	memorystore "github.com/SCKelemen/codesearch/store/memory"
	"github.com/SCKelemen/codesearch/structural"
)

type semanticMatchEmbedder struct{}

func (semanticMatchEmbedder) Embed(_ context.Context, inputs []string) ([][]float32, error) {
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

func (semanticMatchEmbedder) Dimensions() int { return 2 }
func (semanticMatchEmbedder) Model() string   { return "semantic-match" }

type engineStatus struct {
	fileCount      int
	totalBytes     int64
	embeddingCount int
	languages      map[string]int
}

func collectEngineStatus(t *testing.T, engine *Engine) engineStatus {
	t.Helper()

	documents, _, err := engine.Documents.List(context.Background())
	if err != nil {
		t.Fatalf("Documents.List() error = %v", err)
	}
	vectors, _, err := engine.Vectors.List(context.Background())
	if err != nil {
		t.Fatalf("Vectors.List() error = %v", err)
	}

	status := engineStatus{languages: make(map[string]int)}
	for _, document := range documents {
		status.fileCount++
		status.totalBytes += document.Size
		status.languages[document.Language]++
	}
	status.embeddingCount = len(vectors)
	return status
}

func TestNewEngine_Defaults(t *testing.T) {
	engine := New()

	if engine.initErr != nil {
		t.Fatalf("engine.initErr = %v, want nil", engine.initErr)
	}
	if _, ok := engine.Documents.(*memorystore.DocumentStore); !ok {
		t.Fatalf("Documents type = %T, want *memory.DocumentStore", engine.Documents)
	}
	if _, ok := engine.Trigrams.(*memorystore.TrigramStore); !ok {
		t.Fatalf("Trigrams type = %T, want *memory.TrigramStore", engine.Trigrams)
	}
	if _, ok := engine.Vectors.(*memorystore.VectorStore); !ok {
		t.Fatalf("Vectors type = %T, want *memory.VectorStore", engine.Vectors)
	}
	if _, ok := engine.Symbols.(*memorystore.SymbolStore); !ok {
		t.Fatalf("Symbols type = %T, want *memory.SymbolStore", engine.Symbols)
	}
	if engine.Embedder != nil {
		t.Fatal("Embedder is set, want nil")
	}
	if !engine.hybridEnabled {
		t.Fatal("hybridEnabled = false, want true")
	}
	if engine.HybridSearcher != nil {
		t.Fatalf("HybridSearcher = %#v, want nil without an embedder", engine.HybridSearcher)
	}
	if err := engine.ready(); err != nil {
		t.Fatalf("ready() error = %v, want nil", err)
	}
}

func TestNewEngine_WithOptions(t *testing.T) {
	documentStore := memorystore.NewDocumentStore()
	trigramStore := memorystore.NewTrigramStore()
	vectorStore := memorystore.NewVectorStore()
	symbolStore := memorystore.NewSymbolStore()
	embedder := semanticMatchEmbedder{}

	engine := New(
		WithMemoryStore(),
		WithDocumentStore(documentStore),
		WithTrigramStore(trigramStore),
		WithVectorStore(vectorStore),
		WithEmbedder(embedder),
		WithHybridSearch(false),
		func(cfg *engineConfig) {
			cfg.symbolStore = symbolStore
		},
	)

	if engine.Documents != documentStore {
		t.Fatal("Documents does not use the configured document store")
	}
	if engine.Trigrams != trigramStore {
		t.Fatal("Trigrams does not use the configured trigram store")
	}
	if engine.Vectors != vectorStore {
		t.Fatal("Vectors does not use the configured vector store")
	}
	if engine.Symbols != symbolStore {
		t.Fatal("Symbols does not use the configured symbol store")
	}
	if engine.Embedder != embedder {
		t.Fatalf("Embedder = %#v, want %#v", engine.Embedder, embedder)
	}
	if engine.hybridEnabled {
		t.Fatal("hybridEnabled = true, want false")
	}
	if engine.HybridSearcher == nil {
		t.Fatal("HybridSearcher = nil, want a configured hybrid searcher")
	}
}

func TestEngine_Index(t *testing.T) {
	ctx := context.Background()
	rootDir := t.TempDir()
	mainFile := filepath.Join(rootDir, "main.go")
	notesFile := filepath.Join(rootDir, "notes.txt")

	if err := os.WriteFile(mainFile, []byte("package main\n\nconst IndexedToken = \"indexed\"\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(main.go) error = %v", err)
	}
	if err := os.WriteFile(notesFile, []byte("indexed notes\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(notes.txt) error = %v", err)
	}

	engine := New(WithMemoryStore())
	if err := engine.Index(ctx, rootDir); err != nil {
		t.Fatalf("Index(%q) error = %v", rootDir, err)
	}

	results, err := engine.Search(ctx, "IndexedToken")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if got, want := results[0].Path, filepath.ToSlash(filepath.Clean(mainFile)); got != want {
		t.Fatalf("result.Path = %q, want %q", got, want)
	}

	status := collectEngineStatus(t, engine)
	if status.fileCount != 2 {
		t.Fatalf("status.fileCount = %d, want 2", status.fileCount)
	}
}

func TestEngine_Index_WithIndexOptions(t *testing.T) {
	ctx := context.Background()
	rootDir := t.TempDir()
	filePath := filepath.Join(rootDir, "custom.txt")
	if err := os.WriteFile(filePath, []byte("custom indexed content\n"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	extractorCalled := false
	engine := New(WithMemoryStore(), WithEmbedder(semanticMatchEmbedder{}))
	if err := engine.Index(
		ctx,
		filePath,
		WithLanguage("Custom"),
		WithEmbeddings(false),
		WithSymbolExtractor(func(_ context.Context, path, language string, content []byte) ([]structural.Symbol, error) {
			extractorCalled = true
			return []structural.Symbol{{
				Name:     filepath.Base(path),
				Kind:     structural.SymbolKindFunction,
				Language: strings.ToLower(language),
				Path:     filepath.ToSlash(filepath.Clean(path)),
				Range:    structural.Range{StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 6},
			}}, nil
		}),
	); err != nil {
		t.Fatalf("Index() error = %v", err)
	}

	if !extractorCalled {
		t.Fatal("symbol extractor was not called")
	}
	storedDocument, err := engine.Documents.Lookup(ctx, cleanPath(filePath))
	if err != nil {
		t.Fatalf("Documents.Lookup() error = %v", err)
	}
	if storedDocument == nil {
		t.Fatal("stored document = nil")
	}
	if storedDocument.Language != "Custom" {
		t.Fatalf("storedDocument.Language = %q, want %q", storedDocument.Language, "Custom")
	}
	vectors, _, err := engine.Vectors.List(ctx)
	if err != nil {
		t.Fatalf("Vectors.List() error = %v", err)
	}
	if len(vectors) != 0 {
		t.Fatalf("len(vectors) = %d, want 0 when embeddings are disabled", len(vectors))
	}
	symbols, _, err := engine.Symbols.List(ctx)
	if err != nil {
		t.Fatalf("Symbols.List() error = %v", err)
	}
	if len(symbols) != 1 {
		t.Fatalf("len(symbols) = %d, want 1", len(symbols))
	}
}

func TestEngine_Search(t *testing.T) {
	ctx := context.Background()
	engine := New(WithMemoryStore())

	if err := engine.IndexFile(ctx, "main.go", []byte("package main\nconst token = \"go\"\n")); err != nil {
		t.Fatalf("IndexFile() error = %v", err)
	}

	results, err := engine.Search(ctx, "go", WithLimit(1))
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Line != 2 {
		t.Fatalf("result.Line = %d, want 2", results[0].Line)
	}
	if results[0].Snippet == "" {
		t.Fatal("result.Snippet is empty")
	}
	if len(results[0].Matches) != 1 {
		t.Fatalf("len(result.Matches) = %d, want 1", len(results[0].Matches))
	}
}

func TestEngine_Search_Fuzzy(t *testing.T) {
	ctx := context.Background()
	engine := New(WithMemoryStore(), WithEmbedder(semanticMatchEmbedder{}), WithHybridSearch(true))

	if err := engine.IndexFile(ctx, "semantic.txt", []byte("semantic vector content")); err != nil {
		t.Fatalf("IndexFile(semantic.txt) error = %v", err)
	}
	if err := engine.IndexFile(ctx, "noise.txt", []byte("ordinary unrelated text")); err != nil {
		t.Fatalf("IndexFile(noise.txt) error = %v", err)
	}

	results, err := engine.Search(ctx, "approximate concept")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) == 0 {
		t.Fatal("Search() returned no results")
	}
	if results[0].Path != "semantic.txt" {
		t.Fatalf("first result path = %q, want %q", results[0].Path, "semantic.txt")
	}
}

func TestEngine_Search_Exact(t *testing.T) {
	ctx := context.Background()
	engine := New(WithMemoryStore())

	if err := engine.IndexFile(ctx, "exact.go", []byte("package main\nconst value = \"ExactValue\"\n")); err != nil {
		t.Fatalf("IndexFile() error = %v", err)
	}

	results, err := engine.Search(ctx, "ExactValue", WithMode(hybrid.LexicalOnly))
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Path != "exact.go" {
		t.Fatalf("result.Path = %q, want %q", results[0].Path, "exact.go")
	}
}

func TestEngine_Search_WithFilter(t *testing.T) {
	ctx := context.Background()
	engine := New(WithMemoryStore())

	if err := engine.IndexFile(ctx, "main.go", []byte("package main\nconst match = \"needle\"\n")); err != nil {
		t.Fatalf("IndexFile(main.go) error = %v", err)
	}
	if err := engine.IndexFile(ctx, "module.py", []byte("match = 'needle'\n")); err != nil {
		t.Fatalf("IndexFile(module.py) error = %v", err)
	}

	results, err := engine.Search(ctx, "needle", WithFilter(`language == "Go"`))
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Path != "main.go" {
		t.Fatalf("result.Path = %q, want %q", results[0].Path, "main.go")
	}
}

func TestEngine_Search_NoResults(t *testing.T) {
	ctx := context.Background()
	engine := New(WithMemoryStore())

	if err := engine.IndexFile(ctx, "main.go", []byte("package main\nconst present = true\n")); err != nil {
		t.Fatalf("IndexFile() error = %v", err)
	}

	results, err := engine.Search(ctx, "missing")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("len(results) = %d, want 0", len(results))
	}
}

func TestEngine_Status(t *testing.T) {
	ctx := context.Background()
	engine := New(WithMemoryStore(), WithEmbedder(semanticMatchEmbedder{}))

	if err := engine.IndexFile(ctx, "main.go", []byte("package main\nconst status = true\n")); err != nil {
		t.Fatalf("IndexFile(main.go) error = %v", err)
	}
	if err := engine.IndexFile(ctx, "module.py", []byte("status = True\n")); err != nil {
		t.Fatalf("IndexFile(module.py) error = %v", err)
	}

	status := collectEngineStatus(t, engine)
	if status.fileCount != 2 {
		t.Fatalf("status.fileCount = %d, want 2", status.fileCount)
	}
	if status.embeddingCount != 2 {
		t.Fatalf("status.embeddingCount = %d, want 2", status.embeddingCount)
	}
	if status.totalBytes <= 0 {
		t.Fatalf("status.totalBytes = %d, want > 0", status.totalBytes)
	}
	if status.languages["Go"] != 1 {
		t.Fatalf("status.languages[Go] = %d, want 1", status.languages["Go"])
	}
	if status.languages["Python"] != 1 {
		t.Fatalf("status.languages[Python] = %d, want 1", status.languages["Python"])
	}
}

func TestEngine_SearchSymbolsAndIndexSymbols(t *testing.T) {
	ctx := context.Background()
	engine := New(WithMemoryStore())

	if err := engine.IndexFile(ctx, "main.go", []byte("package main\n\ntype Greeter struct{}\n\nfunc (Greeter) Hello() {}\n")); err != nil {
		t.Fatalf("IndexFile(main.go) error = %v", err)
	}
	if err := engine.IndexFile(ctx, "widget.js", []byte("export class Widget {}\nexport function build() {}\n")); err != nil {
		t.Fatalf("IndexFile(widget.js) error = %v", err)
	}

	storedSymbols, _, err := engine.Symbols.List(ctx)
	if err != nil {
		t.Fatalf("Symbols.List() error = %v", err)
	}
	if len(storedSymbols) == 0 {
		t.Fatal("Symbols.List() returned no indexed symbols")
	}

	symbols, err := engine.SearchSymbols(ctx, structural.SymbolQuery{Name: "Widget"})
	if err != nil {
		t.Fatalf("SearchSymbols() error = %v", err)
	}
	if len(symbols) != 1 || symbols[0].Path != "widget.js" {
		t.Fatalf("SearchSymbols() = %#v, want Widget in widget.js", symbols)
	}

	results, err := engine.Search(ctx, "ignored", WithSymbolQuery(structural.SymbolQuery{Name: "Hello"}))
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Symbol == nil || results[0].Symbol.Name != "Hello" {
		t.Fatalf("result.Symbol = %#v, want Hello", results[0].Symbol)
	}
	if results[0].Snippet == "" || results[0].Line != 5 {
		t.Fatalf("result = %#v, want symbol snippet at line 5", results[0])
	}
}

func TestEngine_OpenRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	engine, err := Open(dir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	path := filepath.Join(dir, "persistent.go")
	if err := engine.IndexFile(ctx, path, []byte("package main\nconst persistent = true\n")); err != nil {
		t.Fatalf("IndexFile() error = %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	reopened, err := Open(dir)
	if err != nil {
		t.Fatalf("reopen error = %v", err)
	}
	defer func() { _ = reopened.Close() }()

	results, err := reopened.Search(ctx, "persistent")
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
}

func TestEngine_ReadyErrors(t *testing.T) {
	var nilEngine *Engine
	if err := nilEngine.ready(); err == nil {
		t.Fatal("nil engine ready() error = nil, want error")
	}

	broken := &Engine{}
	if err := broken.ready(); err == nil {
		t.Fatal("broken engine ready() error = nil, want error")
	}
}

func TestNewEngine_WithFileStoreOption(t *testing.T) {
	engine := New(WithFileStore(t.TempDir()))
	if engine.initErr != nil {
		t.Fatalf("engine.initErr = %v, want nil", engine.initErr)
	}
	if err := engine.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestEngine_Index_WithLanguageAlias(t *testing.T) {
	ctx := context.Background()
	engine := New(WithMemoryStore())
	if err := engine.Index(ctx, writeTempFile(t, "script.custom", "def greet():\n    return 1\n"), WithLanguage("py")); err != nil {
		t.Fatalf("Index() error = %v", err)
	}
	symbols, err := engine.SearchSymbols(ctx, structural.SymbolQuery{Name: "greet", Language: "python"})
	if err != nil {
		t.Fatalf("SearchSymbols() error = %v", err)
	}
	if len(symbols) != 1 {
		t.Fatalf("len(symbols) = %d, want 1", len(symbols))
	}
}

func writeTempFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q) error = %v", path, err)
	}
	return path
}

func TestEngine_Index_WithTypeScriptAlias(t *testing.T) {
	ctx := context.Background()
	engine := New(WithMemoryStore())
	if err := engine.Index(ctx, writeTempFile(t, "widget.custom", "export function buildWidget() {}\n"), WithLanguage("tsx")); err != nil {
		t.Fatalf("Index() error = %v", err)
	}
	symbols, err := engine.SearchSymbols(ctx, structural.SymbolQuery{Name: "buildWidget", Language: "typescript"})
	if err != nil {
		t.Fatalf("SearchSymbols() error = %v", err)
	}
	if len(symbols) != 1 {
		t.Fatalf("len(symbols) = %d, want 1", len(symbols))
	}
}
