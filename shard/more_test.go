package shard

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SCKelemen/codesearch/store"
)

type fakeIndexShard struct {
	meta        store.ShardMeta
	docs        []store.Document
	trigrams    []store.PostingResult
	vectors     []store.VectorResult
	symbols     []store.Symbol
	documentErr error
	trigramErr  error
	vectorErr   error
	symbolErr   error
}

func (f fakeIndexShard) Meta() store.ShardMeta          { return f.meta }
func (f fakeIndexShard) Documents() store.DocumentStore { return nil }
func (f fakeIndexShard) Trigrams() store.TrigramStore   { return nil }
func (f fakeIndexShard) Vectors() store.VectorStore     { return nil }
func (f fakeIndexShard) Symbols() store.SymbolStore     { return nil }
func (f fakeIndexShard) MarshalBinary() ([]byte, error) { return nil, nil }
func (f fakeIndexShard) SearchDocuments(context.Context, string, ...store.SearchOption) ([]store.Document, error) {
	return append([]store.Document(nil), f.docs...), f.documentErr
}
func (f fakeIndexShard) SearchTrigrams(context.Context, []store.Trigram, ...store.SearchOption) ([]store.PostingResult, error) {
	return append([]store.PostingResult(nil), f.trigrams...), f.trigramErr
}
func (f fakeIndexShard) SearchVectors(context.Context, []float32, int, store.DistanceMetric, ...store.SearchOption) ([]store.VectorResult, error) {
	return append([]store.VectorResult(nil), f.vectors...), f.vectorErr
}
func (f fakeIndexShard) SearchSymbols(context.Context, string, ...store.SearchOption) ([]store.Symbol, error) {
	return append([]store.Symbol(nil), f.symbols...), f.symbolErr
}

func TestEmptyShardRoundTripAndOptionalSections(t *testing.T) {
	t.Parallel()

	builder := NewBuilder()
	built, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	raw, err := built.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary returned error: %v", err)
	}
	loaded, err := FromBytes(raw)
	if err != nil {
		t.Fatalf("FromBytes returned error: %v", err)
	}

	docs, next, err := loaded.Documents().List(context.Background())
	if err != nil {
		t.Fatalf("Documents().List returned error: %v", err)
	}
	if len(docs) != 0 || next != "" {
		t.Fatalf("Documents().List = (%v, %q), want empty", docs, next)
	}
	vectors, next, err := loaded.Vectors().List(context.Background())
	if err != nil {
		t.Fatalf("Vectors().List returned error: %v", err)
	}
	if len(vectors) != 0 || next != "" {
		t.Fatalf("Vectors().List = (%v, %q), want empty", vectors, next)
	}
	symbols, next, err := loaded.Symbols().List(context.Background())
	if err != nil {
		t.Fatalf("Symbols().List returned error: %v", err)
	}
	if len(symbols) != 0 || next != "" {
		t.Fatalf("Symbols().List = (%v, %q), want empty", symbols, next)
	}
	results, err := loaded.SearchVectors(context.Background(), []float32{1, 0}, 5, store.DistanceMetricCosine)
	if err != nil {
		t.Fatalf("SearchVectors returned error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("SearchVectors = %v, want empty", results)
	}
}

