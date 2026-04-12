package trigram

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var trigramBenchResults []SearchResult
var trigramBenchIndexSize int

type benchmarkIndex struct {
	postings map[Trigram][]Posting
}

func newBenchmarkIndex() *benchmarkIndex {
	return &benchmarkIndex{postings: make(map[Trigram][]Posting)}
}

func (m *benchmarkIndex) Add(_ context.Context, tri Trigram, posting Posting) error {
	m.postings[tri] = append(m.postings[tri], posting)
	return nil
}

func (m *benchmarkIndex) Query(_ context.Context, trigrams []Trigram) ([]Posting, error) {
	seen := make(map[string]struct{})
	results := make([]Posting, 0, len(trigrams))
	for _, tri := range trigrams {
		for _, posting := range m.postings[tri] {
			key := postingKey(posting)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			results = append(results, posting)
		}
	}
	return results, nil
}

func (m *benchmarkIndex) Remove(_ context.Context, _ string) error {
	return nil
}

func BenchmarkExtractTrigrams(b *testing.B) {
	source := benchmarkSourceFile(10 * 1024)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		trigramBenchIndexSize = len(Extract(source))
	}
}

func BenchmarkTrigramSearch(b *testing.B) {
	ctx := context.Background()
	dir := b.TempDir()
	idx := newBenchmarkIndex()
	for i := range 1000 {
		path := filepath.Join(dir, fmt.Sprintf("doc-%04d.go", i))
		content := benchmarkDocumentContent(i, 6)
		if err := os.WriteFile(path, content, 0o600); err != nil {
			b.Fatalf("WriteFile(%s): %v", path, err)
		}
		posting := Posting{
			WorkspaceID:  "workspace-bench",
			RepositoryID: "repository-bench",
			IndexID:      "index-bench",
			FilePath:     path,
			Language:     "Go",
		}
		for _, tri := range Extract(content) {
			if err := idx.Add(ctx, tri, posting); err != nil {
				b.Fatalf("Add(%q): %v", tri, err)
			}
		}
	}

	searcher := NewSearcher(idx)
	opts := SearchOptions{
		WorkspaceID:  "workspace-bench",
		RepositoryID: "repository-bench",
		IndexID:      "index-bench",
		Language:     "Go",
		MaxResults:   25,
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		results, err := searcher.Search(ctx, "HandleCheckoutRequest", opts)
		if err != nil {
			b.Fatalf("Search: %v", err)
		}
		trigramBenchResults = results
	}
}

func BenchmarkBuildIndex(b *testing.B) {
	ctx := context.Background()
	documents := make([]struct {
		path    string
		content []byte
	}, 100)
	for i := range documents {
		documents[i].path = fmt.Sprintf("pkg/service_%03d/handler_%03d.go", i, i)
		documents[i].content = benchmarkDocumentContent(i, 8)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		idx := newBenchmarkIndex()
		for docID, document := range documents {
			posting := Posting{
				WorkspaceID:  "workspace-bench",
				RepositoryID: "repository-bench",
				IndexID:      fmt.Sprintf("run-%d", i),
				FilePath:     document.path,
				Language:     "Go",
			}
			for _, tri := range Extract(document.content) {
				if err := idx.Add(ctx, tri, posting); err != nil {
					b.Fatalf("Add(%d): %v", docID, err)
				}
			}
		}
		trigramBenchIndexSize = len(idx.postings)
	}
}

func benchmarkSourceFile(targetBytes int) []byte {
	var builder strings.Builder
	builder.Grow(targetBytes + 1024)
	for section := 0; builder.Len() < targetBytes; section++ {
		fmt.Fprintf(&builder, "package bench\n\n")
		fmt.Fprintf(&builder, "import \"context\"\n\n")
		fmt.Fprintf(&builder, "type CheckoutRequest%03d struct {\n\tUserID string\n\tWorkspaceID string\n\tAmountCents int\n}\n\n", section)
		fmt.Fprintf(&builder, "func HandleCheckoutRequest%03d(ctx context.Context, request CheckoutRequest%03d) error {\n", section, section)
		fmt.Fprintf(&builder, "\tmessage := \"processing checkout request for workspace \" + request.WorkspaceID\n")
		fmt.Fprintf(&builder, "\t_ = message\n")
		fmt.Fprintf(&builder, "\tfor attempt := 0; attempt < 3; attempt++ {\n")
		fmt.Fprintf(&builder, "\t\tif request.AmountCents > 1000 && attempt == 2 {\n")
		fmt.Fprintf(&builder, "\t\t\treturn nil\n")
		fmt.Fprintf(&builder, "\t\t}\n")
		fmt.Fprintf(&builder, "\t}\n")
		fmt.Fprintf(&builder, "\treturn nil\n")
		fmt.Fprintf(&builder, "}\n\n")
	}
	return []byte(builder.String()[:targetBytes])
}

func benchmarkDocumentContent(docID, repeats int) []byte {
	var builder strings.Builder
	builder.Grow(repeats * 256)
	fmt.Fprintf(&builder, "package bench%03d\n\n", docID)
	for block := 0; block < repeats; block++ {
		fmt.Fprintf(&builder, "type CheckoutWorker%03d_%03d struct {\n\tWorkspaceID string\n\tRepositoryID string\n}\n\n", docID, block)
		fmt.Fprintf(&builder, "func HandleCheckoutRequest(worker CheckoutWorker%03d_%03d, invoiceID string) string {\n", docID, block)
		fmt.Fprintf(&builder, "\tstatus := \"checkout request handled for \" + worker.WorkspaceID + \"/\" + invoiceID\n")
		fmt.Fprintf(&builder, "\tif strings.Contains(status, \"checkout\") {\n")
		fmt.Fprintf(&builder, "\t\treturn status\n")
		fmt.Fprintf(&builder, "\t}\n")
		fmt.Fprintf(&builder, "\treturn \"pending\"\n")
		fmt.Fprintf(&builder, "}\n\n")
		fmt.Fprintf(&builder, "func auditCheckout%03d_%03d() string {\n", docID, block)
		fmt.Fprintf(&builder, "\treturn \"audit checkout request for repository-%03d\"\n", docID)
		fmt.Fprintf(&builder, "}\n\n")
	}
	return []byte(builder.String())
}
