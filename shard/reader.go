package shard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/SCKelemen/codesearch/store"
	"github.com/SCKelemen/codesearch/store/memory"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
)

var _ store.IndexShard = (*Shard)(nil)

var errImmutableStore = errors.New("shard stores are immutable")

// Shard is an immutable, searchable binary shard.
type Shard struct {
	meta   store.ShardMeta
	raw    []byte
	header fileHeader

	file *os.File
	mmap []byte

	docsOnce sync.Once
	docs     *memory.DocumentStore
	docsErr  error

	trigramsOnce sync.Once
	trigrams     *memory.TrigramStore
	trigramsErr  error

	vectorsOnce sync.Once
	vectors     *memory.VectorStore
	vectorsErr  error

	symbolsOnce sync.Once
	symbols     *memory.SymbolStore
	symbolsErr  error
}

func newLoadedShard(meta store.ShardMeta, raw []byte, docs []store.Document, trigrams []store.PostingList, vectors []store.StoredVector, symbols []store.Symbol, refs []store.Reference) *Shard {
	docStore := memory.NewDocumentStore()
	trigramStore := memory.NewTrigramStore()
	vectorStore := memory.NewVectorStore()
	symbolStore := memory.NewSymbolStore()
	for _, doc := range docs {
		_ = docStore.Put(context.Background(), doc)
	}
	for _, list := range trigrams {
		_ = trigramStore.Put(context.Background(), list)
	}
	for _, vector := range vectors {
		_ = vectorStore.Put(context.Background(), vector)
	}
	for _, symbol := range symbols {
		_ = symbolStore.Put(context.Background(), symbol)
	}
	for _, ref := range refs {
		_ = symbolStore.PutReference(context.Background(), ref)
	}
	return &Shard{meta: meta, raw: raw, docs: docStore, trigrams: trigramStore, vectors: vectorStore, symbols: symbolStore}
}

// Open opens a shard from disk using read-only memory mapping.
func Open(path string) (*Shard, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open shard %s: %w", path, err)
	}
	stat, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("stat shard %s: %w", path, err)
	}
	mapped, err := syscall.Mmap(int(f.Fd()), 0, int(stat.Size()), syscall.PROT_READ, syscall.MAP_PRIVATE)
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("mmap shard %s: %w", path, err)
	}
	shard, err := fromRaw(mapped)
	if err != nil {
		_ = syscall.Munmap(mapped)
		_ = f.Close()
		return nil, err
	}
	shard.file = f
	shard.mmap = mapped
	return shard, nil
}

// FromBytes opens a shard from an in-memory byte slice.
func FromBytes(raw []byte) (*Shard, error) {
	return fromRaw(raw)
}

func fromRaw(raw []byte) (*Shard, error) {
	header, err := decodeHeader(raw)
	if err != nil {
		return nil, err
	}
	return &Shard{meta: header.Meta, raw: raw, header: header}, nil
}

func (s *Shard) Close() error {
	var errs []error
	if s.mmap != nil {
		if err := syscall.Munmap(s.mmap); err != nil {
			errs = append(errs, err)
		}
		s.mmap = nil
	}
	if s.file != nil {
		if err := s.file.Close(); err != nil {
			errs = append(errs, err)
		}
		s.file = nil
	}
	return errors.Join(errs...)
}

func (s *Shard) Meta() store.ShardMeta { return s.meta }

func (s *Shard) Documents() store.DocumentStore { return documentView{s} }
func (s *Shard) Trigrams() store.TrigramStore   { return trigramView{s} }
func (s *Shard) Vectors() store.VectorStore     { return vectorView{s} }
func (s *Shard) Symbols() store.SymbolStore     { return symbolView{s} }

func (s *Shard) MarshalBinary() ([]byte, error) {
	out := make([]byte, len(s.raw))
	copy(out, s.raw)
	return out, nil
}

func (s *Shard) SearchDocuments(ctx context.Context, query string, opts ...store.SearchOption) ([]store.Document, error) {
	return s.Documents().Search(ctx, query, opts...)
}

func (s *Shard) SearchTrigrams(ctx context.Context, trigrams []store.Trigram, opts ...store.SearchOption) ([]store.PostingResult, error) {
	return s.Trigrams().Search(ctx, trigrams, opts...)
}

func (s *Shard) SearchVectors(ctx context.Context, query []float32, k int, metric store.DistanceMetric, opts ...store.SearchOption) ([]store.VectorResult, error) {
	return s.Vectors().Search(ctx, query, k, metric, opts...)
}

