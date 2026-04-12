package memory

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/SCKelemen/codesearch/store"
)

// SymbolStore is an in-memory implementation of store.SymbolStore.
type SymbolStore struct {
	mu         sync.RWMutex
	symbols    map[string]store.Symbol
	nameIndex  map[string]map[string]struct{}
	references map[string]map[string]store.Reference
}

// NewSymbolStore creates an empty in-memory symbol store.
func NewSymbolStore() *SymbolStore {
	return &SymbolStore{symbols: make(map[string]store.Symbol), nameIndex: make(map[string]map[string]struct{}), references: make(map[string]map[string]store.Reference)}
}

func (s *SymbolStore) Put(_ context.Context, symbol store.Symbol) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.symbols[symbol.ID]; ok {
		s.removeNameIndex(existing)
	}
	clone := cloneSymbol(symbol)
	s.symbols[symbol.ID] = clone
	s.addNameIndex(clone)
	return nil
}

func (s *SymbolStore) PutReference(_ context.Context, ref store.Reference) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.references[ref.SymbolID]; !ok {
		s.references[ref.SymbolID] = make(map[string]store.Reference)
	}
	s.references[ref.SymbolID][referenceKey(ref)] = cloneReference(ref)
	return nil
}

func (s *SymbolStore) Lookup(_ context.Context, id string, opts ...store.LookupOption) (*store.Symbol, error) {
	options := store.ResolveLookupOptions(opts...)

	s.mu.RLock()
	defer s.mu.RUnlock()

	symbol, ok := s.symbols[id]
	if !ok || !matchesSymbolFilter(symbol, options.Filter) {
		return nil, nil
	}
	clone := cloneSymbol(symbol)
	return &clone, nil
}

func (s *SymbolStore) List(_ context.Context, opts ...store.ListOption) ([]store.Symbol, string, error) {
	options := store.ResolveListOptions(opts...)

	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]store.Symbol, 0, len(s.symbols))
	for _, symbol := range s.symbols {
		if matchesSymbolFilter(symbol, options.Filter) {
			items = append(items, cloneSymbol(symbol))
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return applyPage(items, options.Cursor, options.Limit)
}

func (s *SymbolStore) Search(_ context.Context, query string, opts ...store.SearchOption) ([]store.Symbol, error) {
	options := store.ResolveSearchOptions(opts...)
	needle := strings.ToLower(query)

	s.mu.RLock()
	defer s.mu.RUnlock()

	candidateIDs := make(map[string]struct{})
	if needle == "" {
		for id := range s.symbols {
			candidateIDs[id] = struct{}{}
		}
	} else {
		for id := range s.nameIndex[needle] {
			candidateIDs[id] = struct{}{}
		}
		for id, symbol := range s.symbols {
			if matchesSymbolQuery(symbol, query) {
				candidateIDs[id] = struct{}{}
			}
		}
	}

	items := make([]store.Symbol, 0, len(candidateIDs))
	for id := range candidateIDs {
		symbol := s.symbols[id]
		if !matchesSymbolFilter(symbol, options.Filter) {
			continue
		}
		items = append(items, cloneSymbol(symbol))
	}
	sort.Slice(items, func(i, j int) bool {
		leftRank := symbolRank(items[i], query)
		rightRank := symbolRank(items[j], query)
		if leftRank != rightRank {
			return leftRank < rightRank
		}
		if items[i].Name != items[j].Name {
			return items[i].Name < items[j].Name
		}
		return items[i].ID < items[j].ID
	})
	limit := options.Limit
	if limit <= 0 {
		limit = len(items)
	}
	return applySearchPage(items, options.Cursor, limit)
}

func (s *SymbolStore) References(_ context.Context, symbolID string, opts ...store.ListOption) ([]store.Reference, string, error) {
	options := store.ResolveListOptions(opts...)
	if options.Filter.SymbolID == "" {
		options.Filter.SymbolID = symbolID
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	refs := s.references[symbolID]
	keys := sortedStringKeys(refs)
	items := make([]store.Reference, 0, len(keys))
	for _, key := range keys {
		ref := refs[key]
		if matchesReferenceFilter(ref, options.Filter) {
			items = append(items, cloneReference(ref))
		}
	}
	return applyPage(items, options.Cursor, options.Limit)
}

func (s *SymbolStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if existing, ok := s.symbols[id]; ok {
		s.removeNameIndex(existing)
	}
	delete(s.symbols, id)
	delete(s.references, id)
	return nil
}

func (s *SymbolStore) addNameIndex(symbol store.Symbol) {
	key := strings.ToLower(symbol.Name)
	if _, ok := s.nameIndex[key]; !ok {
		s.nameIndex[key] = make(map[string]struct{})
	}
	s.nameIndex[key][symbol.ID] = struct{}{}
}

func (s *SymbolStore) removeNameIndex(symbol store.Symbol) {
	key := strings.ToLower(symbol.Name)
	ids := s.nameIndex[key]
	delete(ids, symbol.ID)
	if len(ids) == 0 {
		delete(s.nameIndex, key)
	}
}

func matchesSymbolFilter(symbol store.Symbol, filter store.Filter) bool {
	if filter.RepositoryID != "" && symbol.RepositoryID != filter.RepositoryID {
		return false
	}
	if filter.Branch != "" && symbol.Branch != filter.Branch {
		return false
	}
	if filter.PathPrefix != "" && !strings.HasPrefix(symbol.Path, filter.PathPrefix) {
		return false
	}
	if filter.Language != "" && symbol.Language != filter.Language {
		return false
	}
	if filter.DocumentID != "" && symbol.DocumentID != filter.DocumentID {
		return false
	}
	if filter.SymbolID != "" && symbol.ID != filter.SymbolID {
		return false
	}
	if !matchesKinds(symbol.Kind, filter.Kinds) {
		return false
	}
	return matchesMetadata(symbol.Metadata, filter.Metadata)
}

func matchesReferenceFilter(ref store.Reference, filter store.Filter) bool {
	if filter.SymbolID != "" && ref.SymbolID != filter.SymbolID {
		return false
	}
	if filter.DocumentID != "" && ref.DocumentID != filter.DocumentID {
		return false
	}
	if filter.PathPrefix != "" && !strings.HasPrefix(ref.Path, filter.PathPrefix) {
		return false
	}
	return true
}

func matchesSymbolQuery(symbol store.Symbol, query string) bool {
	return containsFold(symbol.ID, query) || containsFold(symbol.Name, query) || containsFold(symbol.Container, query) || containsFold(symbol.Signature, query) || containsFold(symbol.Path, query)
}

func symbolRank(symbol store.Symbol, query string) int {
	if query == "" {
		return 0
	}
	needle := strings.ToLower(query)
	name := strings.ToLower(symbol.Name)
	container := strings.ToLower(symbol.Container)
	signature := strings.ToLower(symbol.Signature)
	switch {
	case name == needle:
		return 0
	case strings.HasPrefix(name, needle):
		return 1
	case strings.Contains(name, needle):
		return 2
	case strings.Contains(container, needle):
		return 3
	case strings.Contains(signature, needle):
		return 4
	default:
		return 5
	}
}

func referenceKey(ref store.Reference) string {
	return fmt.Sprintf("%s:%s:%d:%d:%d:%d:%t", ref.DocumentID, ref.Path, ref.Range.StartLine, ref.Range.StartColumn, ref.Range.EndLine, ref.Range.EndColumn, ref.Definition)
}

var _ store.SymbolStore = (*SymbolStore)(nil)
