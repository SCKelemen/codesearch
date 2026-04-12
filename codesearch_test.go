package codesearch

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/SCKelemen/codesearch/hybrid"
	"github.com/SCKelemen/codesearch/structural"
)

type stubEmbedder struct{}

func (stubEmbedder) Embed(_ context.Context, inputs []string) ([][]float32, error) {
	vectors := make([][]float32, len(inputs))
	for i, input := range inputs {
		if input == "semantic" {
			vectors[i] = []float32{1, 0}
			continue
		}
		vectors[i] = []float32{0, 1}
	}
	return vectors, nil
}

func (stubEmbedder) Dimensions() int { return 2 }
func (stubEmbedder) Model() string   { return "stub" }

func TestEngineIndexFileAndSearch(t *testing.T) {
	ctx := context.Background()
	engine := New()
	if err := engine.IndexFile(ctx, filepath.Join(t.TempDir(), "main.go"), []byte("package main\nfunc main() { println(\"hello\") }\n")); err != nil {
		t.Fatalf("IndexFile returned error: %v", err)
	}

	results, err := engine.Search(ctx, "hello")
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].Line != 2 || results[0].Snippet == "" || len(results[0].Matches) == 0 {
		t.Fatalf("unexpected result: %#v", results[0])
	}
}

func TestEngineSearchWithFilter(t *testing.T) {
	ctx := context.Background()
	engine := New()

	if err := engine.IndexFile(ctx, "main.go", []byte("package main\nconst match = \"needle\"\n")); err != nil {
		t.Fatalf("IndexFile returned error: %v", err)
	}
	if err := engine.IndexFile(ctx, "notes.txt", []byte("needle\n")); err != nil {
		t.Fatalf("IndexFile returned error: %v", err)
	}

	results, err := engine.Search(ctx, "needle", WithFilter(`language == "Go"`))
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 1 || results[0].Path != "main.go" {
		t.Fatalf("Search with filter returned %#v, want only main.go", results)
	}
}

func TestEngineSearchSymbolsAndIndexSymbols(t *testing.T) {
	ctx := context.Background()
	engine := New()

	if err := engine.IndexFile(ctx, "main.go", []byte("package main\n\ntype Greeter struct{}\n\nfunc (Greeter) Hello() {}\n")); err != nil {
		t.Fatalf("IndexFile returned error: %v", err)
	}
	if err := engine.IndexFile(ctx, "widget.js", []byte("export class Widget {}\nexport function build() {}\n")); err != nil {
		t.Fatalf("IndexFile returned error: %v", err)
	}

	storedSymbols, _, err := engine.Symbols.List(ctx)
	if err != nil {
		t.Fatalf("Symbols.List returned error: %v", err)
	}
	if len(storedSymbols) == 0 {
		t.Fatal("Symbols.List returned no indexed symbols")
	}

	symbols, err := engine.SearchSymbols(ctx, structural.SymbolQuery{Name: "Widget"})
	if err != nil {
		t.Fatalf("SearchSymbols returned error: %v", err)
	}
	if len(symbols) != 1 || symbols[0].Path != "widget.js" {
		t.Fatalf("SearchSymbols returned %#v, want Widget in widget.js", symbols)
	}

	results, err := engine.Search(ctx, "ignored", WithSymbolQuery(structural.SymbolQuery{Name: "Hello"}))
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 1 || results[0].Symbol == nil || results[0].Symbol.Name != "Hello" {
		t.Fatalf("Search with symbol query returned %#v, want Hello symbol result", results)
	}
	if results[0].Snippet == "" || results[0].Line != 5 {
		t.Fatalf("Search symbol result = %#v, want line snippet for Hello", results[0])
	}
}

func TestEngineOpenRoundTrip(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	engine, err := Open(dir)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	path := filepath.Join(dir, "file.txt")
	if err := engine.IndexFile(ctx, path, []byte("persistent hello\n")); err != nil {
		t.Fatalf("IndexFile returned error: %v", err)
	}
	if err := engine.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	engine, err = Open(dir)
	if err != nil {
		t.Fatalf("reopen returned error: %v", err)
	}
	defer func() { _ = engine.Close() }()

	results, err := engine.Search(ctx, "hello")
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) != 1 || results[0].Path != filepath.ToSlash(filepath.Clean(path)) {
		t.Fatalf("unexpected results: %#v", results)
	}
}

func TestEngineSemanticSearch(t *testing.T) {
	ctx := context.Background()
	engine := New(WithEmbedder(stubEmbedder{}), WithHybridSearch(true))
	if err := engine.IndexFile(ctx, "semantic.txt", []byte("semantic document")); err != nil {
		t.Fatalf("IndexFile returned error: %v", err)
	}
	if err := engine.IndexFile(ctx, "lexical.txt", []byte("lexical document")); err != nil {
		t.Fatalf("IndexFile returned error: %v", err)
	}

	results, err := engine.Search(ctx, "semantic", WithMode(hybrid.SemanticOnly))
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(results) == 0 {
		t.Fatalf("unexpected semantic results: %#v", results)
	}
	found := false
	for _, result := range results {
		if result.Path == "semantic.txt" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("unexpected semantic results: %#v", results)
	}
}
