package codesearch

import "github.com/SCKelemen/codesearch/structural"

// Result is a single search result.
type Result struct {
	Path    string
	Content string
	Snippet string
	Line    int
	Score   float64
	Matches []Match
	Symbol  *structural.Symbol
}

// Match identifies a matched byte range within Snippet.
type Match struct {
	Start int
	End   int
}