func (s *Shard) SearchSymbols(ctx context.Context, query string, opts ...store.SearchOption) ([]store.Symbol, error) {
	return s.Symbols().Search(ctx, query, opts...)
}

func (s *Shard) loadDocuments() (*memory.DocumentStore, error) {
	s.docsOnce.Do(func() {
		if s.docs != nil {
			return
		}
		var docs []store.Document
		s.docsErr = json.Unmarshal(sectionBytes(s.raw, s.header.Sections.Documents), &docs)
		if s.docsErr == nil {
			s.docs = memory.NewDocumentStore()
			for _, doc := range docs {
				_ = s.docs.Put(context.Background(), doc)
			}
		}
	})
	return s.docs, s.docsErr
}

func (s *Shard) loadTrigrams() (*memory.TrigramStore, error) {
	s.trigramsOnce.Do(func() {
		if s.trigrams != nil {
			return
		}
		var lists []store.PostingList
		s.trigramsErr = json.Unmarshal(sectionBytes(s.raw, s.header.Sections.Trigrams), &lists)
		if s.trigramsErr == nil {
			s.trigrams = memory.NewTrigramStore()
			for _, list := range lists {
				_ = s.trigrams.Put(context.Background(), list)
			}
		}
	})
	return s.trigrams, s.trigramsErr
}

func (s *Shard) loadVectors() (*memory.VectorStore, error) {
	s.vectorsOnce.Do(func() {
		if s.vectors != nil {
			return
		}
		var vectors []store.StoredVector
		s.vectorsErr = json.Unmarshal(sectionBytes(s.raw, s.header.Sections.Vectors), &vectors)
		if s.vectorsErr == nil {
			s.vectors = memory.NewVectorStore()
			for _, vector := range vectors {
				_ = s.vectors.Put(context.Background(), vector)
			}
		}
	})
	return s.vectors, s.vectorsErr
}

func (s *Shard) loadSymbols() (*memory.SymbolStore, error) {
	s.symbolsOnce.Do(func() {
		if s.symbols != nil {
			return
		}
		var payload struct {
			Symbols    []store.Symbol    `json:"symbols"`
			References []store.Reference `json:"references"`
		}
		s.symbolsErr = json.Unmarshal(sectionBytes(s.raw, s.header.Sections.Symbols), &payload)
		if s.symbolsErr == nil {
			s.symbols = memory.NewSymbolStore()
			for _, symbol := range payload.Symbols {
				_ = s.symbols.Put(context.Background(), symbol)
			}
			for _, ref := range payload.References {
				_ = s.symbols.PutReference(context.Background(), ref)
			}
		}
	})
	return s.symbols, s.symbolsErr
}

type documentView struct{ shard *Shard }
type trigramView struct{ shard *Shard }
type vectorView struct{ shard *Shard }
type symbolView struct{ shard *Shard }

func (documentView) Put(context.Context, store.Document) error { return errImmutableStore }
func (documentView) Delete(context.Context, string) error      { return errImmutableStore }

func (v documentView) Lookup(ctx context.Context, id string, opts ...store.LookupOption) (*store.Document, error) {
	if !v.allowLookup(opts...) {
		return nil, nil
	}
	docs, err := v.shard.loadDocuments()
	if err != nil {
		return nil, err
	}
	return docs.Lookup(ctx, id, opts...)
}

func (v documentView) List(ctx context.Context, opts ...store.ListOption) ([]store.Document, string, error) {
	if !v.allowList(opts...) {
		return nil, "", nil
	}
	docs, err := v.shard.loadDocuments()
	if err != nil {
		return nil, "", err
	}
	return docs.List(ctx, opts...)
}

func (v documentView) Search(ctx context.Context, query string, opts ...store.SearchOption) ([]store.Document, error) {
	if !v.allowSearch(opts...) {
		return nil, nil
	}
	docs, err := v.shard.loadDocuments()
	if err != nil {
		return nil, err
	}
	return docs.Search(ctx, query, opts...)
}

func (v documentView) allowLookup(opts ...store.LookupOption) bool {
	return tierAllowed(v.shard.meta, store.ResolveLookupOptions(opts...).Filter)
}
func (v documentView) allowList(opts ...store.ListOption) bool {
	return tierAllowed(v.shard.meta, store.ResolveListOptions(opts...).Filter)
}
func (v documentView) allowSearch(opts ...store.SearchOption) bool {
	return tierAllowed(v.shard.meta, store.ResolveSearchOptions(opts...).Filter)
}

