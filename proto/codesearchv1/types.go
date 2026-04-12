package codesearchv1

type SearchRequest struct {
	Query  string `json:"query"`
	Limit  int32  `json:"limit,omitempty"`
	Mode   string `json:"mode,omitempty"`
	Filter string `json:"filter,omitempty"`
}

type SearchResponse struct {
	Query   string         `json:"query"`
	Limit   int32          `json:"limit"`
	Mode    string         `json:"mode"`
	Results []SearchResult `json:"results"`
}

type SearchResult struct {
	Path    string       `json:"path"`
	Line    int32        `json:"line,omitempty"`
	Score   float64      `json:"score"`
	Snippet string       `json:"snippet,omitempty"`
	Matches []MatchRange `json:"matches,omitempty"`
}

type MatchRange struct {
	Start int32 `json:"start"`
	End   int32 `json:"end"`
}

type IndexStatusRequest struct{}

type IndexStatusResponse struct {
	FileCount      int32            `json:"fileCount"`
	TotalBytes     int64            `json:"totalBytes"`
	IndexBytes     int64            `json:"indexBytes"`
	EmbeddingCount int32            `json:"embeddingCount"`
	Languages      map[string]int32 `json:"languages"`
}
