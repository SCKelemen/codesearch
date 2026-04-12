package embedding

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

type stubEmbedder struct {
	embed      func(context.Context, []string) ([][]float32, error)
	dimensions int
	model      string
}

func (s stubEmbedder) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	return s.embed(ctx, inputs)
}

func (s stubEmbedder) Dimensions() int { return s.dimensions }
func (s stubEmbedder) Model() string   { return s.model }

func TestFunctionChunkerEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("falls back to file chunk when no boundaries exist", func(t *testing.T) {
		t.Parallel()

		content := []byte("just plain text\nwith no functions\n")
		chunks, err := (FunctionChunker{}).Chunk("README.md", content)
		if err != nil {
			t.Fatalf("Chunk returned error: %v", err)
		}
		want := []Chunk{{Path: "README.md", StartLine: 1, EndLine: 2, Content: "just plain text\nwith no functions", Language: "md"}}
		if !reflect.DeepEqual(chunks, want) {
			t.Fatalf("chunks = %#v, want %#v", chunks, want)
		}
	})

	t.Run("drops whitespace only preamble", func(t *testing.T) {
		t.Parallel()

		content := []byte("\n\nfunc Run() {}\n")
		chunks, err := (FunctionChunker{}).Chunk("main.go", content)
		if err != nil {
			t.Fatalf("Chunk returned error: %v", err)
		}
		if len(chunks) != 1 {
			t.Fatalf("len(chunks) = %d, want 1", len(chunks))
		}
		if chunks[0].StartLine != 3 || chunks[0].EndLine != 3 {
			t.Fatalf("chunk range = %d-%d, want 3-3", chunks[0].StartLine, chunks[0].EndLine)
		}
	})

	t.Run("handles exported async arrow functions", func(t *testing.T) {
		t.Parallel()

		content := []byte("export const fetchThing = async (value: string) => {\n  return value;\n};\n")
		chunks, err := (FunctionChunker{}).Chunk("thing.ts", content)
		if err != nil {
			t.Fatalf("Chunk returned error: %v", err)
		}
		if len(chunks) != 1 {
			t.Fatalf("len(chunks) = %d, want 1", len(chunks))
		}
		if !strings.Contains(chunks[0].Content, "fetchThing") {
			t.Fatalf("chunk content = %q, want fetchThing", chunks[0].Content)
		}
	})
}

func TestFixedWindowChunkerAndHelpers(t *testing.T) {
	t.Parallel()

	t.Run("empty input returns nil", func(t *testing.T) {
		t.Parallel()

		chunks, err := (FixedWindowChunker{Size: 4, Overlap: 1}).Chunk("empty.go", nil)
		if err != nil {
			t.Fatalf("Chunk returned error: %v", err)
		}
		if chunks != nil {
			t.Fatalf("chunks = %#v, want nil", chunks)
		}
	})

	t.Run("rejects negative overlap", func(t *testing.T) {
		t.Parallel()

		_, err := (FixedWindowChunker{Size: 3, Overlap: -1}).Chunk("main.go", []byte("a\nb\nc\n"))
		if err == nil || !strings.Contains(err.Error(), "non-negative") {
			t.Fatalf("Chunk error = %v, want overlap validation error", err)
		}
	})

	t.Run("handles large crlf input", func(t *testing.T) {
		t.Parallel()

		var builder strings.Builder
		for line := 1; line <= 101; line++ {
			builder.WriteString("line")
			builder.WriteString(strings.Repeat("x", line%7))
			builder.WriteString("\r\n")
		}
		chunks, err := (FixedWindowChunker{Size: 10, Overlap: 3}).Chunk("large.py", []byte(builder.String()))
		if err != nil {
			t.Fatalf("Chunk returned error: %v", err)
		}
		if len(chunks) != 14 {
			t.Fatalf("len(chunks) = %d, want 14", len(chunks))
		}
		if chunks[0].StartLine != 1 || chunks[0].EndLine != 10 {
			t.Fatalf("first chunk = %d-%d, want 1-10", chunks[0].StartLine, chunks[0].EndLine)
		}
		last := chunks[len(chunks)-1]
		if last.StartLine != 92 || last.EndLine != 101 {
			t.Fatalf("last chunk = %d-%d, want 92-101", last.StartLine, last.EndLine)
		}
		if last.Language != "python" {
			t.Fatalf("last chunk language = %q, want python", last.Language)
		}
	})

	t.Run("split lines normalizes carriage returns", func(t *testing.T) {
		t.Parallel()

		lines := splitLines([]byte("a\rb\r\nc\n"))
		want := []string{"a", "b", "c"}
		if !reflect.DeepEqual(lines, want) {
			t.Fatalf("splitLines = %v, want %v", lines, want)
		}
	})

	t.Run("detect language covers common extensions", func(t *testing.T) {
		t.Parallel()

		cases := map[string]string{
			"main.go":    "go",
			"worker.tsx": "typescript",
			"script.js":  "javascript",
			"header.hpp": "cpp",
			"unknown.zz": "zz",
		}
		for path, want := range cases {
			if got := detectLanguage(path); got != want {
				t.Fatalf("detectLanguage(%q) = %q, want %q", path, got, want)
			}
		}
	})
}