func TestShardCorruptDataAndHelperPaths(t *testing.T) {
	t.Parallel()

	t.Run("invalid shard bytes are rejected", func(t *testing.T) {
		t.Parallel()

		cases := []struct {
			name string
			raw  []byte
			want string
		}{
			{name: "too small", raw: []byte("tiny"), want: "shard too small"},
			{name: "invalid magic", raw: corruptRawWithVersionAndChecksum([]byte("BAD!")), want: "invalid magic"},
			{name: "unsupported version", raw: corruptVersionShard(99), want: "unsupported shard version 99"},
			{name: "checksum mismatch", raw: corruptChecksumShard(), want: errChecksumMismatch.Error()},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				t.Parallel()

				_, err := FromBytes(tc.raw)
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("FromBytes error = %v, want substring %q", err, tc.want)
				}
			})
		}
	})

	t.Run("corrupt section is reported lazily", func(t *testing.T) {
		t.Parallel()

		loaded := buildTestShard(t)
		raw, err := loaded.MarshalBinary()
		if err != nil {
			t.Fatalf("MarshalBinary returned error: %v", err)
		}
		header, err := decodeHeader(raw)
		if err != nil {
			t.Fatalf("decodeHeader returned error: %v", err)
		}
		mutated := append([]byte(nil), raw...)
		section := sectionBytes(mutated, header.Sections.Documents)
		copy(section, []byte("{bad json"))
		rewriteChecksum(mutated)

		corrupt, err := FromBytes(mutated)
		if err != nil {
			t.Fatalf("FromBytes returned error: %v", err)
		}
		_, _, err = corrupt.Documents().List(context.Background())
		if err == nil || !strings.Contains(err.Error(), "invalid character") {
			t.Fatalf("Documents().List error = %v, want JSON error", err)
		}
	})

	t.Run("open missing file", func(t *testing.T) {
		t.Parallel()

		_, err := Open(filepath.Join(t.TempDir(), "missing.cshr"))
		if err == nil || !strings.Contains(err.Error(), "open shard") {
			t.Fatalf("Open error = %v, want open shard", err)
		}
	})

	t.Run("helpers cover pagination and filters", func(t *testing.T) {
		t.Parallel()

		items, next, err := applyPage([]int{1, 2, 3}, "1", 1)
		if err != nil {
			t.Fatalf("applyPage returned error: %v", err)
		}
		if !reflect.DeepEqual(items, []int{2}) || next != "2" {
			t.Fatalf("applyPage = (%v, %q), want ([2], \"2\")", items, next)
		}
		if _, _, err := applyPage([]int{1, 2, 3}, "bad", 1); err == nil || !strings.Contains(err.Error(), "invalid cursor") {
			t.Fatalf("applyPage error = %v, want invalid cursor", err)
		}
		doc := store.Document{ID: "doc", RepositoryID: "repo", Branch: "main", Path: "src/main.go", Language: "go", Metadata: map[string]string{"team": "search"}}
		if !matchesDocumentFilter(doc, store.Filter{RepositoryID: "repo", Branch: "main", PathPrefix: "src/", Language: "go", Metadata: map[string]string{"team": "search"}}) {
			t.Fatal("matchesDocumentFilter returned false for matching filter")
		}
		if matchesDocumentFilter(doc, store.Filter{Metadata: map[string]string{"team": "other"}}) {
			t.Fatal("matchesDocumentFilter returned true for mismatched metadata")
		}
		if matchesMetadata(map[string]string{"a": "1"}, map[string]string{"a": "1", "b": "2"}) {
			t.Fatal("matchesMetadata returned true for missing key")
		}
	})
}

