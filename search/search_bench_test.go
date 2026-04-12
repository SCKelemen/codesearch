package search

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/SCKelemen/codesearch/exact"
	"github.com/SCKelemen/codesearch/trigram"
)

var searchBenchResults []Result

type benchmarkTrigramIndex struct {
	postings map[trigram.Trigram][]trigram.Posting
}

func newBenchmarkTrigramIndex() *benchmarkTrigramIndex {
	return &benchmarkTrigramIndex{postings: make(map[trigram.Trigram][]trigram.Posting)}
}

func (m *benchmarkTrigramIndex) Add(_ context.Context, tri trigram.Trigram, posting trigram.Posting) error {
	m.postings[tri] = append(m.postings[tri], posting)
	return nil
}

func (m *benchmarkTrigramIndex) Query(_ context.Context, trigrams []trigram.Trigram) ([]trigram.Posting, error) {
	seen := make(map[string]struct{})
	results := make([]trigram.Posting, 0, len(trigrams))
	for _, tri := range trigrams {
		for _, posting := range m.postings[tri] {
			key := posting.WorkspaceID + "\x00" + posting.RepositoryID + "\x00" + posting.IndexID + "\x00" + posting.FilePath
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			results = append(results, posting)
		}
	}
	return results, nil
}

func (m *benchmarkTrigramIndex) Remove(_ context.Context, _ string) error {
	return nil
}

func BenchmarkSearch(b *testing.B) {
	engine := NewEngine(benchmarkExactSearchOptions(100)...)
	ctx := context.Background()
	request := Request{Query: "HandleCheckoutRequest", Mode: ModeExact, MaxResults: 20}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		results, err := engine.Search(ctx, request)
		if err != nil {
			b.Fatalf("Search: %v", err)
		}
		searchBenchResults = results
	}
}

func BenchmarkSearchWithFilters(b *testing.B) {
	ctx := context.Background()
	dir := b.TempDir()
	idx := newBenchmarkTrigramIndex()
	for i := range 100 {
		language := "Go"
		ext := ".go"
		if i%2 == 1 {
			language = "Text"
			ext = ".txt"
		}
		path := filepath.Join(dir, fmt.Sprintf("doc-%03d%s", i, ext))
		content := benchmarkSearchDocument(i, language)
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			b.Fatalf("WriteFile(%s): %v", path, err)
		}
		posting := trigram.Posting{
			WorkspaceID:  "workspace-bench",
			RepositoryID: "repository-bench",
			IndexID:      "index-bench",
			FilePath:     path,
			Language:     language,
		}
		for _, tri := range trigram.Extract([]byte(content)) {
			if err := idx.Add(ctx, tri, posting); err != nil {
				b.Fatalf("Add(%q): %v", tri, err)
			}
		}
	}

	engine := NewEngine(WithTrigramIndex(idx))
	request := Request{
		Query:        "HandleCheckoutRequest",
		Mode:         ModeRegex,
		MaxResults:   20,
		WorkspaceID:  "workspace-bench",
		RepositoryID: "repository-bench",
		Language:     "Go",
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		results, err := engine.Search(ctx, request)
		if err != nil {
			b.Fatalf("Search: %v", err)
		}
		searchBenchResults = results
	}
}

func benchmarkExactSearchOptions(count int) []EngineOption {
	opts := make([]EngineOption, 0, count)
	for i := range count {
		uri := fmt.Sprintf("services/workspace_%03d/handler_%03d.go", i, i)
		content := []byte(benchmarkSearchDocument(i, "Go"))
		opts = append(opts, WithExactIndex(uri, exact.NewIndex(content)))
	}
	return opts
}

func benchmarkSearchDocument(docID int, language string) string {
	var builder strings.Builder
	fmt.Fprintf(&builder, "package workspace_%03d\n\n", docID)
	for block := 0; block < 20; block++ {
		fmt.Fprintf(&builder, "func HandleCheckoutRequest%03d_%03d() string {\n", docID, block)
		fmt.Fprintf(&builder, "\treturn \"HandleCheckoutRequest processed %s workspace billing workflow %03d\"\n", language, docID)
		fmt.Fprintf(&builder, "}\n\n")
	}
	return builder.String()
}
