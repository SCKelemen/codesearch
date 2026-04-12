package store

import (
	"context"
	"time"
)

// DistanceMetric selects how vectors are compared.
type DistanceMetric string

const (
	DistanceMetricCosine     DistanceMetric = "cosine"
	DistanceMetricDotProduct DistanceMetric = "dot_product"
	DistanceMetricEuclidean  DistanceMetric = "euclidean"
)

// StoredVector is a vector embedding persisted in the index.
type StoredVector struct {
	ID           string
	DocumentID   string
	RepositoryID string
	Branch       string
	Path         string
	Model        string
	Values       []float32
	Metadata     map[string]string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// VectorResult is a ranked result returned from a vector search.
type VectorResult struct {
	Vector   StoredVector
	Distance float32
	Score    float32
	Rank     int
}

// VectorStore stores and searches vector embeddings.
type VectorStore interface {
	// Put creates or replaces a stored vector.
	Put(ctx context.Context, vector StoredVector) error

	// Lookup returns a vector by ID.
	Lookup(ctx context.Context, id string, opts ...LookupOption) (*StoredVector, error)

	// List returns vectors and the next cursor.
	// An empty next cursor means there are no more results.
	List(ctx context.Context, opts ...ListOption) ([]StoredVector, string, error)

	// Search returns the top-k nearest vectors using the requested distance metric.
	Search(ctx context.Context, query []float32, k int, metric DistanceMetric, opts ...SearchOption) ([]VectorResult, error)

	// Delete removes a vector by ID.
	Delete(ctx context.Context, id string) error
}