func TestShardViewsAndMultiShardSearcherEdgeCases(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	loaded := buildTestShard(t)

	if err := loaded.Documents().Put(ctx, store.Document{}); !errors.Is(err, errImmutableStore) {
		t.Fatalf("Documents().Put error = %v, want errImmutableStore", err)
	}
	if err := loaded.Trigrams().Put(ctx, store.PostingList{}); !errors.Is(err, errImmutableStore) {
		t.Fatalf("Trigrams().Put error = %v, want errImmutableStore", err)
	}
	if err := loaded.Vectors().Delete(ctx, "vec-1"); !errors.Is(err, errImmutableStore) {
		t.Fatalf("Vectors().Delete error = %v, want errImmutableStore", err)
	}
	if err := loaded.Symbols().PutReference(ctx, store.Reference{}); !errors.Is(err, errImmutableStore) {
		t.Fatalf("Symbols().PutReference error = %v, want errImmutableStore", err)
	}

	doc, err := loaded.Documents().Lookup(ctx, "repo:main:src/main.go", store.WithTier(store.T1), store.WithLanguage("go"))
	if err != nil {
		t.Fatalf("Documents().Lookup returned error: %v", err)
	}
	if doc == nil || doc.Path != "src/main.go" {
		t.Fatalf("Documents().Lookup = %#v, want src/main.go", doc)
	}
	miss, err := loaded.Documents().Lookup(ctx, "repo:main:src/main.go", store.WithTier(store.T2))
	if err != nil {
		t.Fatalf("Documents().Lookup tier mismatch returned error: %v", err)
	}
	if miss != nil {
		t.Fatalf("Documents().Lookup tier mismatch = %#v, want nil", miss)
	}
	docs, next, err := loaded.Documents().List(ctx, store.WithTier(store.T1), store.WithLimit(1))
	if err != nil {
		t.Fatalf("Documents().List returned error: %v", err)
	}
	if len(docs) != 1 || next != "1" {
		t.Fatalf("Documents().List = (%v, %q), want one result and next cursor", docs, next)
	}
	searchDocs, err := loaded.Documents().Search(ctx, "main", store.WithTier(store.T1))
	if err != nil {
		t.Fatalf("Documents().Search returned error: %v", err)
	}
	if len(searchDocs) == 0 || searchDocs[0].Path != "src/main.go" {
		t.Fatalf("Documents().Search = %v, want src/main.go first", searchDocs)
	}

	tri, err := loaded.Trigrams().Lookup(ctx, store.NewTrigram('m', 'a', 'i'), store.WithDocumentID("repo:main:src/main.go"), store.WithTier(store.T1))
	if err != nil {
		t.Fatalf("Trigrams().Lookup returned error: %v", err)
	}
	if tri == nil || !reflect.DeepEqual(tri.DocumentIDs, []string{"repo:main:src/main.go"}) {
		t.Fatalf("Trigrams().Lookup = %#v, want main doc only", tri)
	}
	lists, _, err := loaded.Trigrams().List(ctx, store.WithDocumentID("repo:main:src/main.go"), store.WithTier(store.T1))
	if err != nil {
		t.Fatalf("Trigrams().List returned error: %v", err)
	}
	if len(lists) == 0 {
		t.Fatal("Trigrams().List returned no posting lists")
	}
	matches, err := loaded.Trigrams().Search(ctx, []store.Trigram{store.NewTrigram('m', 'a', 'i')}, store.WithDocumentID("repo:main:src/main.go"), store.WithTier(store.T1))
	if err != nil {
		t.Fatalf("Trigrams().Search returned error: %v", err)
	}
	if len(matches) != 1 || matches[0].DocumentID != "repo:main:src/main.go" {
		t.Fatalf("Trigrams().Search = %v, want main doc", matches)
	}

	vector, err := loaded.Vectors().Lookup(ctx, "src/main.go:1:2", store.WithTier(store.T1))
	if err != nil {
		t.Fatalf("Vectors().Lookup returned error: %v", err)
	}
	if vector == nil || vector.Path != "src/main.go" {
		t.Fatalf("Vectors().Lookup = %#v, want src/main.go", vector)
	}
	vectorList, _, err := loaded.Vectors().List(ctx, store.WithTier(store.T1))
	if err != nil {
		t.Fatalf("Vectors().List returned error: %v", err)
	}
	if len(vectorList) != 2 {
		t.Fatalf("Vectors().List len = %d, want 2", len(vectorList))
	}
	vectorResults, err := loaded.Vectors().Search(ctx, []float32{1, 0}, 2, store.DistanceMetricCosine, store.WithTier(store.T1))
	if err != nil {
		t.Fatalf("Vectors().Search returned error: %v", err)
	}
	if len(vectorResults) == 0 || vectorResults[0].Vector.Path != "src/main.go" {
		t.Fatalf("Vectors().Search = %v, want src/main.go first", vectorResults)
	}

	symbol, err := loaded.Symbols().Lookup(ctx, "src/main.go:Main:1:1", store.WithTier(store.T1))
	if err != nil {
		t.Fatalf("Symbols().Lookup returned error: %v", err)
	}
	if symbol == nil || symbol.Name != "Main" {
		t.Fatalf("Symbols().Lookup = %#v, want Main", symbol)
	}
	symbols, _, err := loaded.Symbols().List(ctx, store.WithTier(store.T1))
	if err != nil {
		t.Fatalf("Symbols().List returned error: %v", err)
	}
	if len(symbols) != 1 {
		t.Fatalf("Symbols().List len = %d, want 1", len(symbols))
	}
	symbolMatches, err := loaded.Symbols().Search(ctx, "ma", store.WithTier(store.T1))
	if err != nil {
		t.Fatalf("Symbols().Search returned error: %v", err)
	}
	if len(symbolMatches) != 1 || symbolMatches[0].Name != "Main" {
		t.Fatalf("Symbols().Search = %v, want Main", symbolMatches)
	}
	references, _, err := loaded.Symbols().References(ctx, "src/main.go:Main:1:1", store.WithTier(store.T1), store.WithPathPrefix("src/"))
	if err != nil {
		t.Fatalf("Symbols().References returned error: %v", err)
	}
	if len(references) != 1 || references[0].Path != "src/helper.go" {
		t.Fatalf("Symbols().References = %v, want helper reference", references)
	}

	searcher := NewSearcher(
		fakeIndexShard{docs: []store.Document{{ID: "doc-b", Path: "b.go", Content: []byte("needle")}, {ID: "doc-a", Path: "a.go", Content: []byte("needle")}}, trigrams: []store.PostingResult{{DocumentID: "doc-a", MatchedTrigrams: 1, CandidateTrigrams: 2}}, vectors: []store.VectorResult{{Vector: store.StoredVector{ID: "vec-b"}, Score: 0.5, Distance: 0.5}}, symbols: []store.Symbol{{ID: "sym-a", Name: "Alpha"}}},
		fakeIndexShard{docs: []store.Document{{ID: "doc-a", Path: "a.go", Content: []byte("needle")}}, trigrams: []store.PostingResult{{DocumentID: "doc-a", MatchedTrigrams: 2, CandidateTrigrams: 3}, {DocumentID: "doc-b", MatchedTrigrams: 1, CandidateTrigrams: 3}}, vectors: []store.VectorResult{{Vector: store.StoredVector{ID: "vec-a"}, Score: 0.9, Distance: 0.1}}, symbols: []store.Symbol{{ID: "sym-a", Name: "Alpha"}, {ID: "sym-b", Name: "Beta"}}},
	)
	searchDocs, err = searcher.SearchDocuments(ctx, "needle", store.WithLimit(1))
	if err != nil {
		t.Fatalf("SearchDocuments returned error: %v", err)
	}
	if len(searchDocs) != 1 || searchDocs[0].ID != "doc-a" {
		t.Fatalf("SearchDocuments = %v, want deduped top doc-a", searchDocs)
	}
	triMatches, err := searcher.SearchTrigrams(ctx, []store.Trigram{store.NewTrigram('n', 'e', 'e')})
	if err != nil {
		t.Fatalf("SearchTrigrams returned error: %v", err)
	}
	if len(triMatches) != 2 || triMatches[0].DocumentID != "doc-a" || triMatches[0].MatchedTrigrams != 3 || triMatches[0].CandidateTrigrams != 3 {
		t.Fatalf("SearchTrigrams = %v, want aggregated doc-a first", triMatches)
	}
	vecMatches, err := searcher.SearchVectors(ctx, []float32{1, 0}, 2, store.DistanceMetricCosine)
	if err != nil {
		t.Fatalf("SearchVectors returned error: %v", err)
	}
	if len(vecMatches) != 2 || vecMatches[0].Vector.ID != "vec-a" || vecMatches[0].Rank != 1 || vecMatches[1].Rank != 2 {
		t.Fatalf("SearchVectors = %v, want sorted ranked vectors", vecMatches)
	}
	symbolResults, err := searcher.SearchSymbols(ctx, "a", store.WithLimit(2))
	if err != nil {
		t.Fatalf("SearchSymbols returned error: %v", err)
	}
	if len(symbolResults) != 2 || symbolResults[0].Name != "Alpha" {
		t.Fatalf("SearchSymbols = %v, want Alpha first", symbolResults)
	}

	errSearcher := NewSearcher(fakeIndexShard{documentErr: errors.New("documents failed")})
	_, err = errSearcher.SearchDocuments(ctx, "needle")
	if err == nil || !strings.Contains(err.Error(), "documents failed") {
		t.Fatalf("SearchDocuments error = %v, want documents failed", err)
	}
	canceled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = errSearcher.SearchSymbols(canceled, "needle")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("SearchSymbols error = %v, want context canceled", err)
	}
}

