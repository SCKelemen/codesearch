package codesearch

// Result is a single search result.
type Result struct {
	Path    string
	Content string
	Snippet string
	Line    int
	Score   float64
	Matches []Match
}

// Match identifies a matched byte range within Snippet.
type Match struct {
	Start int
	End   int
}
