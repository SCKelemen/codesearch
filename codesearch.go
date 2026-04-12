package codesearch

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/SCKelemen/codesearch/embedding"
	"github.com/SCKelemen/codesearch/hybrid"
	"github.com/SCKelemen/codesearch/linguist"
	"github.com/SCKelemen/codesearch/store"
	filestore "github.com/SCKelemen/codesearch/store/file"
	memorystore "github.com/SCKelemen/codesearch/store/memory"
	"github.com/SCKelemen/codesearch/trigram"
)

const defaultSearchLimit = 100

type flusher interface {
	Flush() error
}

type closer interface {
	Close() error
}

// Engine is the top-level codesearch API.
type Engine struct {
	Documents        store.DocumentStore
	Trigrams         store.TrigramStore
	Vectors          store.VectorStore
	Symbols          store.SymbolStore
	Embedder         embedding.Embedder
	LexicalSearcher  hybrid.BackendSearcher
	SemanticSearcher hybrid.BackendSearcher
	HybridSearcher   *hybrid.HybridSearcher

	hybridEnabled bool
	flushers      []flusher
	closers       []closer
	initErr       error
}

// New creates a new engine with in-memory stores by default.
func New(opts ...Option) *Engine {
	engine, err := newEngine("", opts...)
	if err != nil {
		return &Engine{initErr: err}
	}
	return engine
}

// Open opens or creates a file-backed index rooted at dir.
func Open(dir string, opts ...Option) (*Engine, error) {
	return newEngine(dir, append([]Option{WithFileStore(dir)}, opts...)...)
}

func newEngine(defaultDir string, opts ...Option) (*Engine, error) {
	cfg := engineConfig{hybridSearch: true}
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if defaultDir != "" && cfg.storeDir == "" {
		cfg.storeDir = defaultDir
		cfg.useFileStore = true
	}

	engine := &Engine{
		Embedder:      cfg.embedder,
		hybridEnabled: cfg.hybridSearch,
	}

	if cfg.documentStore != nil {
		engine.Documents = cfg.documentStore
	} else if cfg.useFileStore {
		docStore, err := filestore.NewDocumentStore(filepath.Join(cfg.storeDir, "documents"))
		if err != nil {
			return nil, err
		}
		engine.Documents = docStore
		engine.flushers = append(engine.flushers, docStore)
		engine.closers = append(engine.closers, docStore)
	} else {
		engine.Documents = memorystore.NewDocumentStore()
	}

	if cfg.trigramStore != nil {
		engine.Trigrams = cfg.trigramStore
	} else if cfg.useFileStore {
		trigramStore, err := filestore.NewTrigramStore(filepath.Join(cfg.storeDir, "trigrams"))
		if err != nil {
			return nil, err
		}
		engine.Trigrams = trigramStore
		engine.flushers = append(engine.flushers, trigramStore)
		engine.closers = append(engine.closers, trigramStore)
	} else {
		engine.Trigrams = memorystore.NewTrigramStore()
	}

	if cfg.vectorStore != nil {
		engine.Vectors = cfg.vectorStore
	} else if cfg.useFileStore {
		vectorStore, err := filestore.NewVectorStore(filepath.Join(cfg.storeDir, "vectors"))
		if err != nil {
			return nil, err
		}
		engine.Vectors = vectorStore
		engine.flushers = append(engine.flushers, vectorStore)
		engine.closers = append(engine.closers, vectorStore)
	} else {
		engine.Vectors = memorystore.NewVectorStore()
	}

	if cfg.symbolStore != nil {
		engine.Symbols = cfg.symbolStore
	} else if cfg.useFileStore {
		symbolStore, err := filestore.NewSymbolStore(filepath.Join(cfg.storeDir, "symbols"))
		if err != nil {
			return nil, err
		}
		engine.Symbols = symbolStore
		engine.flushers = append(engine.flushers, symbolStore)
		engine.closers = append(engine.closers, symbolStore)
	} else {
		engine.Symbols = memorystore.NewSymbolStore()
	}

	engine.LexicalSearcher = lexicalBackend{engine: engine}
	if engine.Embedder != nil && engine.Vectors != nil {
		engine.SemanticSearcher = semanticBackend{engine: engine}
		engine.HybridSearcher = hybrid.NewHybridSearcher(engine.LexicalSearcher, engine.SemanticSearcher, hybrid.DefaultSearcherConfig())
	}

	return engine, nil
}

