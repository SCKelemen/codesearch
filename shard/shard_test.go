package shard

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/SCKelemen/codesearch/store"
)

func TestShardRoundTripAndSearch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	builder := NewBuilder()
	builder.SetMeta(store.ShardMeta{
		RepositoryID: "repo-1",
		Branch:       "main",
		CreatedAt:    time.Unix(1_700_000_000, 0).UTC(),
		Tier:         store.T1,
	})

	docMain, err := builder.AddFile("main.go", []byte("package main\n\nfunc HelloWorld() string {\n\treturn \"Hello shard\"\n}\n"), "go")
	if err != nil {
		t.Fatalf("AddFile main.go: %v", err)
	}
	docUtil, err := builder.AddFile("util.txt", []byte("hello helper text\n"), "text")
	if err != nil {
		t.Fatalf("AddFile util.txt: %v", err)
	}

	vector, err := builder.AddEmbedding("main.go", 3, 4, []float32{1, 0, 0})
	if err != nil {
		t.Fatalf("AddEmbedding: %v", err)
	}
	if _, err := builder.AddEmbedding("util.txt", 1, 1, []float32{0, 1, 0}); err != nil {
		t.Fatalf("AddEmbedding util: %v", err)
	}

	symbol := store.Symbol{
		Name:       "HelloWorld",
		Kind:       store.SymbolKindFunction,
		Path:       "main.go",
		Language:   "go",
		Definition: true,
		Exported:   true,
		Range:      store.Span{StartLine: 3, StartColumn: 1, EndLine: 3, EndColumn: 16},
		Signature:  "func HelloWorld() string",
	}
	if err := builder.AddSymbol(ctx, symbol); err != nil {
		t.Fatalf("AddSymbol: %v", err)
	}
	if err := builder.AddReference(ctx, store.Reference{
		SymbolID:   "main.go:HelloWorld:3:1",
		DocumentID: docUtil.ID,
		Path:       "util.txt",
		Range:      store.Span{StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 5},
	}); err != nil {
		t.Fatalf("AddReference: %v", err)
	}

	indexShard, err := builder.Build(ctx)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}
	raw, err := indexShard.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary: %v", err)
	}
	if got := indexShard.Meta().FileCount; got != 2 {
		t.Fatalf("Meta().FileCount = %d, want 2", got)
	}
	if got := indexShard.Meta().ByteSize; got != int64(len(raw)) {
		t.Fatalf("Meta().ByteSize = %d, want %d", got, len(raw))
	}

	var streamed bytes.Buffer
	if _, err := builder.WriteTo(&streamed); err != nil {
		t.Fatalf("WriteTo: %v", err)
	}
	if !bytes.Equal(streamed.Bytes(), raw) {
		t.Fatalf("WriteTo bytes differ from MarshalBinary output")
	}

	loaded, err := FromBytes(raw)
	if err != nil {
		t.Fatalf("FromBytes: %v", err)
	}

	docs, next, err := loaded.Documents().List(ctx)
	if err != nil {
		t.Fatalf("Documents().List: %v", err)
	}
	if next != "" {
		t.Fatalf("Documents().List next cursor = %q, want empty", next)
	}
	if len(docs) != 2 {
		t.Fatalf("Documents().List len = %d, want 2", len(docs))
	}
	if !reflect.DeepEqual(docs[0], docMain) {
		t.Fatalf("first document mismatch\n got: %#v\nwant: %#v", docs[0], docMain)
	}

	triResults, err := loaded.SearchTrigrams(ctx, []store.Trigram{store.NewTrigram('H', 'e', 'l')})
	if err != nil {
		t.Fatalf("SearchTrigrams: %v", err)
	}
	if len(triResults) != 1 || triResults[0].DocumentID != docMain.ID {
		t.Fatalf("SearchTrigrams got %#v, want only %q", triResults, docMain.ID)
	}

	vecResults, err := loaded.SearchVectors(ctx, []float32{1, 0, 0}, 1, store.DistanceMetricCosine)
	if err != nil {
		t.Fatalf("SearchVectors: %v", err)
	}
	if len(vecResults) != 1 || vecResults[0].Vector.ID != vector.ID {
		t.Fatalf("SearchVectors got %#v, want top vector %q", vecResults, vector.ID)
	}

	symbolResults, err := loaded.SearchSymbols(ctx, "hello")
	if err != nil {
		t.Fatalf("SearchSymbols: %v", err)
	}
	if len(symbolResults) != 1 || symbolResults[0].Name != "HelloWorld" {
		t.Fatalf("SearchSymbols got %#v, want HelloWorld", symbolResults)
	}

	docResults, err := loaded.SearchDocuments(ctx, "hello shard")
	if err != nil {
		t.Fatalf("SearchDocuments: %v", err)
	}
	if len(docResults) != 1 || docResults[0].ID != docMain.ID {
		t.Fatalf("SearchDocuments got %#v, want %q", docResults, docMain.ID)
	}

	refs, next, err := loaded.Symbols().References(ctx, "main.go:HelloWorld:3:1")
	if err != nil {
		t.Fatalf("References: %v", err)
	}
	if next != "" || len(refs) != 1 || refs[0].DocumentID != docUtil.ID {
		t.Fatalf("References got %#v next=%q", refs, next)
	}

	tempDir := t.TempDir()
	shardPath := filepath.Join(tempDir, "sample.cshr")
	if err := os.WriteFile(shardPath, raw, 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	mmapped, err := Open(shardPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer mmapped.Close()

	gotRaw, err := mmapped.MarshalBinary()
	if err != nil {
		t.Fatalf("Open().MarshalBinary: %v", err)
	}
	if !bytes.Equal(gotRaw, raw) {
		t.Fatalf("Open().MarshalBinary bytes differ from original")
	}
}

func TestMultiShardSearcher(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	first := NewBuilder()
	first.SetMeta(store.ShardMeta{RepositoryID: "repo-1", Branch: "main", CreatedAt: time.Unix(1_700_000_000, 0).UTC(), Tier: store.T1})
	firstDoc, err := first.AddFile("main.go", []byte("Hello from shard one\n"), "go")
	if err != nil {
		t.Fatalf("first AddFile: %v", err)
	}
	if err := first.AddSymbol(ctx, store.Symbol{Name: "HelloWorld", Kind: store.SymbolKindFunction, Path: "main.go", Language: "go", Definition: true, Range: store.Span{StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 10}}); err != nil {
		t.Fatalf("first AddSymbol: %v", err)
	}
	firstShard, err := first.Build(ctx)
	if err != nil {
		t.Fatalf("first Build: %v", err)
	}

	second := NewBuilder()
	second.SetMeta(store.ShardMeta{RepositoryID: "repo-2", Branch: "main", CreatedAt: time.Unix(1_700_000_100, 0).UTC(), Tier: store.T1})
	secondDoc, err := second.AddFile("helper.go", []byte("another hello helper\n"), "go")
	if err != nil {
		t.Fatalf("second AddFile: %v", err)
	}
	if _, err := second.AddEmbedding("helper.go", 1, 1, []float32{0.9, 0.1}); err != nil {
		t.Fatalf("second AddEmbedding: %v", err)
	}
	if err := second.AddSymbol(ctx, store.Symbol{Name: "HelloHelper", Kind: store.SymbolKindFunction, Path: "helper.go", Language: "go", Definition: true, Range: store.Span{StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 12}}); err != nil {
		t.Fatalf("second AddSymbol: %v", err)
	}
	secondShard, err := second.Build(ctx)
	if err != nil {
		t.Fatalf("second Build: %v", err)
	}

	searcher := NewSearcher(firstShard, secondShard)
	docs, err := searcher.SearchDocuments(ctx, "hello")
	if err != nil {
		t.Fatalf("SearchDocuments: %v", err)
	}
	if len(docs) != 2 {
		t.Fatalf("SearchDocuments len = %d, want 2", len(docs))
	}
	ids := []string{docs[0].ID, docs[1].ID}
	wantIDs := []string{firstDoc.ID, secondDoc.ID}
	if !sameStrings(ids, wantIDs) {
		t.Fatalf("SearchDocuments ids = %v, want %v", ids, wantIDs)
	}

	symbols, err := searcher.SearchSymbols(ctx, "hello")
	if err != nil {
		t.Fatalf("SearchSymbols: %v", err)
	}
	if len(symbols) != 2 {
		t.Fatalf("SearchSymbols len = %d, want 2", len(symbols))
	}

	vectorResults, err := searcher.SearchVectors(ctx, []float32{1, 0}, 1, store.DistanceMetricCosine)
	if err != nil {
		t.Fatalf("SearchVectors: %v", err)
	}
	if len(vectorResults) != 1 || vectorResults[0].Vector.Path != "helper.go" {
		t.Fatalf("SearchVectors got %#v, want helper.go top result", vectorResults)
	}

	triResults, err := searcher.SearchTrigrams(ctx, []store.Trigram{store.NewTrigram('h', 'e', 'l')})
	if err != nil {
		t.Fatalf("SearchTrigrams: %v", err)
	}
	if len(triResults) != 1 || triResults[0].DocumentID != secondDoc.ID {
		t.Fatalf("SearchTrigrams got %#v, want %q", triResults, secondDoc.ID)
	}
}

func sameStrings(got []string, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	got = append([]string(nil), got...)
	want = append([]string(nil), want...)
	for i := 1; i < len(got); i++ {
		for j := i; j > 0 && got[j] < got[j-1]; j-- {
			got[j], got[j-1] = got[j-1], got[j]
		}
	}
	for i := 1; i < len(want); i++ {
		for j := i; j > 0 && want[j] < want[j-1]; j-- {
			want[j], want[j-1] = want[j-1], want[j]
		}
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
