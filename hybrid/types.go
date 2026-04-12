package hybrid

// SearchMode selects which backend set to execute.
type SearchMode string

const (
	// LexicalOnly runs only lexical backends such as exact, fuzzy, or trigram search.
	LexicalOnly SearchMode = "lexical_only"

	// SemanticOnly runs only semantic backends such as vector search.
	SemanticOnly SearchMode = "semantic_only"

	// Hybrid runs all configured backends and fuses the results.
	Hybrid SearchMode = "hybrid"
)

// SearchRequest describes a hybrid search query.
type SearchRequest struct {
	Query      string
	Vector     []float32
	MaxResults int
	Mode       SearchMode
}

// SearchResult is a scored result from a single backend.
type SearchResult struct {
	DocumentID string
	Score      float64
	Snippet    string
}

// ResultList contains the ordered results from one backend.
type ResultList struct {
	Backend string
	Weight  float64
	Results []SearchResult
}

// FusedResult is a merged document-level result across backends.
type FusedResult struct {
	DocumentID     string
	Score          float64
	BackendScores  map[string]float64
	SnippetSources map[string]string
	Snippets       []string
}