// Index indexes a file or every file under a directory.
func (e *Engine) Index(ctx context.Context, path string, opts ...IndexOption) error {
	if err := e.ready(); err != nil {
		return err
	}

	indexOpts := resolveIndexOptions(opts...)
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return e.indexDocument(ctx, path, content, indexOpts)
	}

	return filepath.WalkDir(path, func(current string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		content, err := os.ReadFile(current)
		if err != nil {
			return err
		}
		return e.indexDocument(ctx, current, content, indexOpts)
	})
}

// IndexFile indexes a single file.
func (e *Engine) IndexFile(ctx context.Context, path string, content []byte) error {
	if err := e.ready(); err != nil {
		return err
	}
	return e.indexDocument(ctx, path, content, resolveIndexOptions())
}

func (e *Engine) indexDocument(ctx context.Context, path string, content []byte, opts indexOptions) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	now := time.Now().UTC()
	documentID := cleanPath(path)
	existing, err := e.Documents.Lookup(ctx, documentID)
	if err != nil {
		return err
	}
	createdAt := now
	if existing != nil && !existing.CreatedAt.IsZero() {
		createdAt = existing.CreatedAt
	}
	if err := e.removeDocumentPostings(ctx, documentID); err != nil {
		return err
	}
	if err := e.Vectors.Delete(ctx, documentID); err != nil {
		return err
	}

	language := opts.language
	if language == "" {
		language = detectLanguage(path)
	}

	doc := store.Document{
		ID:        documentID,
		Path:      documentID,
		Language:  language,
		Content:   append([]byte(nil), content...),
		Size:      int64(len(content)),
		Checksum:  checksum(content),
		CreatedAt: createdAt,
		UpdatedAt: now,
	}
	if err := e.Documents.Put(ctx, doc); err != nil {
		return err
	}

	for _, tri := range trigram.Extract(content) {
		postingTrigram := store.NewTrigram(tri[0], tri[1], tri[2])
		list, err := e.Trigrams.Lookup(ctx, postingTrigram)
		if err != nil {
			return err
		}
		postingList := store.PostingList{Trigram: postingTrigram, DocumentIDs: []string{documentID}}
		if list != nil {
			postingList.DocumentIDs = append(append([]string(nil), list.DocumentIDs...), documentID)
		}
		if err := e.Trigrams.Put(ctx, postingList); err != nil {
			return err
		}
	}

	if e.Embedder != nil && opts.embeddings {
		vectors, err := e.Embedder.Embed(ctx, []string{string(content)})
		if err != nil {
			return err
		}
		if len(vectors) > 0 {
			if err := e.Vectors.Put(ctx, store.StoredVector{
				ID:         documentID,
				DocumentID: documentID,
				Path:       documentID,
				Model:      e.Embedder.Model(),
				Values:     append([]float32(nil), vectors[0]...),
				CreatedAt:  createdAt,
				UpdatedAt:  now,
			}); err != nil {
				return err
			}
		}
	}

	return e.flush()
}