func (trigramView) Put(context.Context, store.PostingList) error { return errImmutableStore }
func (trigramView) Delete(context.Context, store.Trigram) error  { return errImmutableStore }

func (v trigramView) Lookup(ctx context.Context, trigram store.Trigram, opts ...store.LookupOption) (*store.PostingList, error) {
	if !tierAllowed(v.shard.meta, store.ResolveLookupOptions(opts...).Filter) {
		return nil, nil
	}
	lists, err := v.shard.loadTrigrams()
	if err != nil {
		return nil, err
	}
	list, err := lists.Lookup(ctx, trigram)
	if err != nil || list == nil {
		return list, err
	}
	filter := store.ResolveLookupOptions(opts...).Filter
	docIDs, err := v.filterDocumentIDs(ctx, list.DocumentIDs, filter)
	if err != nil {
		return nil, err
	}
	list.DocumentIDs = docIDs
	if filter.DocumentID != "" && len(list.DocumentIDs) == 0 {
		return nil, nil
	}
	return list, nil
}

func (v trigramView) List(ctx context.Context, opts ...store.ListOption) ([]store.PostingList, string, error) {
	resolved := store.ResolveListOptions(opts...)
	if !tierAllowed(v.shard.meta, resolved.Filter) {
		return nil, "", nil
	}
	lists, err := v.shard.loadTrigrams()
	if err != nil {
		return nil, "", err
	}
	all, _, err := lists.List(ctx)
	if err != nil {
		return nil, "", err
	}
	filtered := make([]store.PostingList, 0, len(all))
	for _, list := range all {
		docIDs, err := v.filterDocumentIDs(ctx, list.DocumentIDs, resolved.Filter)
		if err != nil {
			return nil, "", err
		}
		list.DocumentIDs = docIDs
		if resolved.Filter.DocumentID != "" && len(docIDs) == 0 {
			continue
		}
		filtered = append(filtered, list)
	}
	return applyPage(filtered, resolved.Cursor, resolved.Limit)
}

func (v trigramView) Search(ctx context.Context, trigrams []store.Trigram, opts ...store.SearchOption) ([]store.PostingResult, error) {
	resolved := store.ResolveSearchOptions(opts...)
	if !tierAllowed(v.shard.meta, resolved.Filter) {
		return nil, nil
	}
	lists, err := v.shard.loadTrigrams()
	if err != nil {
		return nil, err
	}
	results, err := lists.Search(ctx, trigrams)
	if err != nil {
		return nil, err
	}
	filtered := results[:0]
	for _, result := range results {
		doc, err := v.lookupDocument(ctx, result.DocumentID)
		if err != nil {
			return nil, err
		}
		if doc != nil && matchesDocumentFilter(*doc, resolved.Filter) {
			filtered = append(filtered, result)
		}
	}
	if resolved.Limit > 0 && len(filtered) > resolved.Limit {
		filtered = filtered[:resolved.Limit]
	}
	return append([]store.PostingResult(nil), filtered...), nil
}

func (v trigramView) lookupDocument(ctx context.Context, documentID string) (*store.Document, error) {
	docs, err := v.shard.loadDocuments()
	if err != nil {
		return nil, err
	}
	return docs.Lookup(ctx, documentID)
}

func (v trigramView) filterDocumentIDs(ctx context.Context, ids []string, filter store.Filter) ([]string, error) {
	filtered := make([]string, 0, len(ids))
	for _, id := range ids {
		doc, err := v.lookupDocument(ctx, id)
		if err != nil {
			return nil, err
		}
		if doc != nil && matchesDocumentFilter(*doc, filter) {
			filtered = append(filtered, id)
		}
	}
	return filtered, nil
}

func (vectorView) Put(context.Context, store.StoredVector) error { return errImmutableStore }
func (vectorView) Delete(context.Context, string) error          { return errImmutableStore }

func (v vectorView) Lookup(ctx context.Context, id string, opts ...store.LookupOption) (*store.StoredVector, error) {
	if !tierAllowed(v.shard.meta, store.ResolveLookupOptions(opts...).Filter) {
		return nil, nil
	}
	vectors, err := v.shard.loadVectors()
	if err != nil {
		return nil, err
	}
	return vectors.Lookup(ctx, id, opts...)
}

