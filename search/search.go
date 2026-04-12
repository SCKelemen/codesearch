// Package search provides a unified search engine that composes
// multiple index types: trigram (regex), exact (suffix array),
// fuzzy (fzf algorithm), and symbol (LSIF intelligence).
//
// It accepts a query, routes to the appropriate indexes, merges
// results, applies ranking, and optionally filters via CEL.
package search

import (
	"context"

	"github.com/SCKelemen/codesearch/exact"
	"github.com/SCKelemen/codesearch/fuzzy"
	"github.com/SCKelemen/codesearch/ranking"
	"github.com/SCKelemen/codesearch/symbol"
	"github.com/SCKelemen/codesearch/trigram"
)

// Mode determines which search strategy to use.
type Mode int

const (
	ModeAuto   Mode = iota // auto-detect from query
	ModeRegex              // trigram-prefiltered regex
	ModeExact              // suffix array exact match
	ModeFuzzy              // fzf fuzzy match
	ModeSymbol             // symbol name search
)

// Request describes a search query.
type Request struct {
	Query      string
	Mode       Mode
	MaxResults int
	Filter     string // CEL filter expression (optional)

	// Scope constraints
	WorkspaceID  string
	RepositoryID string
	Branch       string
	Language     string
}

// Result is a single search hit.
type Result struct {
	URI         string
	Path        string
	Line        int
	Column      int
	LineContent string
	Score       float64
	Mode        Mode // which mode produced this result

	// Symbol metadata (if from symbol search)
	SymbolName   string
	SymbolKind   symbol.Kind
	IsDefinition bool
}

// Engine composes multiple search indexes.
type Engine struct {
	trigramIndex trigram.Index
	exactIndexes map[string]*exact.Index // URI -> exact index
	symbolIndex  *symbol.Index
	ranker       *ranking.Ranker
}

// NewEngine creates a search engine with the provided indexes.
func NewEngine(opts ...EngineOption) *Engine {
	e := &Engine{
		exactIndexes: make(map[string]*exact.Index),
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

// EngineOption configures the search engine.
type EngineOption func(*Engine)

// WithTrigramIndex sets the trigram index for regex search.
func WithTrigramIndex(idx trigram.Index) EngineOption {
	return func(e *Engine) {
		e.trigramIndex = idx
	}
}

// WithExactIndex adds an exact search index for a document.
func WithExactIndex(uri string, idx *exact.Index) EngineOption {
	return func(e *Engine) {
		e.exactIndexes[uri] = idx
	}
}

// WithSymbolIndex sets the symbol index for code intelligence search.
func WithSymbolIndex(idx *symbol.Index) EngineOption {
	return func(e *Engine) {
		e.symbolIndex = idx
	}
}

// WithRanker sets the ranking model.
func WithRanker(r *ranking.Ranker) EngineOption {
	return func(e *Engine) {
		e.ranker = r
	}
}

// Search executes a search request and returns ranked results.
func (e *Engine) Search(ctx context.Context, req Request) ([]Result, error) {
	if req.MaxResults <= 0 {
		req.MaxResults = 100
	}

	mode := req.Mode
	if mode == ModeAuto {
		mode = detectMode(req.Query)
	}

	var results []Result
	var err error

	switch mode {
	case ModeRegex:
		results, err = e.searchRegex(ctx, req)
	case ModeExact:
		results, err = e.searchExact(ctx, req)
	case ModeFuzzy:
		results, err = e.searchFuzzy(ctx, req)
	case ModeSymbol:
		results, err = e.searchSymbol(ctx, req)
	default:
		results, err = e.searchExact(ctx, req)
	}

	if err != nil {
		return nil, err
	}

	// Truncate to max results
	if len(results) > req.MaxResults {
		results = results[:req.MaxResults]
	}

	return results, nil
}

func (e *Engine) searchRegex(ctx context.Context, req Request) ([]Result, error) {
	if e.trigramIndex == nil {
		return nil, nil
	}

	opts := trigram.SearchOptions{
		WorkspaceID:  req.WorkspaceID,
		RepositoryID: req.RepositoryID,
		Language:     req.Language,
		MaxResults:   req.MaxResults,
	}

	triResults, err := trigram.NewSearcher(e.trigramIndex).Search(ctx, req.Query, opts)
	if err != nil {
		return nil, err
	}

	results := make([]Result, 0, len(triResults))
	for _, tr := range triResults {
		results = append(results, Result{
			URI:         tr.FilePath,
			Path:        tr.FilePath,
			Line:        tr.LineNumber,
			LineContent: tr.LineContent,
			Mode:        ModeRegex,
		})
	}
	return results, nil
}

func (e *Engine) searchExact(_ context.Context, req Request) ([]Result, error) {
	var results []Result
	pattern := []byte(req.Query)
	for uri, idx := range e.exactIndexes {
		matches := idx.Search(pattern)
		for _, m := range matches {
			results = append(results, Result{
				URI:         uri,
				Path:        uri,
				Line:        m.Line,
				Column:      m.Column,
				LineContent: m.Text,
				Mode:        ModeExact,
			})
		}
	}
	return results, nil
}

func (e *Engine) searchFuzzy(_ context.Context, req Request) ([]Result, error) {
	if e.symbolIndex == nil {
		return nil, nil
	}

	opts := fuzzy.Options{WithPositions: true}
	syms := e.symbolIndex.All()
	var results []Result
	for _, sym := range syms {
		r := fuzzy.Match(sym.Name, req.Query, opts)
		if r.Start >= 0 && r.Score > 0 {
			results = append(results, Result{
				URI:          sym.Location.URI,
				Path:         sym.Location.URI,
				Line:         sym.Location.StartLine + 1,
				Column:       sym.Location.StartCol,
				Score:        float64(r.Score),
				SymbolName:   sym.Name,
				SymbolKind:   sym.Kind,
				IsDefinition: sym.IsDefinition,
				Mode:         ModeFuzzy,
			})
		}
	}

	// Sort by score descending
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].Score > results[j-1].Score; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}

	return results, nil
}

func (e *Engine) searchSymbol(_ context.Context, req Request) ([]Result, error) {
	if e.symbolIndex == nil {
		return nil, nil
	}

	syms := e.symbolIndex.LookupName(req.Query)
	results := make([]Result, 0, len(syms))
	for _, sym := range syms {
		results = append(results, Result{
			URI:          sym.Location.URI,
			Path:         sym.Location.URI,
			Line:         sym.Location.StartLine + 1,
			Column:       sym.Location.StartCol,
			SymbolName:   sym.Name,
			SymbolKind:   sym.Kind,
			IsDefinition: sym.IsDefinition,
			Mode:         ModeSymbol,
		})
	}
	return results, nil
}

// detectMode guesses the search mode from the query string.
func detectMode(query string) Mode {
	// If query contains regex metacharacters, use regex mode
	for _, c := range query {
		switch c {
		case '.', '*', '+', '?', '[', ']', '(', ')', '{', '}', '|', '^', '$', '\\':
			return ModeRegex
		}
	}
	// If query looks like a symbol name (starts with uppercase or contains ::)
	if len(query) > 0 {
		first := rune(query[0])
		if first >= 'A' && first <= 'Z' {
			return ModeSymbol
		}
	}
	// Default to exact
	return ModeExact
}