// Search executes lexical, semantic, or hybrid search.
func (e *Engine) Search(ctx context.Context, query string, opts ...SearchOption) ([]Result, error) {
	if err := e.ready(); err != nil {
		return nil, err
	}

	searchOpts := resolveSearchOptions(opts...)
	mode := searchOpts.mode
	if mode == "" {
		if e.HybridSearcher != nil && e.hybridEnabled {
			mode = hybrid.Hybrid
		} else {
			mode = hybrid.LexicalOnly
		}
	}
	if e.Embedder == nil {
		if mode == hybrid.SemanticOnly {
			return nil, errors.New("semantic search requires an embedder")
		}
		mode = hybrid.LexicalOnly
	}
	if !e.hybridEnabled && mode == hybrid.Hybrid {
		mode = hybrid.LexicalOnly
	}

	if mode == hybrid.LexicalOnly || e.HybridSearcher == nil {
		hits, err := e.lexicalSearch(ctx, query, searchOpts.limit)
		if err != nil {
			return nil, err
		}
		return hitsToResults(hits), nil
	}

	queryVector, err := e.embedQuery(ctx, query)
	if err != nil {
		return nil, err
	}
	if mode == hybrid.SemanticOnly {
		hits, err := e.semanticSearch(ctx, queryVector, searchOpts.limit)
		if err != nil {
			return nil, err
		}
		return hitsToResults(hits), nil
	}

	fused, err := e.HybridSearcher.Search(ctx, hybrid.SearchRequest{
		Query:      query,
		Vector:     queryVector,
		MaxResults: searchOpts.limit,
		Mode:       mode,
	})
	if err != nil {
		return nil, err
	}
	lexicalHits, err := e.lexicalSearch(ctx, query, searchOpts.limit)
	if err != nil {
		return nil, err
	}
	semanticHits, err := e.semanticSearch(ctx, queryVector, searchOpts.limit)
	if err != nil {
		return nil, err
	}
	return e.fusedResults(ctx, fused, lexicalHits, semanticHits)
}

