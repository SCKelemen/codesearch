package file

import (
	"context"

	"github.com/SCKelemen/codesearch/store"
	"github.com/SCKelemen/codesearch/store/memory"
)

const symbolFilename = "symbols.gob"

type symbolState struct {
	Symbols    []store.Symbol
	References []store.Reference
}

// SymbolStore provides file-backed persistent storage for symbols and references.
type SymbolStore struct {
	memory *memory.SymbolStore
	disk   *persistence
}

// NewSymbolStore creates a new file-backed symbol store in the given directory.
func NewSymbolStore(dir string, options ...Option) (*SymbolStore, error) {
	memoryStore := memory.NewSymbolStore()
	store := &SymbolStore{memory: memoryStore}

	disk, err := newPersistence(dir, symbolFilename, store.saveSnapshot, options...)
	if err != nil {
		return nil, err
	}
	store.disk = disk

	var state symbolState
	if _, err := disk.load(&state); err != nil {
		_ = disk.Close()
		return nil, err
	}
	if err := loadSymbols(memoryStore, state); err != nil {
		_ = disk.Close()
		return nil, err
	}
	return store, nil
}

func (s *SymbolStore) Put(ctx context.Context, symbol store.Symbol) error {
	if err := s.memory.Put(ctx, symbol); err != nil {
		return err
	}
	return s.disk.markDirty()
}

func (s *SymbolStore) PutReference(ctx context.Context, ref store.Reference) error {
	if err := s.memory.PutReference(ctx, ref); err != nil {
		return err
	}
	return s.disk.markDirty()
}

func (s *SymbolStore) Lookup(ctx context.Context, id string, opts ...store.LookupOption) (*store.Symbol, error) {
	return s.memory.Lookup(ctx, id, opts...)
}

func (s *SymbolStore) List(ctx context.Context, opts ...store.ListOption) ([]store.Symbol, string, error) {
	return s.memory.List(ctx, opts...)
}

func (s *SymbolStore) Search(ctx context.Context, query string, opts ...store.SearchOption) ([]store.Symbol, error) {
	return s.memory.Search(ctx, query, opts...)
}

func (s *SymbolStore) References(ctx context.Context, symbolID string, opts ...store.ListOption) ([]store.Reference, string, error) {
	return s.memory.References(ctx, symbolID, opts...)
}

func (s *SymbolStore) Delete(ctx context.Context, id string) error {
	if err := s.memory.Delete(ctx, id); err != nil {
		return err
	}
	return s.disk.markDirty()
}

func (s *SymbolStore) Flush() error {
	return s.disk.Flush()
}

func (s *SymbolStore) Close() error {
	return s.disk.Close()
}

func (s *SymbolStore) saveSnapshot() error {
	ctx := context.Background()
	symbols, _, err := s.memory.List(ctx)
	if err != nil {
		return err
	}

	references := make([]store.Reference, 0)
	for _, symbol := range symbols {
		refs, _, err := s.memory.References(ctx, symbol.ID)
		if err != nil {
			return err
		}
		references = append(references, refs...)
	}

	return saveGobFile(s.disk.path, symbolState{Symbols: symbols, References: references})
}

func loadSymbols(memoryStore *memory.SymbolStore, state symbolState) error {
	ctx := context.Background()
	for _, symbol := range state.Symbols {
		if err := memoryStore.Put(ctx, symbol); err != nil {
			return err
		}
	}
	for _, ref := range state.References {
		if err := memoryStore.PutReference(ctx, ref); err != nil {
			return err
		}
	}
	return nil
}
