package hybrid

import (
	"context"
	"errors"
	"sort"
)

const defaultRRFK = 60.0

// Fusioner merges one or more backend result lists into a single ranked list.
type Fusioner interface {
	Fuse(ctx context.Context, lists ...ResultList) ([]FusedResult, error)
}

// RRF fuses rankings using Reciprocal Rank Fusion.
type RRF struct {
	K float64
}

// Fuse merges result lists using Reciprocal Rank Fusion.
func (f RRF) Fuse(ctx context.Context, lists ...ResultList) ([]FusedResult, error) {
	k := f.K
	if k <= 0 {
		k = defaultRRFK
	}

	accumulators := make(map[string]*fusedAccumulator)
	for _, list := range lists {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		for rank, result := range uniqueResults(list.Results) {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if result.DocumentID == "" {
				continue
			}

			acc := getAccumulator(accumulators, result.DocumentID)
			acc.result.Score += 1.0 / (k + float64(rank+1))
			acc.result.BackendScores[list.Backend] = result.Score
			acc.addSnippet(list.Backend, result.Snippet)
		}
	}

	return finalize(accumulators), nil
}

// WeightedSum fuses normalized backend scores using configured backend weights.
type WeightedSum struct {
	Normalize Normalizer
}

// Fuse merges result lists using a weighted sum of normalized scores.
func (f WeightedSum) Fuse(ctx context.Context, lists ...ResultList) ([]FusedResult, error) {
	normalize := f.Normalize
	if normalize == nil {
		normalize = MinMaxNormalize
	}

	accumulators := make(map[string]*fusedAccumulator)
	for _, list := range lists {
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		results := uniqueResults(list.Results)
		scores := make([]float64, len(results))
		for i, result := range results {
			scores[i] = result.Score
		}

		normalized := normalize(scores)
		if len(normalized) != len(results) {
			return nil, errors.New("normalizer returned a mismatched score count")
		}

		for i, result := range results {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if result.DocumentID == "" {
				continue
			}

			acc := getAccumulator(accumulators, result.DocumentID)
			acc.result.Score += list.Weight * normalized[i]
			acc.result.BackendScores[list.Backend] = result.Score
			acc.addSnippet(list.Backend, result.Snippet)
		}
	}

	return finalize(accumulators), nil
}

type fusedAccumulator struct {
	result      FusedResult
	snippetSeen map[string]struct{}
}

func getAccumulator(accumulators map[string]*fusedAccumulator, documentID string) *fusedAccumulator {
	acc := accumulators[documentID]
	if acc != nil {
		return acc
	}

	acc = &fusedAccumulator{
		result: FusedResult{
			DocumentID:     documentID,
			BackendScores:  make(map[string]float64),
			SnippetSources: make(map[string]string),
		},
		snippetSeen: make(map[string]struct{}),
	}
	accumulators[documentID] = acc
	return acc
}

func (a *fusedAccumulator) addSnippet(backend, snippet string) {
	if snippet == "" {
		return
	}
	if _, exists := a.snippetSeen[snippet]; !exists {
		a.result.Snippets = append(a.result.Snippets, snippet)
		a.snippetSeen[snippet] = struct{}{}
	}
	if backend != "" {
		a.result.SnippetSources[backend] = snippet
	}
}

func finalize(accumulators map[string]*fusedAccumulator) []FusedResult {
	results := make([]FusedResult, 0, len(accumulators))
	for _, acc := range accumulators {
		results = append(results, acc.result)
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].Score == results[j].Score {
			return results[i].DocumentID < results[j].DocumentID
		}
		return results[i].Score > results[j].Score
	})
	return results
}

func uniqueResults(results []SearchResult) []SearchResult {
	if len(results) <= 1 {
		return results
	}

	seen := make(map[string]struct{}, len(results))
	out := make([]SearchResult, 0, len(results))
	for _, result := range results {
		if result.DocumentID == "" {
			out = append(out, result)
			continue
		}
		if _, exists := seen[result.DocumentID]; exists {
			continue
		}
		seen[result.DocumentID] = struct{}{}
		out = append(out, result)
	}
	return out
}