func TestBatchEmbedderAndNoopEmbedderEdgeCases(t *testing.T) {
	t.Parallel()

	t.Run("batch embedder propagates errors", func(t *testing.T) {
		t.Parallel()

		embedder := NewBatchEmbedder(stubEmbedder{
			embed: func(context.Context, []string) ([][]float32, error) {
				return nil, errors.New("boom")
			},
			dimensions: 4,
			model:      "stub",
		}, BatchConfig{BatchSize: 2, MaxConcurrent: 2})

		_, err := embedder.Embed(context.Background(), []string{"a", "b"})
		if err == nil || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("Embed error = %v, want boom", err)
		}
		if embedder.Dimensions() != 4 {
			t.Fatalf("Dimensions = %d, want 4", embedder.Dimensions())
		}
		if embedder.Model() != "stub" {
			t.Fatalf("Model = %q, want stub", embedder.Model())
		}
	})

	t.Run("batch embedder rejects mismatched vector counts", func(t *testing.T) {
		t.Parallel()

		embedder := NewBatchEmbedder(stubEmbedder{
			embed: func(context.Context, []string) ([][]float32, error) {
				return [][]float32{{1}}, nil
			},
		}, BatchConfig{BatchSize: 2, MaxConcurrent: 1})

		_, err := embedder.Embed(context.Background(), []string{"a", "b"})
		if err == nil || !strings.Contains(err.Error(), "returned 1 embeddings for 2 inputs") {
			t.Fatalf("Embed error = %v, want mismatched count", err)
		}
	})

	t.Run("batch embedder respects canceled context while waiting", func(t *testing.T) {
		t.Parallel()

		embedder := NewBatchEmbedder(stubEmbedder{
			embed: func(ctx context.Context, inputs []string) ([][]float32, error) {
				if inputs[0] == "hold" {
					select {
					case <-time.After(100 * time.Millisecond):
					case <-ctx.Done():
						return nil, ctx.Err()
					}
				}
				return [][]float32{{1}}, nil
			},
		}, BatchConfig{BatchSize: 1, MaxConcurrent: 1})

		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		_, err := embedder.Embed(ctx, []string{"hold", "wait"})
		if err == nil || !strings.Contains(err.Error(), context.DeadlineExceeded.Error()) {
			t.Fatalf("Embed error = %v, want deadline exceeded", err)
		}
	})

	t.Run("noop embedder normalizes dimensions and cancellation", func(t *testing.T) {
		t.Parallel()

		noop := NewNoopEmbedder(-5)
		if noop.Dimensions() != 0 {
			t.Fatalf("Dimensions = %d, want 0", noop.Dimensions())
		}
		if noop.Model() != "noop" {
			t.Fatalf("Model = %q, want noop", noop.Model())
		}

		vectors, err := noop.Embed(context.Background(), []string{"a", "b"})
		if err != nil {
			t.Fatalf("Embed returned error: %v", err)
		}
		want := [][]float32{{}, {}}
		if !reflect.DeepEqual(vectors, want) {
			t.Fatalf("vectors = %#v, want %#v", vectors, want)
		}

		canceled, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = noop.Embed(canceled, []string{"a"})
		if err == nil || !strings.Contains(err.Error(), context.Canceled.Error()) {
			t.Fatalf("Embed error = %v, want context canceled", err)
		}

		plain := NoopEmbedder{}
		if plain.Model() != "noop" {
			t.Fatalf("Model = %q, want noop", plain.Model())
		}
	})
}
