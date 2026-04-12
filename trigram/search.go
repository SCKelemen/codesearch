package trigram

import (
	"bytes"
	"context"
	"fmt"
	"os"
)

// SearchOptions constrain search results.
type SearchOptions struct {
	WorkspaceID  string
	RepositoryID string
	IndexID      string
	Language     string
	MaxResults   int
}

// SearchResult is a verified regex match in a file line.
type SearchResult struct {
	Posting
	LineNumber  int
	LineContent string
	MatchRanges []Range
}

// Range describes a half-open byte range [Start, End) within LineContent.
type Range struct {
	Start int
	End   int
}

// Searcher performs trigram-prefiltered regex search.
type Searcher struct {
	idx Index
}

func NewSearcher(idx Index) *Searcher {
	return &Searcher{idx: idx}
}

func (s *Searcher) Search(ctx context.Context, pattern string, opts SearchOptions) ([]SearchResult, error) {
	if s == nil || s.idx == nil {
		return nil, fmt.Errorf("nil trigram index")
	}

	plan, err := BuildQueryPlan(pattern)
	if err != nil {
		return nil, err
	}

	postings, err := s.idx.Query(ctx, plan.Trigrams)
	if err != nil {
		return nil, err
	}
	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = 100
	}

	results := make([]SearchResult, 0)
	seen := make(map[string]struct{}, len(postings))
	for _, posting := range postings {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if !matchesOptions(posting, opts) {
			continue
		}

		key := postingKey(posting)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		fileResults, err := searchPostingFile(ctx, posting, plan, maxResults-len(results))
		if err != nil {
			return nil, err
		}
		results = append(results, fileResults...)
		if len(results) >= maxResults {
			return results[:maxResults], nil
		}
	}

	return results, nil
}

func matchesOptions(posting Posting, opts SearchOptions) bool {
	if opts.WorkspaceID != "" && posting.WorkspaceID != opts.WorkspaceID {
		return false
	}
	if opts.RepositoryID != "" && posting.RepositoryID != opts.RepositoryID {
		return false
	}
	if opts.IndexID != "" && posting.IndexID != opts.IndexID {
		return false
	}
	if opts.Language != "" && posting.Language != opts.Language {
		return false
	}
	return true
}

func postingKey(posting Posting) string {
	return posting.WorkspaceID + "\x00" + posting.RepositoryID + "\x00" + posting.IndexID + "\x00" + posting.FilePath
}

func searchPostingFile(ctx context.Context, posting Posting, plan *QueryPlan, remaining int) ([]SearchResult, error) {
	if remaining <= 0 {
		return nil, nil
	}

	content, err := os.ReadFile(posting.FilePath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", posting.FilePath, err)
	}
	if !plan.Regex.Match(content) {
		return nil, nil
	}

	results := make([]SearchResult, 0)
	lineNumber := 1
	for line := range bytes.SplitSeq(content, []byte{'\n'}) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		matches := plan.Regex.FindAllIndex(line, -1)
		if len(matches) > 0 {
			ranges := make([]Range, 0, len(matches))
			for _, match := range matches {
				ranges = append(ranges, Range{Start: match[0], End: match[1]})
			}
			results = append(results, SearchResult{
				Posting:     posting,
				LineNumber:  lineNumber,
				LineContent: string(line),
				MatchRanges: ranges,
			})
			if len(results) >= remaining {
				return results[:remaining], nil
			}
		}
		lineNumber++
	}

	return results, nil
}
