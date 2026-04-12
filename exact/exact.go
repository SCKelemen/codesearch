// Package exact provides exact substring search using Go's suffix array.
//
// For indexed search, it builds a suffix array over the content which
// allows O(m log n) substring lookups with no false positives.
// For simple one-shot search, it provides Boyer-Moore-Horspool.
package exact

import (
	"index/suffixarray"
	"sort"
)

// Index is an exact substring search index over a byte corpus.
// It wraps Go's standard library suffix array for O(m log n) lookups.
type Index struct {
	data []byte
	sa   *suffixarray.Index

	// Line offsets for mapping byte positions to lines.
	lines []int // byte offset of each line start
}

// NewIndex builds an exact search index over the given content.
func NewIndex(data []byte) *Index {
	idx := &Index{
		data: data,
		sa:   suffixarray.New(data),
	}
	idx.buildLineIndex()
	return idx
}

func (idx *Index) buildLineIndex() {
	idx.lines = []int{0}
	for i, b := range idx.data {
		if b == '\n' {
			idx.lines = append(idx.lines, i+1)
		}
	}
}

// Match represents an exact substring match.
type Match struct {
	Offset int    // byte offset in the corpus
	Line   int    // 1-based line number
	Column int    // 0-based column (byte offset within line)
	Text   string // the matched line content
}

// Search finds all occurrences of the literal pattern.
// Results are sorted by offset.
func (idx *Index) Search(pattern []byte) []Match {
	if len(pattern) == 0 || len(idx.data) == 0 {
		return nil
	}

	offsets := idx.sa.Lookup(pattern, -1)
	if len(offsets) == 0 {
		return nil
	}

	sort.Ints(offsets)

	matches := make([]Match, 0, len(offsets))
	for _, off := range offsets {
		line := idx.offsetToLine(off)
		lineStart := idx.lines[line]
		lineEnd := idx.lineEnd(line)
		matches = append(matches, Match{
			Offset: off,
			Line:   line + 1,
			Column: off - lineStart,
			Text:   string(idx.data[lineStart:lineEnd]),
		})
	}
	return matches
}

// SearchString is a convenience wrapper for string inputs.
func (idx *Index) SearchString(pattern string) []Match {
	return idx.Search([]byte(pattern))
}

// Count returns the number of non-overlapping occurrences.
func (idx *Index) Count(pattern []byte) int {
	if len(pattern) == 0 {
		return 0
	}
	return len(idx.sa.Lookup(pattern, -1))
}

// Size returns the size of the indexed content in bytes.
func (idx *Index) Size() int {
	return len(idx.data)
}

// LineCount returns the number of lines in the indexed content.
func (idx *Index) LineCount() int {
	return len(idx.lines)
}

func (idx *Index) offsetToLine(offset int) int {
	line := sort.SearchInts(idx.lines, offset+1) - 1
	if line < 0 {
		return 0
	}
	return line
}

func (idx *Index) lineEnd(line int) int {
	if line+1 < len(idx.lines) {
		end := idx.lines[line+1]
		if end > 0 && idx.data[end-1] == '\n' {
			return end - 1
		}
		return end
	}
	return len(idx.data)
}
