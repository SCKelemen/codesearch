package codesearch

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/SCKelemen/codesearch/hybrid"
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
