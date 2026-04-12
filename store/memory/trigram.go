package memory

import (
	"context"
	"sort"
	"sync"

	"github.com/SCKelemen/codesearch/store"
)

// TrigramStore is an in-memory implementation of store.TrigramStore.
type TrigramStore struct {
	mu       sync.RWMutex
	postings map[store.Trigram][]string
}

// NewTrigramStore creates an empty in-memory trigram store.
func NewTrigramStore() *TrigramStore {
	return &TrigramStore{postings: make(map[store.Trigram][]string)}
}

func (s *TrigramStore) Put(_ context.Context, list store.PostingList) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.postings[list.Trigram] = uniqueSortedStrings(list.DocumentIDs)
	return nil
}

func (s *TrigramStore) Lookup(_ context.Context, trigram store.Trigram, opts ...store.LookupOption) (*store.PostingList, error) {
	options := store.ResolveLookupOptions(opts...)

	s.mu.RLock()
	defer s.mu.RUnlock()

	docIDs, ok := s.postings[trigram]
	if !ok {
		return nil, nil
	}
	filtered := filterPostingDocIDs(docIDs, options.Filter)
	if len(filtered) == 0 && options.Filter.DocumentID != "" {
		return nil, nil
	}
	list := store.PostingList{Trigram: trigram, DocumentIDs: filtered}
	return &list, nil
}

func (s *TrigramStore) List(_ context.Context, opts ...store.ListOption) ([]store.PostingList, string, error) {
	options := store.ResolveListOptions(opts...)

	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]store.PostingList, 0, len(s.postings))
	for trigram, docIDs := range s.postings {
		filtered := filterPostingDocIDs(docIDs, options.Filter)
		if len(filtered) == 0 && options.Filter.DocumentID != "" {
			continue
		}
		items = append(items, store.PostingList{Trigram: trigram, DocumentIDs: filtered})
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Trigram < items[j].Trigram
	})
	return applyPage(items, options.Cursor, options.Limit)
}

func (s *TrigramStore) Search(_ context.Context, trigrams []store.Trigram, opts ...store.SearchOption) ([]store.PostingResult, error) {
	options := store.ResolveSearchOptions(opts...)

	s.mu.RLock()
	defer s.mu.RUnlock()

	counts := make(map[string]int)
	for _, trigram := range trigrams {
		for _, docID := range s.postings[trigram] {
			if options.Filter.DocumentID != "" && docID != options.Filter.DocumentID {
				continue
			}
			counts[docID]++
		}
	}

	results := make([]store.PostingResult, 0, len(counts))
	for docID, matched := range counts {
		results = append(results, store.PostingResult{DocumentID: docID, MatchedTrigrams: matched, CandidateTrigrams: len(trigrams)})
	}
	sort.Slice(results, func(i, j int) bool {
		if results[i].MatchedTrigrams != results[j].MatchedTrigrams {
			return results[i].MatchedTrigrams > results[j].MatchedTrigrams
		}
		return results[i].DocumentID < results[j].DocumentID
	})
	limit := options.Limit
	if limit <= 0 {
		limit = len(results)
	}
	return applySearchPage(results, options.Cursor, limit)
}

func (s *TrigramStore) Delete(_ context.Context, trigram store.Trigram) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.postings, trigram)
	return nil
}

func filterPostingDocIDs(docIDs []string, filter store.Filter) []string {
	if filter.DocumentID == "" {
		return copyStringSlice(docIDs)
	}
	filtered := make([]string, 0, 1)
	for _, docID := range docIDs {
		if docID == filter.DocumentID {
			filtered = append(filtered, docID)
		}
	}
	return filtered
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

var _ store.TrigramStore = (*TrigramStore)(nil)
