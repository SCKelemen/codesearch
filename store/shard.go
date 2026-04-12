package store

import (
	"context"
	"time"
)

// Tier identifies the storage tier for a shard.
type Tier string

const (
	// T0 is the hottest tier for actively queried, in-memory shards.
	T0 Tier = "t0"

	// T1 is the warm tier for locally persisted shards with low latency.
	T1 Tier = "t1"

	// T2 is the cold distributed tier for shared remote storage.
	T2 Tier = "t2"

	// T3 is the archive tier for the lowest-cost, highest-latency storage.
	T3 Tier = "t3"
)

// ShardMeta describes a physical shard of index data.
type ShardMeta struct {
	RepositoryID string
	Branch       string
	FileCount    int64
	ByteSize     int64
	CreatedAt    time.Time
	Tier         Tier
}

// ShardSearcher defines the read operations supported by a shard.
type ShardSearcher interface {
	SearchDocuments(ctx context.Context, query string, opts ...SearchOption) ([]Document, error)
	SearchTrigrams(ctx context.Context, trigrams []Trigram, opts ...SearchOption) ([]PostingResult, error)
	SearchVectors(ctx context.Context, query []float32, k int, metric DistanceMetric, opts ...SearchOption) ([]VectorResult, error)
	SearchSymbols(ctx context.Context, query string, opts ...SearchOption) ([]Symbol, error)
}

// IndexShard is a serialized, searchable unit of index storage.
type IndexShard interface {
	ShardSearcher

	Meta() ShardMeta
	Documents() DocumentStore
	Trigrams() TrigramStore
	Vectors() VectorStore
	Symbols() SymbolStore
	MarshalBinary() ([]byte, error)
}

// ShardBuilder incrementally constructs an index shard.
type ShardBuilder interface {
	SetMeta(meta ShardMeta)
	AddDocument(ctx context.Context, doc Document) error
	AddPostingList(ctx context.Context, list PostingList) error
	AddVector(ctx context.Context, vector StoredVector) error
	AddSymbol(ctx context.Context, symbol Symbol) error
	AddReference(ctx context.Context, ref Reference) error
	Build(ctx context.Context) (IndexShard, error)
}
