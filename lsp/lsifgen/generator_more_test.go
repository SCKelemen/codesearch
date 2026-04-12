package lsifgen

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/SCKelemen/codesearch/lsp"
)

func TestGenerate_EmptyInput(t *testing.T) {
	generator := NewGenerator(&lsp.Multiplexer{})
	var buf bytes.Buffer

	count, err := generator.Generate(context.Background(), map[string]string{}, &buf)
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("Generate() count = %d, want 2", count)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("Generate() produced %d lines, want 2", len(lines))
	}

	var metadata map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &metadata); err != nil {
		t.Fatalf("json.Unmarshal(metadata) error = %v", err)
	}
	if metadata["label"] != "metaData" {
		t.Fatalf("metadata label = %v, want metaData", metadata["label"])
	}

	var project map[string]any
	if err := json.Unmarshal([]byte(lines[1]), &project); err != nil {
		t.Fatalf("json.Unmarshal(project) error = %v", err)
	}
	if project["label"] != "project" {
		t.Fatalf("project label = %v, want project", project["label"])
	}
}

func TestGenerate_OutputFormat(t *testing.T) {
	generator := NewGenerator(&lsp.Multiplexer{})
	var buf bytes.Buffer

	if _, err := generator.Generate(context.Background(), map[string]string{}, &buf); err != nil {
		t.Fatalf("Generate() error = %v", err)
	}

	for i, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if strings.TrimSpace(line) == "" {
			t.Fatalf("line %d is empty, want one JSON object per line", i)
		}
		var entry map[string]any
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatalf("line %d is not valid JSON: %v", i, err)
		}
	}
}

func TestGenerateWithStats_Counting(t *testing.T) {
	generator := NewGenerator(&lsp.Multiplexer{})
	var buf bytes.Buffer

	stats, err := generator.GenerateWithStats(context.Background(), map[string]string{
		"example.go": "package main\n",
	}, &buf)
	if err != nil {
		t.Fatalf("GenerateWithStats() error = %v", err)
	}
	if stats.Documents != 0 {
		t.Fatalf("Documents = %d, want 0", stats.Documents)
	}
	if stats.Errors != 1 {
		t.Fatalf("Errors = %d, want 1", stats.Errors)
	}
	if stats.Symbols != 0 || stats.References != 0 || stats.HoverInfos != 0 || stats.Definitions != 0 {
		t.Fatalf("stats = %#v, want only one error counted", stats)
	}
}

func TestNewGenerator_Defaults(t *testing.T) {
	generator := NewGenerator(nil)
	if generator.Version != "0.4.3" {
		t.Fatalf("Version = %q, want 0.4.3", generator.Version)
	}
	if generator.version() != "0.4.3" {
		t.Fatalf("version() = %q, want 0.4.3", generator.version())
	}
}

func TestGenerate_NilChecks(t *testing.T) {
	var nilGenerator *Generator
	if _, err := nilGenerator.Generate(context.Background(), map[string]string{}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "nil generator") {
		t.Fatalf("Generate(nil) error = %v, want nil generator", err)
	}

	generator := &Generator{}
	if _, err := generator.Generate(context.Background(), map[string]string{}, &bytes.Buffer{}); err == nil || !strings.Contains(err.Error(), "nil multiplexer") {
		t.Fatalf("Generate() error = %v, want nil multiplexer", err)
	}
	generator.Mux = &lsp.Multiplexer{}
	if _, err := generator.Generate(context.Background(), map[string]string{}, nil); err == nil || !strings.Contains(err.Error(), "nil writer") {
		t.Fatalf("Generate() error = %v, want nil writer", err)
	}
}

func TestEnsureDocumentAndRange_Deduplicate(t *testing.T) {
	generator := NewGenerator(&lsp.Multiplexer{})
	stats := &Stats{Languages: make(map[string]int)}
	docs := make(map[string]*documentState)
	ranges := make(map[string]rangeRef)
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)

	path := "/tmp/example.go"
	uri := lsp.FileURI(path)
	doc, created := generator.ensureDocument(enc, docs, path, uri, stats)
	if !created {
		t.Fatal("ensureDocument() created = false, want true")
	}
	if doc.language != "go" {
		t.Fatalf("language = %q, want go", doc.language)
	}
	if stats.Documents != 1 || stats.Languages["go"] != 1 {
		t.Fatalf("stats after ensureDocument() = %#v, want one Go document", stats)
	}

	if sameDoc, created := generator.ensureDocument(enc, docs, path, uri, stats); created || sameDoc != doc {
		t.Fatalf("second ensureDocument() = (%p, %v), want (%p, false)", sameDoc, created, doc)
	}

	rng := lsp.Range{
		Start: lsp.Position{Line: 1, Character: 0},
		End:   lsp.Position{Line: 1, Character: 4},
	}
	rangeID, created := generator.ensureRange(enc, ranges, docs, path, uri, rng, map[string]any{"type": "definition"}, stats)
	if !created {
		t.Fatal("ensureRange() created = false, want true")
	}
	if rangeID == 0 || len(doc.ranges) != 1 {
		t.Fatalf("rangeID = %d, doc.ranges = %#v, want one range", rangeID, doc.ranges)
	}

	if sameRangeID, created := generator.ensureRange(enc, ranges, docs, path, uri, rng, map[string]any{"type": "definition"}, stats); created || sameRangeID != rangeID {
		t.Fatalf("second ensureRange() = (%d, %v), want (%d, false)", sameRangeID, created, rangeID)
	}
}

func TestGeneratorHelpers(t *testing.T) {
	t.Run("flattenSymbols", func(t *testing.T) {
		symbols := []lsp.Symbol{{
			Name: "parent",
			Children: []lsp.Symbol{{
				Name: "child",
			}},
		}}
		flat := flattenSymbols(symbols)
		if len(flat) != 2 || flat[0].Name != "parent" || flat[1].Name != "child" {
			t.Fatalf("flattenSymbols() = %#v, want parent then child", flat)
		}
	})

	t.Run("normalizeRange", func(t *testing.T) {
		fallback := lsp.Range{Start: lsp.Position{Line: 2, Character: 1}}
		if got := normalizeRange(lsp.Range{}, fallback); got != fallback {
			t.Fatalf("normalizeRange() = %#v, want fallback %#v", got, fallback)
		}
	})

	t.Run("symbolPosition", func(t *testing.T) {
		symbol := lsp.Symbol{
			Range:          lsp.Range{Start: lsp.Position{Line: 1, Character: 1}},
			SelectionRange: lsp.Range{Start: lsp.Position{Line: 3, Character: 4}},
		}
		if got := symbolPosition(symbol); got != (lsp.Position{Line: 3, Character: 4}) {
			t.Fatalf("symbolPosition() = %#v, want selection range start", got)
		}
	})

	t.Run("languageForPath", func(t *testing.T) {
		tests := map[string]string{
			"main.go":   "go",
			"index.tsx": "typescriptreact",
			"README":    "plaintext",
			"file.xyz":  "xyz",
		}
		for path, want := range tests {
			if got := languageForPath(path); got != want {
				t.Fatalf("languageForPath(%q) = %q, want %q", path, got, want)
			}
		}
	})

	t.Run("addRange deduplicates", func(t *testing.T) {
		doc := &documentState{rangeSet: make(map[int]struct{})}
		doc.addRange(7)
		doc.addRange(7)
		if len(doc.ranges) != 1 || doc.ranges[0] != 7 {
			t.Fatalf("doc.ranges = %#v, want [7]", doc.ranges)
		}
	})
}
