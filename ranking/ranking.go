// Package ranking provides code-aware search result scoring.
//
// It combines BM25 text relevance with code-specific signals:
// symbol importance, definition vs reference, file recency, etc.
package ranking

import "math"

// Document represents a searchable document for ranking.
type Document struct {
	URI       string
	Language  string
	LineCount int

	// Symbol-level signals
	IsDefinition   bool
	IsExported     bool
	SymbolKind     int // LSP symbol kind
	ReferenceCount int // how many references point to this symbol

	// File-level signals
	LastModified int64   // unix timestamp
	ImportCount  int     // number of other files importing this
	Depth        int     // directory depth from root
	Score        float64 // computed score (output)
}

// Query holds query-level signals for ranking.
type Query struct {
	Terms    []string
	IsSymbol bool // query looks like a symbol name
	IsExact  bool // exact match requested
}

// Config holds tunable ranking parameters.
type Config struct {
	// BM25 parameters
	K1 float64 // term frequency saturation, default 1.2
	B  float64 // length normalization, default 0.75

	// Code-specific weights
	DefinitionBoost  float64 // boost for definitions over references
	ExportedBoost    float64 // boost for exported/public symbols
	SymbolNameBoost  float64 // boost when query matches a symbol name exactly
	ReferenceWeight  float64 // weight for reference count (log scaled)
	RecencyWeight    float64 // weight for file recency
	ImportWeight     float64 // weight for import count (log scaled)
	DepthPenalty     float64 // penalty per directory depth level
	ConsecutiveBoost float64 // boost for consecutive term matches
}

// DefaultConfig returns production-quality ranking parameters.
func DefaultConfig() Config {
	return Config{
		K1:               1.2,
		B:                0.75,
		DefinitionBoost:  3.0,
		ExportedBoost:    1.5,
		SymbolNameBoost:  5.0,
		ReferenceWeight:  0.5,
		RecencyWeight:    0.1,
		ImportWeight:     0.3,
		DepthPenalty:     0.05,
		ConsecutiveBoost: 1.5,
	}
}

// Ranker scores and ranks search results.
type Ranker struct {
	cfg          Config
	avgDocLength float64
	totalDocs    int
	docFreqs     map[string]int // term -> doc frequency
}

// NewRanker creates a ranker with corpus statistics.
func NewRanker(cfg Config, totalDocs int, avgDocLength float64, docFreqs map[string]int) *Ranker {
	return &Ranker{
		cfg:          cfg,
		avgDocLength: avgDocLength,
		totalDocs:    totalDocs,
		docFreqs:     docFreqs,
	}
}

// Score computes the final relevance score for a document.
func (r *Ranker) Score(doc *Document, query Query, termFreqs map[string]int) float64 {
	score := r.bm25(doc, query, termFreqs)
	score += r.symbolBoost(doc, query)
	score += r.structuralBoost(doc)
	doc.Score = score
	return score
}

// bm25 computes the BM25 text relevance component.
func (r *Ranker) bm25(doc *Document, query Query, termFreqs map[string]int) float64 {
	score := 0.0
	docLen := float64(doc.LineCount)
	for _, term := range query.Terms {
		tf := float64(termFreqs[term])
		df := float64(r.docFreqs[term])
		if df == 0 {
			continue
		}
		// IDF with smoothing
		idf := math.Log((float64(r.totalDocs) - df + 0.5) / (df + 0.5))
		if idf < 0 {
			idf = 0
		}
		// TF saturation with length normalization
		tfNorm := (tf * (r.cfg.K1 + 1)) /
			(tf + r.cfg.K1*(1-r.cfg.B+r.cfg.B*(docLen/r.avgDocLength)))
		score += idf * tfNorm
	}
	return score
}

// symbolBoost adds code-intelligence signals.
func (r *Ranker) symbolBoost(doc *Document, query Query) float64 {
	boost := 0.0

	if doc.IsDefinition {
		boost += r.cfg.DefinitionBoost
	}
	if doc.IsExported {
		boost += r.cfg.ExportedBoost
	}
	if doc.ReferenceCount > 0 {
		boost += r.cfg.ReferenceWeight * math.Log1p(float64(doc.ReferenceCount))
	}

	return boost
}

// structuralBoost adds file-level signals.
func (r *Ranker) structuralBoost(doc *Document) float64 {
	boost := 0.0

	if doc.ImportCount > 0 {
		boost += r.cfg.ImportWeight * math.Log1p(float64(doc.ImportCount))
	}
	if doc.Depth > 0 {
		boost -= r.cfg.DepthPenalty * float64(doc.Depth)
	}
	if doc.LastModified > 0 {
		// Recency: score decays with age. This is a simplified model.
		boost += r.cfg.RecencyWeight
	}

	return boost
}

// Rank sorts documents by score in descending order.
func (r *Ranker) Rank(docs []*Document, query Query, termFreqsByDoc map[string]map[string]int) {
	for _, doc := range docs {
		freqs := termFreqsByDoc[doc.URI]
		if freqs == nil {
			freqs = map[string]int{}
		}
		r.Score(doc, query, freqs)
	}
	SortByScore(docs)
}

// SortByScore sorts documents by score descending.
func SortByScore(docs []*Document) {
	n := len(docs)
	for i := 1; i < n; i++ {
		for j := i; j > 0 && docs[j].Score > docs[j-1].Score; j-- {
			docs[j], docs[j-1] = docs[j-1], docs[j]
		}
	}
}
