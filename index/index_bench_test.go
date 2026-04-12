package index

import (
	"context"
	"fmt"
	"testing"

	"github.com/SCKelemen/codesearch/trigram"
)

var indexBenchStats Stats
var indexBenchMap map[string]any

type benchmarkSource struct {
	files []FileEntry
}

func (s *benchmarkSource) List(context.Context) ([]FileEntry, error) {
	return s.files, nil
}

func (s *benchmarkSource) Watch(context.Context) (<-chan Change, error) {
	ch := make(chan Change)
	close(ch)
	return ch, nil
}

type benchmarkTransformer struct{}

func (benchmarkTransformer) Transform(_ context.Context, entry FileEntry) (map[string]any, error) {
	tris := trigram.Extract(entry.Content)
	return map[string]any{
		"trigrams":  tris,
		"byte_size": len(entry.Content),
		"language":  entry.Language,
	}, nil
}

type benchmarkSink struct {
	items map[string]map[string]any
}

func newBenchmarkSink() *benchmarkSink {
	return &benchmarkSink{items: make(map[string]map[string]any)}
}

func (s *benchmarkSink) Store(_ context.Context, entry FileEntry, data map[string]any) error {
	copyData := make(map[string]any, len(data))
	for key, value := range data {
		copyData[key] = value
	}
	s.items[entry.URI] = copyData
	return nil
}

func (s *benchmarkSink) Delete(_ context.Context, uri string) error {
	delete(s.items, uri)
	return nil
}

func (s *benchmarkSink) Flush(context.Context) error {
	return nil
}

func BenchmarkPipelineRun(b *testing.B) {
	ctx := context.Background()
	source := &benchmarkSource{files: benchmarkIndexFiles(100)}
	transformer := benchmarkTransformer{}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		sink := newBenchmarkSink()
		pipeline := NewPipeline(source, sink, WithTransformer(transformer))
		stats, err := pipeline.Run(ctx)
		if err != nil {
			b.Fatalf("Run: %v", err)
		}
		indexBenchStats = stats
	}
}

func BenchmarkAddDocument(b *testing.B) {
	ctx := context.Background()
	entry := benchmarkIndexFiles(1)[0]
	sink := newBenchmarkSink()
	pipeline := NewPipeline(&benchmarkSource{files: []FileEntry{entry}}, sink, WithTransformer(benchmarkTransformer{}))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		data, err := pipeline.processFile(ctx, entry)
		if err != nil {
			b.Fatalf("processFile: %v", err)
		}
		if err := sink.Store(ctx, entry, data); err != nil {
			b.Fatalf("Store: %v", err)
		}
		indexBenchMap = data
	}
}

func benchmarkIndexFiles(count int) []FileEntry {
	files := make([]FileEntry, count)
	for i := range files {
		content := fmt.Sprintf("package workspace_%03d\n\nfunc HandleCheckoutRequest%03d() string {\n\treturn \"checkout request for workspace %03d\"\n}\n", i, i, i)
		files[i] = FileEntry{
			URI:      fmt.Sprintf("file:///workspace/services/checkout_%03d.go", i),
			Path:     fmt.Sprintf("services/checkout_%03d.go", i),
			Language: "go",
			Content:  []byte(content),
			Metadata: map[string]string{"team": "search", "component": "benchmark"},
		}
	}
	return files
}