func (v vectorView) List(ctx context.Context, opts ...store.ListOption) ([]store.StoredVector, string, error) {
	resolved := store.ResolveListOptions(opts...)
	if !tierAllowed(v.shard.meta, resolved.Filter) {
		return nil, "", nil
	}
	vectors, err := v.shard.loadVectors()
	if err != nil {
		return nil, "", err
	}
	return vectors.List(ctx, opts...)
}

func (v vectorView) Search(ctx context.Context, query []float32, k int, metric store.DistanceMetric, opts ...store.SearchOption) ([]store.VectorResult, error) {
	resolved := store.ResolveSearchOptions(opts...)
	if !tierAllowed(v.shard.meta, resolved.Filter) {
		return nil, nil
	}
	vectors, err := v.shard.loadVectors()
	if err != nil {
		return nil, err
	}
	return vectors.Search(ctx, query, k, metric, opts...)
}

func (symbolView) Put(context.Context, store.Symbol) error             { return errImmutableStore }
func (symbolView) PutReference(context.Context, store.Reference) error { return errImmutableStore }
func (symbolView) Delete(context.Context, string) error                { return errImmutableStore }

func (v symbolView) Lookup(ctx context.Context, id string, opts ...store.LookupOption) (*store.Symbol, error) {
	if !tierAllowed(v.shard.meta, store.ResolveLookupOptions(opts...).Filter) {
		return nil, nil
	}
	symbols, err := v.shard.loadSymbols()
	if err != nil {
		return nil, err
	}
	return symbols.Lookup(ctx, id, opts...)
}

func (v symbolView) List(ctx context.Context, opts ...store.ListOption) ([]store.Symbol, string, error) {
	resolved := store.ResolveListOptions(opts...)
	if !tierAllowed(v.shard.meta, resolved.Filter) {
		return nil, "", nil
	}
	symbols, err := v.shard.loadSymbols()
	if err != nil {
		return nil, "", err
	}
	return symbols.List(ctx, opts...)
}

func (v symbolView) Search(ctx context.Context, query string, opts ...store.SearchOption) ([]store.Symbol, error) {
	resolved := store.ResolveSearchOptions(opts...)
	if !tierAllowed(v.shard.meta, resolved.Filter) {
		return nil, nil
	}
	symbols, err := v.shard.loadSymbols()
	if err != nil {
		return nil, err
	}
	return symbols.Search(ctx, query, opts...)
}

func (v symbolView) References(ctx context.Context, symbolID string, opts ...store.ListOption) ([]store.Reference, string, error) {
	resolved := store.ResolveListOptions(opts...)
	if !tierAllowed(v.shard.meta, resolved.Filter) {
		return nil, "", nil
	}
	symbols, err := v.shard.loadSymbols()
	if err != nil {
		return nil, "", err
	}
	return symbols.References(ctx, symbolID, opts...)
}

func tierAllowed(meta store.ShardMeta, filter store.Filter) bool {
	return filter.Tier == "" || filter.Tier == meta.Tier
}

func matchesDocumentFilter(doc store.Document, filter store.Filter) bool {
	if filter.RepositoryID != "" && doc.RepositoryID != filter.RepositoryID {
		return false
	}
	if filter.Branch != "" && doc.Branch != filter.Branch {
		return false
	}
	if filter.PathPrefix != "" && !strings.HasPrefix(doc.Path, filter.PathPrefix) {
		return false
	}
	if filter.Language != "" && doc.Language != filter.Language {
		return false
	}
	if filter.DocumentID != "" && doc.ID != filter.DocumentID {
		return false
	}
	return matchesMetadata(doc.Metadata, filter.Metadata)
}

func matchesMetadata(actual, required map[string]string) bool {
	for key, value := range required {
		if actual[key] != value {
			return false
		}
	}
	return true
}

func applyPage[T any](items []T, cursor string, limit int) ([]T, string, error) {
	start, err := parseCursor(cursor)
	if err != nil {
		return nil, "", err
	}
	if start > len(items) {
		start = len(items)
	}
	end := len(items)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	page := append([]T(nil), items[start:end]...)
	next := ""
	if end < len(items) {
		next = strconv.Itoa(end)
	}
	return page, next, nil
}

func parseCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(cursor)
	if err != nil || offset < 0 {
		return 0, fmt.Errorf("invalid cursor %q", cursor)
	}
	return offset, nil
}
