package file

import (
	"context"

	"github.com/SCKelemen/codesearch/store"
	"github.com/SCKelemen/codesearch/store/memory"
)

const documentFilename = "documents.gob"

type documentState struct {
	Documents []store.Document
}

type DocumentStore struct {
	memory *memory.DocumentStore
	disk   *persistence
}

func NewDocumentStore(dir string, options ...Option) (*DocumentStore, error) {
	memoryStore := memory.NewDocumentStore()
	store := &DocumentStore{memory: memoryStore}

	disk, err := newPersistence(dir, documentFilename, store.saveSnapshot, options...)
	if err != nil {
		return nil, err
	}
	store.disk = disk

	var state documentState
	if _, err := disk.load(&state); err != nil {
		_ = disk.Close()
		return nil, err
	}
	if err := loadDocuments(memoryStore, state.Documents); err != nil {
		_ = disk.Close()
		return nil, err
	}
	return store, nil
}

func (s *DocumentStore) Put(ctx context.Context, doc store.Document) error {
	if err := s.memory.Put(ctx, doc); err != nil {
		return err
	}
	return s.disk.markDirty()
}

func (s *DocumentStore) Lookup(ctx context.Context, id string, opts ...store.LookupOption) (*store.Document, error) {
	return s.memory.Lookup(ctx, id, opts...)
}

func (s *DocumentStore) List(ctx context.Context, opts ...store.ListOption) ([]store.Document, string, error) {
	return s.memory.List(ctx, opts...)
}

func (s *DocumentStore) Search(ctx context.Context, query string, opts ...store.SearchOption) ([]store.Document, error) {
	return s.memory.Search(ctx, query, opts...)
}

func (s *DocumentStore) Delete(ctx context.Context, id string) error {
	if err := s.memory.Delete(ctx, id); err != nil {
		return err
	}
	return s.disk.markDirty()
}

func (s *DocumentStore) Flush() error {
	return s.disk.Flush()
}

func (s *DocumentStore) Close() error {
	return s.disk.Close()
}

func (s *DocumentStore) saveSnapshot() error {
	documents, _, err := s.memory.List(context.Background())
	if err != nil {
		return err
	}
	return saveGobFile(s.disk.path, documentState{Documents: documents})
}

func loadDocuments(memoryStore *memory.DocumentStore, documents []store.Document) error {
	ctx := context.Background()
	for _, doc := range documents {
		if err := memoryStore.Put(ctx, doc); err != nil {
			return err
		}
	}
	return nil
}
