package shard

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/SCKelemen/codesearch/store"
)

var _ store.ShardSearcher = (*Searcher)(nil)

// Searcher searches multiple shards in parallel.
type Searcher struct {
	shards []store.IndexShard
}

func NewSearcher(shards ...store.IndexShard) *Searcher {
	return &Searcher{shards: append([]store.IndexShard(nil), shards...)}
}

func (s *Searcher) SearchDocuments(ctx context.Context, query string, opts ...store.SearchOption) ([]store.Document, error) {
	results := make([][]store.Document, len(s.shards))
	if err := s.run(ctx, func(i int, shard store.IndexShard) error {
		docs, err := shard.SearchDocuments(ctx, query, opts...)
		if err == nil {
			results[i] = docs
		}
		return err
	}); err != nil {
		return nil, err
	}
	merged := make(map[string]store.Document)
	for _, batch := range results {
		for _, doc := range batch {
			merged[doc.ID] = doc
		}
	}
	out := make([]store.Document, 0, len(merged))
	needle := strings.ToLower(query)
	for _, doc := range merged {
		out = append(out, doc)
	}
	sort.Slice(out, func(i, j int) bool {
		li := scoreDocument(out[i], needle)
		lj := scoreDocument(out[j], needle)
		if li != lj {
			return li > lj
		}
		return out[i].Path < out[j].Path
	})
	if limit := store.ResolveSearchOptions(opts...).Limit; limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Searcher) SearchTrigrams(ctx context.Context, trigrams []store.Trigram, opts ...store.SearchOption) ([]store.PostingResult, error) {
	results := make([][]store.PostingResult, len(s.shards))
	if err := s.run(ctx, func(i int, shard store.IndexShard) error {
		matches, err := shard.SearchTrigrams(ctx, trigrams, opts...)
		if err == nil {
			results[i] = matches
		}
		return err
	}); err != nil {
		return nil, err
	}
	merged := make(map[string]store.PostingResult)
	for _, batch := range results {
		for _, result := range batch {
			current := merged[result.DocumentID]
			current.DocumentID = result.DocumentID
			current.MatchedTrigrams += result.MatchedTrigrams
			if result.CandidateTrigrams > current.CandidateTrigrams {
				current.CandidateTrigrams = result.CandidateTrigrams
			}
			merged[result.DocumentID] = current
		}
	}
	out := make([]store.PostingResult, 0, len(merged))
	for _, result := range merged {
		out = append(out, result)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].MatchedTrigrams != out[j].MatchedTrigrams {
			return out[i].MatchedTrigrams > out[j].MatchedTrigrams
		}
		return out[i].DocumentID < out[j].DocumentID
	})
	if limit := store.ResolveSearchOptions(opts...).Limit; limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Searcher) SearchVectors(ctx context.Context, query []float32, k int, metric store.DistanceMetric, opts ...store.SearchOption) ([]store.VectorResult, error) {
	results := make([][]store.VectorResult, len(s.shards))
	if err := s.run(ctx, func(i int, shard store.IndexShard) error {
		matches, err := shard.SearchVectors(ctx, query, k, metric, opts...)
		if err == nil {
			results[i] = matches
		}
		return err
	}); err != nil {
		return nil, err
	}
	out := make([]store.VectorResult, 0)
	for _, batch := range results {
		out = append(out, batch...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		if out[i].Distance != out[j].Distance {
			return out[i].Distance < out[j].Distance
		}
		return out[i].Vector.ID < out[j].Vector.ID
	})
	limit := k
	if limit <= 0 {
		limit = len(out)
	}
	if l := store.ResolveSearchOptions(opts...).Limit; l > 0 && l < limit {
		limit = l
	}
	if len(out) > limit {
		out = out[:limit]
	}
	for i := range out {
		out[i].Rank = i + 1
	}
	return out, nil
}

func (s *Searcher) SearchSymbols(ctx context.Context, query string, opts ...store.SearchOption) ([]store.Symbol, error) {
	results := make([][]store.Symbol, len(s.shards))
	if err := s.run(ctx, func(i int, shard store.IndexShard) error {
		matches, err := shard.SearchSymbols(ctx, query, opts...)
		if err == nil {
			results[i] = matches
		}
		return err
	}); err != nil {
		return nil, err
	}
	merged := make(map[string]store.Symbol)
	for _, batch := range results {
		for _, sym := range batch {
			merged[sym.ID] = sym
		}
	}
	out := make([]store.Symbol, 0, len(merged))
	needle := strings.ToLower(query)
	for _, sym := range merged {
		out = append(out, sym)
	}
	sort.Slice(out, func(i, j int) bool {
		li := scoreSymbol(out[i], needle)
		lj := scoreSymbol(out[j], needle)
		if li != lj {
			return li > lj
		}
		return out[i].Name < out[j].Name
	})
	if limit := store.ResolveSearchOptions(opts...).Limit; limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Searcher) run(ctx context.Context, fn func(i int, shard store.IndexShard) error) error {
	var wg sync.WaitGroup
	errCh := make(chan error, len(s.shards))
	for i, shard := range s.shards {
		wg.Add(1)
		go func(i int, shard store.IndexShard) {
			defer wg.Done()
			if err := ctx.Err(); err != nil {
				errCh <- err
				return
			}
			if err := fn(i, shard); err != nil {
				errCh <- err
			}
		}(i, shard)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			return err
		}
	}
	return nil
}

func scoreDocument(doc store.Document, needle string) int {
	if needle == "" {
		return 1
	}
	score := 0
	content := strings.ToLower(string(doc.Content))
	path := strings.ToLower(doc.Path)
	if strings.Contains(path, needle) {
		score += 4
	}
	if strings.Contains(content, needle) {
		score += 2
	}
	return score
}

func scoreSymbol(symbol store.Symbol, needle string) int {
	if needle == "" {
		return 1
	}
	name := strings.ToLower(symbol.Name)
	switch {
	case name == needle:
		return 4
	case strings.HasPrefix(name, needle):
		return 3
	case strings.Contains(name, needle):
		return 2
	default:
		return 0
	}
}
