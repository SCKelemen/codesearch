package file

import (
	"context"

	"github.com/SCKelemen/codesearch/store"
	"github.com/SCKelemen/codesearch/store/memory"
)

const trigramFilename = "trigrams.gob"

type trigramState struct {
	PostingLists []store.PostingList
}

type TrigramStore struct {
	memory *memory.TrigramStore
	disk   *persistence
}

func NewTrigramStore(dir string, options ...Option) (*TrigramStore, error) {
	memoryStore := memory.NewTrigramStore()
	store := &TrigramStore{memory: memoryStore}

	disk, err := newPersistence(dir, trigramFilename, store.saveSnapshot, options...)
	if err != nil {
		return nil, err
	}
	store.disk = disk

	var state trigramState
	if _, err := disk.load(&state); err != nil {
		_ = disk.Close()
		return nil, err
	}
	if err := loadPostingLists(memoryStore, state.PostingLists); err != nil {
		_ = disk.Close()
		return nil, err
	}
	return store, nil
}

func (s *TrigramStore) Put(ctx context.Context, list store.PostingList) error {
	if err := s.memory.Put(ctx, list); err != nil {
		return err
	}
	return s.disk.markDirty()
}

func (s *TrigramStore) Lookup(ctx context.Context, trigram store.Trigram, opts ...store.LookupOption) (*store.PostingList, error) {
	return s.memory.Lookup(ctx, trigram, opts...)
}

func (s *TrigramStore) List(ctx context.Context, opts ...store.ListOption) ([]store.PostingList, string, error) {
	return s.memory.List(ctx, opts...)
}

func (s *TrigramStore) Search(ctx context.Context, trigrams []store.Trigram, opts ...store.SearchOption) ([]store.PostingResult, error) {
	return s.memory.Search(ctx, trigrams, opts...)
}

func (s *TrigramStore) Delete(ctx context.Context, trigram store.Trigram) error {
	if err := s.memory.Delete(ctx, trigram); err != nil {
		return err
	}
	return s.disk.markDirty()
}

func (s *TrigramStore) Flush() error {
	return s.disk.Flush()
}

func (s *TrigramStore) Close() error {
	return s.disk.Close()
}

func (s *TrigramStore) saveSnapshot() error {
	postingLists, _, err := s.memory.List(context.Background())
	if err != nil {
		return err
	}
	return saveGobFile(s.disk.path, trigramState{PostingLists: postingLists})
}

func loadPostingLists(memoryStore *memory.TrigramStore, postingLists []store.PostingList) error {
	ctx := context.Background()
	for _, postingList := range postingLists {
		if err := memoryStore.Put(ctx, postingList); err != nil {
			return err
		}
	}
	return nil
}