// Close flushes and closes any managed stores.
func (e *Engine) Close() error {
	var errs []error
	if err := e.flush(); err != nil {
		errs = append(errs, err)
	}
	for _, c := range e.closers {
		if c == nil {
			continue
		}
		if err := c.Close(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (e *Engine) flush() error {
	var errs []error
	for _, f := range e.flushers {
		if f == nil {
			continue
		}
		if err := f.Flush(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (e *Engine) ready() error {
	if e == nil {
		return errors.New("codesearch engine is nil")
	}
	if e.initErr != nil {
		return e.initErr
	}
	if e.Documents == nil || e.Trigrams == nil || e.Vectors == nil || e.Symbols == nil {
		return errors.New("codesearch engine is not fully configured")
	}
	return nil
}

type searchHit struct {
	documentID string
	path       string
	content    string
	snippet    string
	line       int
	score      float64
	matches    []Match
}

type lexicalBackend struct {
	engine *Engine
}

func (b lexicalBackend) Search(ctx context.Context, req hybrid.SearchRequest) ([]hybrid.SearchResult, error) {
	hits, err := b.engine.lexicalSearch(ctx, req.Query, req.MaxResults)
	if err != nil {
		return nil, err
	}
	results := make([]hybrid.SearchResult, 0, len(hits))
	for _, hit := range hits {
		results = append(results, hybrid.SearchResult{DocumentID: hit.documentID, Score: hit.score, Snippet: hit.snippet})
	}
	return results, nil
}

type semanticBackend struct {
	engine *Engine
}

func (b semanticBackend) Search(ctx context.Context, req hybrid.SearchRequest) ([]hybrid.SearchResult, error) {
	hits, err := b.engine.semanticSearch(ctx, req.Vector, req.MaxResults)
	if err != nil {
		return nil, err
	}
	results := make([]hybrid.SearchResult, 0, len(hits))
	for _, hit := range hits {
		results = append(results, hybrid.SearchResult{DocumentID: hit.documentID, Score: hit.score, Snippet: hit.snippet})
	}
	return results, nil
}

func (e *Engine) lexicalSearch(ctx context.Context, query string, limit int) ([]searchHit, error) {
	if limit <= 0 {
		limit = defaultSearchLimit
	}
	plan, err := trigram.BuildQueryPlan(query)
	if err != nil {
		if errors.Is(err, trigram.ErrNoExtractableTrigrams) {
			return e.documentSearch(ctx, query, limit)
		}
		return nil, err
	}

	trigramsToSearch := make([]store.Trigram, 0, len(plan.Trigrams))
	for _, tri := range plan.Trigrams {
		trigramsToSearch = append(trigramsToSearch, store.NewTrigram(tri[0], tri[1], tri[2]))
	}
	candidateLimit := limit * 10
	if candidateLimit < limit {
		candidateLimit = limit
	}
	candidates, err := e.Trigrams.Search(ctx, trigramsToSearch, store.WithLimit(candidateLimit))
	if err != nil {
		return nil, err
	}

	hits := make([]searchHit, 0, min(limit, len(candidates)))
	for _, candidate := range candidates {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		doc, err := e.Documents.Lookup(ctx, candidate.DocumentID)
		if err != nil {
			return nil, err
		}
		if doc == nil {
			continue
		}
		hit, ok := regexSearchHit(*doc, plan.Regex, scoreCandidate(candidate))
		if !ok {
			continue
		}
		hits = append(hits, hit)
		if len(hits) >= limit {
			break
		}
	}
	if len(hits) == 0 {
		return e.documentSearch(ctx, query, limit)
	}
	return hits, nil
}

func (e *Engine) documentSearch(ctx context.Context, query string, limit int) ([]searchHit, error) {
	docs, err := e.Documents.Search(ctx, query, store.WithLimit(limit))
	if err != nil {
		return nil, err
	}
	hits := make([]searchHit, 0, len(docs))
	for _, doc := range docs {
		hit, ok := substringSearchHit(doc, query)
		if ok {
			hits = append(hits, hit)
		}
	}
	return hits, nil
}

func (e *Engine) semanticSearch(ctx context.Context, queryVector []float32, limit int) ([]searchHit, error) {
	if len(queryVector) == 0 {
		return nil, errors.New("semantic search requires a query embedding")
	}
	vectors, err := e.Vectors.Search(ctx, queryVector, limit, store.DistanceMetricCosine, store.WithLimit(limit))
	if err != nil {
		return nil, err
	}
	hits := make([]searchHit, 0, len(vectors))
	for _, result := range vectors {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		doc, err := e.Documents.Lookup(ctx, result.Vector.DocumentID)
		if err != nil {
			return nil, err
		}
		if doc == nil {
			continue
		}
		hits = append(hits, searchHit{
			documentID: doc.ID,
			path:       doc.Path,
			content:    string(doc.Content),
			snippet:    firstSnippet(doc.Content),
			line:       1,
			score:      float64(result.Score),
		})
	}
	return hits, nil
}

func (e *Engine) embedQuery(ctx context.Context, query string) ([]float32, error) {
	if e.Embedder == nil {
		return nil, errors.New("semantic search requires an embedder")
	}
	vectors, err := e.Embedder.Embed(ctx, []string{query})
	if err != nil {
		return nil, err
	}
	if len(vectors) == 0 {
		return nil, errors.New("embedder returned no query embedding")
	}
	return vectors[0], nil
}

func (e *Engine) fusedResults(ctx context.Context, fused []hybrid.FusedResult, lexicalHits []searchHit, semanticHits []searchHit) ([]Result, error) {
	lexicalByID := make(map[string]searchHit, len(lexicalHits))
	for _, hit := range lexicalHits {
		lexicalByID[hit.documentID] = hit
	}
	semanticByID := make(map[string]searchHit, len(semanticHits))
	for _, hit := range semanticHits {
		semanticByID[hit.documentID] = hit
	}

	results := make([]Result, 0, len(fused))
	for _, fusedResult := range fused {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		hit, ok := lexicalByID[fusedResult.DocumentID]
		if !ok {
			hit, ok = semanticByID[fusedResult.DocumentID]
		}
		if !ok {
			doc, err := e.Documents.Lookup(ctx, fusedResult.DocumentID)
			if err != nil {
				return nil, err
			}
			if doc == nil {
				continue
			}
			hit = searchHit{
				documentID: doc.ID,
				path:       doc.Path,
				content:    string(doc.Content),
				snippet:    firstSnippet(doc.Content),
				line:       1,
			}
		}
		if hit.snippet == "" && len(fusedResult.Snippets) > 0 {
			hit.snippet = fusedResult.Snippets[0]
		}
		results = append(results, Result{
			Path:    hit.path,
			Content: hit.content,
			Snippet: hit.snippet,
			Line:    hit.line,
			Score:   fusedResult.Score,
			Matches: append([]Match(nil), hit.matches...),
		})
	}
	return results, nil
}

func hitsToResults(hits []searchHit) []Result {
	results := make([]Result, 0, len(hits))
	for _, hit := range hits {
		results = append(results, Result{
			Path:    hit.path,
			Content: hit.content,
			Snippet: hit.snippet,
			Line:    hit.line,
			Score:   hit.score,
			Matches: append([]Match(nil), hit.matches...),
		})
	}
	return results
}

func regexSearchHit(doc store.Document, expression *regexp.Regexp, score float64) (searchHit, bool) {
	if expression == nil || !expression.Match(doc.Content) {
		return searchHit{}, false
	}
	lineNumber := 1
	for line := range bytes.SplitSeq(doc.Content, []byte{10}) {
		matches := expression.FindAllIndex(line, -1)
		if len(matches) == 0 {
			lineNumber++
			continue
		}
		ranges := make([]Match, 0, len(matches))
		for _, match := range matches {
			ranges = append(ranges, Match{Start: match[0], End: match[1]})
		}
		return searchHit{
			documentID: doc.ID,
			path:       doc.Path,
			content:    string(doc.Content),
			snippet:    string(line),
			line:       lineNumber,
			score:      score,
			matches:    ranges,
		}, true
	}
	return searchHit{}, false
}

func substringSearchHit(doc store.Document, query string) (searchHit, bool) {
	if query == "" {
		return searchHit{}, false
	}
	lineNumber := 1
	needle := strings.ToLower(query)
	for line := range bytes.SplitSeq(doc.Content, []byte{10}) {
		lower := strings.ToLower(string(line))
		start := strings.Index(lower, needle)
		if start < 0 {
			lineNumber++
			continue
		}
		return searchHit{
			documentID: doc.ID,
			path:       doc.Path,
			content:    string(doc.Content),
			snippet:    string(line),
			line:       lineNumber,
			score:      1,
			matches:    []Match{{Start: start, End: start + len(query)}},
		}, true
	}
	return searchHit{}, false
}

func (e *Engine) removeDocumentPostings(ctx context.Context, documentID string) error {
	postingLists, _, err := e.Trigrams.List(ctx)
	if err != nil {
		return err
	}
	for _, postingList := range postingLists {
		filtered := make([]string, 0, len(postingList.DocumentIDs))
		removed := false
		for _, candidateID := range postingList.DocumentIDs {
			if candidateID == documentID {
				removed = true
				continue
			}
			filtered = append(filtered, candidateID)
		}
		if !removed {
			continue
		}
		if len(filtered) == 0 {
			if err := e.Trigrams.Delete(ctx, postingList.Trigram); err != nil {
				return err
			}
			continue
		}
		postingList.DocumentIDs = filtered
		if err := e.Trigrams.Put(ctx, postingList); err != nil {
			return err
		}
	}
	return nil
}

func detectLanguage(path string) string {
	language := linguist.LookupByExtension(filepath.Ext(path))
	if language == nil {
		return ""
	}
	return language.Name
}

func checksum(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func cleanPath(path string) string {
	return filepath.ToSlash(filepath.Clean(path))
}

func scoreCandidate(candidate store.PostingResult) float64 {
	if candidate.CandidateTrigrams <= 0 {
		return 0
	}
	return float64(candidate.MatchedTrigrams) / float64(candidate.CandidateTrigrams)
}

func firstSnippet(content []byte) string {
	for line := range bytes.SplitSeq(content, []byte{10}) {
		if len(line) == 0 {
			continue
		}
		return string(line)
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
