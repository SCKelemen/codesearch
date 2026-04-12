package shard

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/SCKelemen/codesearch/store"
)

var shardBenchBytes []byte
var shardBenchDocuments []store.Document

func BenchmarkWriteShard(b *testing.B) {
	documents := benchmarkShardDocuments(1000)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		builder := benchmarkShardBuilder(documents)
		var out bytes.Buffer
		if _, err := builder.WriteTo(&out); err != nil {
			b.Fatalf("WriteTo: %v", err)
		}
		shardBenchBytes = out.Bytes()
	}
}

func BenchmarkReadShard(b *testing.B) {
	ctx := context.Background()
	shard := benchmarkBuiltShard(b, benchmarkShardDocuments(1000))
	raw, err := shard.MarshalBinary()
	if err != nil {
		b.Fatalf("MarshalBinary: %v", err)
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		loaded, err := FromBytes(raw)
		if err != nil {
			b.Fatalf("FromBytes: %v", err)
		}
		documents, err := loaded.SearchDocuments(ctx, "HandleCheckoutRequest")
		if err != nil {
			b.Fatalf("SearchDocuments: %v", err)
		}
		shardBenchDocuments = documents
	}
}

func BenchmarkMergeShard(b *testing.B) {
	ctx := context.Background()
	left := benchmarkBuiltShard(b, benchmarkShardDocuments(500))
	right := benchmarkBuiltShard(b, benchmarkShardDocumentsFromOffset(500, 500))
	searcher := NewSearcher(left, right)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		documents, err := searcher.SearchDocuments(ctx, "HandleCheckoutRequest")
		if err != nil {
			b.Fatalf("SearchDocuments: %v", err)
		}
		shardBenchDocuments = documents
	}
}

func benchmarkShardDocuments(count int) []store.Document {
	return benchmarkShardDocumentsFromOffset(count, 0)
}

func benchmarkShardDocumentsFromOffset(count, offset int) []store.Document {
	documents := make([]store.Document, count)
	for i := range documents {
		id := offset + i
		documents[i] = store.Document{
			ID:           fmt.Sprintf("repo/main/services/checkout_%04d.go", id),
			RepositoryID: "repo",
			Branch:       "main",
			Path:         fmt.Sprintf("services/checkout_%04d.go", id),
			Language:     "go",
			Content:      []byte(fmt.Sprintf("package bench\n\nfunc HandleCheckoutRequest%04d() string {\n\treturn \"checkout request %04d\"\n}\n", id, id)),
			CreatedAt:    time.Unix(1_700_000_000+int64(id), 0).UTC(),
			UpdatedAt:    time.Unix(1_700_000_000+int64(id), 0).UTC(),
		}
	}
	return documents
}

func benchmarkShardBuilder(documents []store.Document) *Builder {
	builder := NewBuilder()
	builder.SetMeta(store.ShardMeta{RepositoryID: "repo", Branch: "main", Tier: store.T1, CreatedAt: time.Unix(1_700_000_000, 0).UTC()})
	for _, document := range documents {
		if err := builder.AddDocument(context.Background(), document); err != nil {
			panic(err)
		}
		for _, tri := range documentTrigrams(document.Content) {
			if err := builder.AddPostingList(context.Background(), store.PostingList{Trigram: tri, DocumentIDs: []string{document.ID}}); err != nil {
				panic(err)
			}
		}
	}
	return builder
}

func benchmarkBuiltShard(b *testing.B, documents []store.Document) store.IndexShard {
	b.Helper()
	shard, err := benchmarkShardBuilder(documents).Build(context.Background())
	if err != nil {
		b.Fatalf("Build: %v", err)
	}
	return shard
}

func documentTrigrams(content []byte) []store.Trigram {
	tris := make([]store.Trigram, 0)
	seen := make(map[store.Trigram]struct{})
	for i := 0; i+3 <= len(content); i++ {
		tri := store.NewTrigram(content[i], content[i+1], content[i+2])
		if _, ok := seen[tri]; ok {
			continue
		}
		seen[tri] = struct{}{}
		tris = append(tris, tri)
	}
	return tris
}
