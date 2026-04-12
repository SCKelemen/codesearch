package codesearch

import (
	"context"

	"github.com/SCKelemen/codesearch/embedding"
	"github.com/SCKelemen/codesearch/hybrid"
	"github.com/SCKelemen/codesearch/store"
	"github.com/SCKelemen/codesearch/structural"
)

// Option configures an Engine.
type Option func(*engineConfig)

type engineConfig struct {
	useFileStore bool
	storeDir     string

	documentStore store.DocumentStore
	trigramStore  store.TrigramStore
	vectorStore   store.VectorStore
	symbolStore   store.SymbolStore

	embedder     embedding.Embedder
	hybridSearch bool
}

// WithMemoryStore configures the engine to use in-memory stores.
func WithMemoryStore() Option {
	return func(cfg *engineConfig) {
		cfg.useFileStore = false
		cfg.storeDir = ""
	}
}

// WithFileStore configures the engine to use file-backed stores.
func WithFileStore(dir string) Option {
	return func(cfg *engineConfig) {
		cfg.useFileStore = true
		cfg.storeDir = dir
	}
}

// WithDocumentStore uses a custom document store backend.
func WithDocumentStore(documentStore store.DocumentStore) Option {
	return func(cfg *engineConfig) {
		cfg.documentStore = documentStore
	}
}

// WithTrigramStore uses a custom trigram store backend.
func WithTrigramStore(trigramStore store.TrigramStore) Option {
	return func(cfg *engineConfig) {
		cfg.trigramStore = trigramStore
	}
}

// WithVectorStore uses a custom vector store backend.
func WithVectorStore(vectorStore store.VectorStore) Option {
	return func(cfg *engineConfig) {
		cfg.vectorStore = vectorStore
	}
}

// WithEmbedder enables semantic search with the provided embedder.
func WithEmbedder(embedder embedding.Embedder) Option {
	return func(cfg *engineConfig) {
		cfg.embedder = embedder
	}
}

// WithHybridSearch enables or disables hybrid search when embeddings are available.
func WithHybridSearch(enabled bool) Option {
	return func(cfg *engineConfig) {
		cfg.hybridSearch = enabled
	}
}

// SearchOption configures a search request.
type SearchOption func(*searchOptions)

type searchOptions struct {
	limit       int
	mode        hybrid.SearchMode
	filter      string
	symbolQuery *structural.SymbolQuery
}

// WithLimit limits the number of returned results.
func WithLimit(limit int) SearchOption {
	return func(opts *searchOptions) {
		opts.limit = limit
	}
}

// WithMode selects the hybrid search mode.
func WithMode(mode hybrid.SearchMode) SearchOption {
	return func(opts *searchOptions) {
		opts.mode = mode
	}
}

// WithFilter applies a CEL filter expression to search results.
func WithFilter(expression string) SearchOption {
	return func(opts *searchOptions) {
		opts.filter = expression
	}
}

// WithSymbolQuery enables structural symbol search mode.
func WithSymbolQuery(query structural.SymbolQuery) SearchOption {
	return func(opts *searchOptions) {
		queryCopy := query
		opts.symbolQuery = &queryCopy
	}
}

func resolveSearchOptions(opts ...SearchOption) searchOptions {
	resolved := searchOptions{limit: defaultSearchLimit}
	for _, opt := range opts {
		if opt != nil {
			opt(&resolved)
		}
	}
	if resolved.limit <= 0 {
		resolved.limit = defaultSearchLimit
	}
	return resolved
}

// IndexOption configures indexing behavior.
type IndexOption func(*indexOptions)

// SymbolExtractor overrides structural symbol extraction during indexing.
type SymbolExtractor func(ctx context.Context, path string, language string, content []byte) ([]structural.Symbol, error)

type indexOptions struct {
	language        string
	embeddings      bool
	symbolExtractor SymbolExtractor
}

// WithLanguage overrides language detection during indexing.
func WithLanguage(language string) IndexOption {
	return func(opts *indexOptions) {
		opts.language = language
	}
}

// WithEmbeddings enables or disables embedding generation while indexing.
func WithEmbeddings(enabled bool) IndexOption {
	return func(opts *indexOptions) {
		opts.embeddings = enabled
	}
}

// WithSymbolExtractor overrides structural symbol extraction during indexing.
func WithSymbolExtractor(extractor SymbolExtractor) IndexOption {
	return func(opts *indexOptions) {
		opts.symbolExtractor = extractor
	}
}

func resolveIndexOptions(opts ...IndexOption) indexOptions {
	resolved := indexOptions{embeddings: true}
	for _, opt := range opts {
		if opt != nil {
			opt(&resolved)
		}
	}
	return resolved
}
