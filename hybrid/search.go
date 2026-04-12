package hybrid

import (
	"context"
	"errors"
	"sync"
)

// BackendSearcher is implemented by lexical and semantic search backends.
type BackendSearcher interface {
	Search(ctx context.Context, req SearchRequest) ([]SearchResult, error)
}

// SearcherConfig configures HybridSearcher behavior.
type SearcherConfig struct {
	Fusioner        Fusioner
	LexicalWeight   float64
	SemanticWeight  float64
	LexicalBackend  string
	SemanticBackend string
}

// DefaultSearcherConfig returns the default hybrid search configuration.
func DefaultSearcherConfig() SearcherConfig {
	return SearcherConfig{
		Fusioner:        RRF{},
		LexicalWeight:   1,
		SemanticWeight:  1,
		LexicalBackend:  "lexical",
		SemanticBackend: "semantic",
	}
}

// HybridSearcher executes lexical and semantic search in parallel and fuses the results.
type HybridSearcher struct {
	lexical  BackendSearcher
	semantic BackendSearcher
	cfg      SearcherConfig
}

// NewHybridSearcher creates a new hybrid searcher.
func NewHybridSearcher(lexical, semantic BackendSearcher, cfg SearcherConfig) *HybridSearcher {
	cfg = withSearcherDefaults(cfg)
	return &HybridSearcher{
		lexical:  lexical,
		semantic: semantic,
		cfg:      cfg,
	}
}

// Search executes the configured backend set and fuses the results.
func (s *HybridSearcher) Search(ctx context.Context, req SearchRequest) ([]FusedResult, error) {
	mode := req.Mode
	if mode == "" {
		mode = Hybrid
	}

	lists, err := s.collect(ctx, req, mode)
	if err != nil {
		return nil, err
	}

	results, err := s.cfg.Fusioner.Fuse(ctx, lists...)
	if err != nil {
		return nil, err
	}
	if req.MaxResults > 0 && len(results) > req.MaxResults {
		results = results[:req.MaxResults]
	}
	return results, nil
}

func (s *HybridSearcher) collect(ctx context.Context, req SearchRequest, mode SearchMode) ([]ResultList, error) {
	requests := make([]backendRequest, 0, 2)
	switch mode {
	case LexicalOnly:
		if s.lexical == nil {
			return nil, errors.New("lexical searcher is nil")
		}
		requests = append(requests, backendRequest{
			index:    0,
			name:     s.cfg.LexicalBackend,
			weight:   s.cfg.LexicalWeight,
			searcher: s.lexical,
		})
	case SemanticOnly:
		if s.semantic == nil {
			return nil, errors.New("semantic searcher is nil")
		}
		requests = append(requests, backendRequest{
			index:    0,
			name:     s.cfg.SemanticBackend,
			weight:   s.cfg.SemanticWeight,
			searcher: s.semantic,
		})
	case Hybrid:
		if s.lexical != nil {
			requests = append(requests, backendRequest{
				index:    len(requests),
				name:     s.cfg.LexicalBackend,
				weight:   s.cfg.LexicalWeight,
				searcher: s.lexical,
			})
		}
		if s.semantic != nil {
			requests = append(requests, backendRequest{
				index:    len(requests),
				name:     s.cfg.SemanticBackend,
				weight:   s.cfg.SemanticWeight,
				searcher: s.semantic,
			})
		}
		if len(requests) == 0 {
			return nil, errors.New("no search backends configured")
		}
	default:
		return nil, errors.New("unknown search mode")
	}

	results := make([]ResultList, len(requests))
	errs := make(chan error, len(requests))
	var wg sync.WaitGroup

	for _, backend := range requests {
		backend := backend
		wg.Add(1)
		go func() {
			defer wg.Done()
			backendResults, err := backend.searcher.Search(ctx, req)
			if err != nil {
				errs <- err
				return
			}
			results[backend.index] = ResultList{
				Backend: backend.name,
				Weight:  backend.weight,
				Results: backendResults,
			}
		}()
	}

	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

type backendRequest struct {
	index    int
	name     string
	weight   float64
	searcher BackendSearcher
}

func withSearcherDefaults(cfg SearcherConfig) SearcherConfig {
	defaults := DefaultSearcherConfig()
	if cfg.Fusioner == nil {
		cfg.Fusioner = defaults.Fusioner
	}
	if cfg.LexicalBackend == "" {
		cfg.LexicalBackend = defaults.LexicalBackend
	}
	if cfg.SemanticBackend == "" {
		cfg.SemanticBackend = defaults.SemanticBackend
	}
	return cfg
}
