package exact

import "bytes"

// PhraseSearch finds exact phrase matches and returns surrounding context.
type PhraseSearch struct {
	idx *Index
}

// NewPhraseSearch creates a phrase searcher over an existing index.
func NewPhraseSearch(idx *Index) *PhraseSearch {
	return &PhraseSearch{idx: idx}
}

// PhraseMatch holds a phrase match with surrounding context.
type PhraseMatch struct {
	Match
	Before string // context before the match on the same line
	After  string // context after the match on the same line
}

// Search finds all occurrences of the phrase with surrounding context.
func (ps *PhraseSearch) Search(phrase []byte) []PhraseMatch {
	matches := ps.idx.Search(phrase)
	results := make([]PhraseMatch, 0, len(matches))
	for _, m := range matches {
		lineBytes := []byte(m.Text)
		phrIdx := bytes.Index(lineBytes[m.Column:], phrase)
		if phrIdx < 0 {
			phrIdx = 0
		}
		beforeEnd := m.Column + phrIdx
		afterStart := beforeEnd + len(phrase)
		if afterStart > len(lineBytes) {
			afterStart = len(lineBytes)
		}
		results = append(results, PhraseMatch{
			Match:  m,
			Before: string(lineBytes[:beforeEnd]),
			After:  string(lineBytes[afterStart:]),
		})
	}
	return results
}
