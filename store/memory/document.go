package memory

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/SCKelemen/codesearch/store"
)

// DocumentStore is an in-memory implementation of store.DocumentStore.
type DocumentStore struct {
	mu   sync.RWMutex
	docs map[string]store.Document
}

// NewDocumentStore creates an empty in-memory document store.
func NewDocumentStore() *DocumentStore {
	return &DocumentStore{docs: make(map[string]store.Document)}
}

func (s *DocumentStore) Put(_ context.Context, doc store.Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.docs[doc.ID] = cloneDocument(doc)
	return nil
}

func (s *DocumentStore) Lookup(_ context.Context, id string, opts ...store.LookupOption) (*store.Document, error) {
	options := store.ResolveLookupOptions(opts...)

	s.mu.RLock()
	defer s.mu.RUnlock()

	doc, ok := s.docs[id]
	if !ok || !matchesDocumentFilter(doc, options.Filter) {
		return nil, nil
	}
	clone := cloneDocument(doc)
	return &clone, nil
}

func (s *DocumentStore) List(_ context.Context, opts ...store.ListOption) ([]store.Document, string, error) {
	options := store.ResolveListOptions(opts...)

	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]store.Document, 0, len(s.docs))
	for _, doc := range s.docs {
		if matchesDocumentFilter(doc, options.Filter) {
			items = append(items, cloneDocument(doc))
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].ID != items[j].ID {
			return items[i].ID < items[j].ID
		}
		return items[i].Path < items[j].Path
	})
	return applyPage(items, options.Cursor, options.Limit)
}

func (s *DocumentStore) Search(_ context.Context, query string, opts ...store.SearchOption) ([]store.Document, error) {
	options := store.ResolveSearchOptions(opts...)

	s.mu.RLock()
	defer s.mu.RUnlock()

	items := make([]store.Document, 0, len(s.docs))
	for _, doc := range s.docs {
		if !matchesDocumentFilter(doc, options.Filter) {
			continue
		}
		if query != "" && !matchesDocumentQuery(doc, query) {
			continue
		}
		items = append(items, cloneDocument(doc))
	}
	sort.Slice(items, func(i, j int) bool {
		leftScore := pathQueryScore(items[i], query)
		rightScore := pathQueryScore(items[j], query)
		if leftScore != rightScore {
			return leftScore < rightScore
		}
		if items[i].Path != items[j].Path {
			return items[i].Path < items[j].Path
		}
		return items[i].ID < items[j].ID
	})
	limit := options.Limit
	if limit <= 0 {
		limit = len(items)
	}
	return applySearchPage(items, options.Cursor, limit)
}

func (s *DocumentStore) Delete(_ context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.docs, id)
	return nil
}

func matchesDocumentFilter(doc store.Document, filter store.Filter) bool {
	if filter.RepositoryID != "" && doc.RepositoryID != filter.RepositoryID {
		return false
	}
	if filter.Branch != "" && doc.Branch != filter.Branch {
		return false
	}
	if filter.PathPrefix != "" && !strings.HasPrefix(doc.Path, filter.PathPrefix) {
		return false
	}
	if filter.Language != "" && doc.Language != filter.Language {
		return false
	}
	if filter.DocumentID != "" && doc.ID != filter.DocumentID {
		return false
	}
	return matchesMetadata(doc.Metadata, filter.Metadata)
}

func matchesDocumentQuery(doc store.Document, query string) bool {
	return containsFold(doc.ID, query) ||
		containsFold(doc.Path, query) ||
		containsFold(doc.RepositoryID, query) ||
		containsFold(doc.Language, query) ||
		containsFold(string(doc.Content), query)
}

func pathQueryScore(doc store.Document, query string) int {
	if query == "" {
		return 0
	}
	needle := strings.ToLower(query)
	switch {
	case strings.EqualFold(doc.Path, query):
		return 0
	case strings.Contains(strings.ToLower(doc.Path), needle):
		return 1
	case strings.Contains(strings.ToLower(string(doc.Content)), needle):
		return 2
	default:
		return 3
	}
}

var _ store.DocumentStore = (*DocumentStore)(nil)
