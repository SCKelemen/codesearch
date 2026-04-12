package codesearchv1

// SearchRequest contains the parameters for a code search query.
type SearchRequest struct {
	Query  string `json:"query"`
	Limit  int32  `json:"limit,omitempty"`
	Mode   string `json:"mode,omitempty"`
	Filter string `json:"filter,omitempty"`
}

// SearchResponse contains the results of a code search query.
type SearchResponse struct {
	Query   string         `json:"query"`
	Limit   int32          `json:"limit"`
	Mode    string         `json:"mode"`
	Results []SearchResult `json:"results"`
}

// SearchResult represents a single search hit with match details.
type SearchResult struct {
	Path    string       `json:"path"`
	Line    int32        `json:"line,omitempty"`
	Score   float64      `json:"score"`
	Snippet string       `json:"snippet,omitempty"`
	Matches []MatchRange `json:"matches,omitempty"`
}

// MatchRange identifies a matched region within a line of content.
type MatchRange struct {
	Start int32 `json:"start"`
	End   int32 `json:"end"`
}

// SearchSymbolsRequest contains the parameters for a symbol search query.
type SearchSymbolsRequest struct {
	Name      string `json:"name,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Language  string `json:"language,omitempty"`
	Container string `json:"container,omitempty"`
	Path      string `json:"path,omitempty"`
	Limit     int32  `json:"limit,omitempty"`
}

// SearchSymbolsResponse contains the results of a symbol search query.
type SearchSymbolsResponse struct {
	Results []SymbolResult `json:"results"`
}

// SymbolResult represents a single symbol search hit.
type SymbolResult struct {
	Name      string      `json:"name"`
	Kind      string      `json:"kind"`
	Language  string      `json:"language"`
	Path      string      `json:"path"`
	Container string      `json:"container,omitempty"`
	Exported  bool        `json:"exported"`
	Range     SourceRange `json:"range"`
}

// SourceRange identifies a range of lines and columns in a source file.
type SourceRange struct {
	StartLine   int32 `json:"startLine"`
	StartColumn int32 `json:"startColumn"`
	EndLine     int32 `json:"endLine"`
	EndColumn   int32 `json:"endColumn"`
}

// IndexStatusRequest is the request message for index status queries.
type IndexStatusRequest struct{}

// IndexStatusResponse contains index health and statistics.
type IndexStatusResponse struct {
	FileCount      int32            `json:"fileCount"`
	TotalBytes     int64            `json:"totalBytes"`
	IndexBytes     int64            `json:"indexBytes"`
	EmbeddingCount int32            `json:"embeddingCount"`
	Languages      map[string]int32 `json:"languages"`
}
