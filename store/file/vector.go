package file

import (
	"context"

	"github.com/SCKelemen/codesearch/store"
	"github.com/SCKelemen/codesearch/store/memory"
)

const vectorFilename = "vectors.gob"

type vectorState struct {
	Vectors []store.StoredVector
}

// VectorStore provides file-backed persistent storage for embedding vectors.
type VectorStore struct {
	memory *memory.VectorStore
	disk   *persistence
}

// NewVectorStore creates a new file-backed vector store in the given directory.
func NewVectorStore(dir string, options ...Option) (*VectorStore, error) {
	memoryStore := memory.NewVectorStore()
	store := &VectorStore{memory: memoryStore}

	disk, err := newPersistence(dir, vectorFilename, store.saveSnapshot, options...)
	if err != nil {
		return nil, err
	}
	store.disk = disk

	var state vectorState
	if _, err := disk.load(&state); err != nil {
		_ = disk.Close()
		return nil, err
	}
	if err := loadVectors(memoryStore, state.Vectors); err != nil {
		_ = disk.Close()
		return nil, err
	}
	return store, nil
}

func (s *VectorStore) Put(ctx context.Context, vector store.StoredVector) error {
	if err := s.memory.Put(ctx, vector); err != nil {
		return err
	}
	return s.disk.markDirty()
}

func (s *VectorStore) Lookup(ctx context.Context, id string, opts ...store.LookupOption) (*store.StoredVector, error) {
	return s.memory.Lookup(ctx, id, opts...)
}

func (s *VectorStore) List(ctx context.Context, opts ...store.ListOption) ([]store.StoredVector, string, error) {
	return s.memory.List(ctx, opts...)
}

func (s *VectorStore) Search(ctx context.Context, query []float32, k int, metric store.DistanceMetric, opts ...store.SearchOption) ([]store.VectorResult, error) {
	return s.memory.Search(ctx, query, k, metric, opts...)
}

func (s *VectorStore) Delete(ctx context.Context, id string) error {
	if err := s.memory.Delete(ctx, id); err != nil {
		return err
	}
	return s.disk.markDirty()
}

func (s *VectorStore) Flush() error {
	return s.disk.Flush()
}

func (s *VectorStore) Close() error {
	return s.disk.Close()
}

func (s *VectorStore) saveSnapshot() error {
	vectors, _, err := s.memory.List(context.Background())
	if err != nil {
		return err
	}
	return saveGobFile(s.disk.path, vectorState{Vectors: vectors})
}

func loadVectors(memoryStore *memory.VectorStore, vectors []store.StoredVector) error {
	ctx := context.Background()
	for _, vector := range vectors {
		if err := memoryStore.Put(ctx, vector); err != nil {
			return err
		}
	}
	return nil
}