func TestLargeShardMmapConcurrentAccess(t *testing.T) {
	t.Parallel()

	builder := NewBuilder()
	builder.SetMeta(store.ShardMeta{RepositoryID: "repo", Branch: "main", CreatedAt: time.Unix(1_700_000_000, 0).UTC(), Tier: store.T1})
	for i := 0; i < 80; i++ {
		path := fmt.Sprintf("src/file-%03d.go", i)
		content := []byte(fmt.Sprintf("package main\n\nfunc Item%d() string {\n\treturn \"needle %d\"\n}\n", i, i))
		if _, err := builder.AddFile(path, content, "go"); err != nil {
			t.Fatalf("AddFile(%s) returned error: %v", path, err)
		}
		if _, err := builder.AddEmbedding(path, 3, 4, []float32{float32(i + 1), 1}); err != nil {
			t.Fatalf("AddEmbedding(%s) returned error: %v", path, err)
		}
	}
	built, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	raw, err := built.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary returned error: %v", err)
	}
	path := filepath.Join(t.TempDir(), "large.cshr")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatalf("WriteFile returned error: %v", err)
	}
	loaded, err := Open(path)
	if err != nil {
		t.Fatalf("Open returned error: %v", err)
	}
	defer loaded.Close()

	errCh := make(chan error, 32)
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			ctx := context.Background()
			docs, err := loaded.SearchDocuments(ctx, "needle", store.WithLimit(5), store.WithTier(store.T1))
			if err != nil {
				errCh <- fmt.Errorf("SearchDocuments(%d): %w", index, err)
				return
			}
			if len(docs) == 0 {
				errCh <- fmt.Errorf("SearchDocuments(%d) returned no results", index)
				return
			}
			vectorResults, err := loaded.SearchVectors(ctx, []float32{80, 1}, 3, store.DistanceMetricCosine, store.WithTier(store.T1))
			if err != nil {
				errCh <- fmt.Errorf("SearchVectors(%d): %w", index, err)
				return
			}
			if len(vectorResults) == 0 {
				errCh <- fmt.Errorf("SearchVectors(%d) returned no results", index)
				return
			}
			lookup, err := loaded.Documents().Lookup(ctx, "repo:main:src/file-000.go", store.WithTier(store.T1))
			if err != nil {
				errCh <- fmt.Errorf("Lookup(%d): %w", index, err)
				return
			}
			if lookup == nil {
				errCh <- fmt.Errorf("Lookup(%d) returned nil", index)
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

func buildTestShard(t *testing.T) *Shard {
	t.Helper()

	builder := NewBuilder()
	builder.SetMeta(store.ShardMeta{RepositoryID: "repo", Branch: "main", CreatedAt: time.Unix(1_700_000_000, 0).UTC(), Tier: store.T1})
	mainDoc, err := builder.AddFile("src/main.go", []byte("package main\n\nfunc Main() string {\n\treturn \"main needle\"\n}\n"), "go")
	if err != nil {
		t.Fatalf("AddFile main returned error: %v", err)
	}
	helperDoc, err := builder.AddFile("src/helper.go", []byte("package main\n\nfunc Helper() string {\n\treturn \"helper\"\n}\n"), "go")
	if err != nil {
		t.Fatalf("AddFile helper returned error: %v", err)
	}
	if err := builder.AddDocument(context.Background(), store.Document{ID: mainDoc.ID, RepositoryID: mainDoc.RepositoryID, Branch: mainDoc.Branch, Path: mainDoc.Path, Language: mainDoc.Language, Content: mainDoc.Content, Metadata: map[string]string{"team": "search"}, CreatedAt: mainDoc.CreatedAt, UpdatedAt: mainDoc.UpdatedAt}); err != nil {
		t.Fatalf("AddDocument main returned error: %v", err)
	}
	if _, err := builder.AddEmbedding("src/main.go", 1, 2, []float32{1, 0}); err != nil {
		t.Fatalf("AddEmbedding main returned error: %v", err)
	}
	if _, err := builder.AddEmbedding("src/helper.go", 1, 2, []float32{0.5, 0.5}); err != nil {
		t.Fatalf("AddEmbedding helper returned error: %v", err)
	}
	if err := builder.AddSymbol(context.Background(), store.Symbol{Name: "Main", Kind: store.SymbolKindFunction, Path: "src/main.go", Language: "go", Definition: true, Range: store.Span{StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 5}}); err != nil {
		t.Fatalf("AddSymbol returned error: %v", err)
	}
	if err := builder.AddReference(context.Background(), store.Reference{SymbolID: "src/main.go:Main:1:1", DocumentID: helperDoc.ID, Path: helperDoc.Path, Range: store.Span{StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 4}}); err != nil {
		t.Fatalf("AddReference returned error: %v", err)
	}
	shard, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Build returned error: %v", err)
	}
	raw, err := shard.MarshalBinary()
	if err != nil {
		t.Fatalf("MarshalBinary returned error: %v", err)
	}
	loaded, err := FromBytes(raw)
	if err != nil {
		t.Fatalf("FromBytes returned error: %v", err)
	}
	return loaded
}

func corruptRawWithVersionAndChecksum(magic []byte) []byte {
	raw := make([]byte, 16)
	copy(raw[:4], magic)
	binary.LittleEndian.PutUint32(raw[4:8], Version)
	binary.LittleEndian.PutUint32(raw[8:12], 0)
	rewriteChecksum(raw)
	return raw
}

func corruptVersionShard(version uint32) []byte {
	raw := make([]byte, 16)
	copy(raw[:4], []byte(Magic))
	binary.LittleEndian.PutUint32(raw[4:8], version)
	binary.LittleEndian.PutUint32(raw[8:12], 0)
	rewriteChecksum(raw)
	return raw
}

func corruptChecksumShard() []byte {
	raw := corruptVersionShard(Version)
	raw[len(raw)-1]++
	return raw
}

func rewriteChecksum(raw []byte) {
	footerStart := len(raw) - 4
	binary.LittleEndian.PutUint32(raw[footerStart:], crc32.ChecksumIEEE(raw[:footerStart]))
}
