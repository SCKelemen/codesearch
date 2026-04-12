package memory

import (
	"context"
	"math"
	"math/rand"
	"sort"
	"strconv"
	"testing"
	"time"

	"github.com/SCKelemen/codesearch/store"
)

func TestVectorStoreCRUDAndFilters(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	vectors := NewVectorStore(WithRandomSeed(7), WithM(8), WithEFConstruction(64), WithEFSearch(32))
	now := time.Now().UTC()

	seed := []store.StoredVector{
		{ID: "vec-1", DocumentID: "doc-1", RepositoryID: "repo-1", Branch: "main", Path: "a.go", Values: []float32{1, 0}, Metadata: map[string]string{"team": "search"}, CreatedAt: now, UpdatedAt: now},
		{ID: "vec-2", DocumentID: "doc-2", RepositoryID: "repo-1", Branch: "main", Path: "b.go", Values: []float32{0.9, 0.1}, Metadata: map[string]string{"team": "search"}, CreatedAt: now, UpdatedAt: now},
		{ID: "vec-3", DocumentID: "doc-3", RepositoryID: "repo-2", Branch: "dev", Path: "c.py", Values: []float32{-1, 0}, Metadata: map[string]string{"team": "ml"}, CreatedAt: now, UpdatedAt: now},
	}
	for _, vector := range seed {
		if err := vectors.Put(ctx, vector); err != nil {
			t.Fatalf("Put(%s) error = %v", vector.ID, err)
		}
	}

	got, err := vectors.Lookup(ctx, "vec-1", store.WithRepositoryID("repo-1"), store.WithDocumentID("doc-1"))
	if err != nil {
		t.Fatalf("Lookup error = %v", err)
	}
	if got == nil || got.ID != "vec-1" {
		t.Fatalf("Lookup = %v, want vec-1", got)
	}

	page, next, err := vectors.List(ctx, store.WithRepositoryID("repo-1"), store.WithLimit(1))
	if err != nil {
		t.Fatalf("List page 1 error = %v", err)
	}
	if len(page) != 1 || page[0].ID != "vec-1" || next != "1" {
		t.Fatalf("List page 1 = (%v, %q), want vec-1 and cursor 1", page, next)
	}

	page, next, err = vectors.List(ctx, store.WithRepositoryID("repo-1"), store.WithCursor(next), store.WithLimit(2))
	if err != nil {
		t.Fatalf("List page 2 error = %v", err)
	}
	if len(page) != 1 || page[0].ID != "vec-2" || next != "" {
		t.Fatalf("List page 2 = (%v, %q), want vec-2 and empty cursor", page, next)
	}

	results, err := vectors.Search(ctx, []float32{1, 0}, 2, store.DistanceMetricCosine, store.WithRepositoryID("repo-1"), store.WithMetadata("team", "search"))
	if err != nil {
		t.Fatalf("Search error = %v", err)
	}
	if len(results) != 2 || results[0].Vector.ID != "vec-1" {
		t.Fatalf("Search results = %v, want vec-1 first", results)
	}

	if err := vectors.Delete(ctx, "vec-1"); err != nil {
		t.Fatalf("Delete error = %v", err)
	}
	got, err = vectors.Lookup(ctx, "vec-1")
	if err != nil {
		t.Fatalf("Lookup after delete error = %v", err)
	}
	if got != nil {
		t.Fatalf("Lookup after delete = %v, want nil", got)
	}
}

func TestVectorStoreHNSWRecallQuality(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	vectors := NewVectorStore(WithRandomSeed(42), WithM(12), WithEFConstruction(128), WithEFSearch(128))
	rng := rand.New(rand.NewSource(99))
	all := make([]store.StoredVector, 0, 200)

	for index := 0; index < 200; index++ {
		values := randomUnitVector(rng, 10)
		vector := store.StoredVector{
			ID:           formatID("vec", index),
			DocumentID:   formatID("doc", index),
			RepositoryID: "repo-1",
			Branch:       "main",
			Path:         formatID("path", index),
			Values:       values,
		}
		all = append(all, vector)
		if err := vectors.Put(ctx, vector); err != nil {
			t.Fatalf("Put(%s) error = %v", vector.ID, err)
		}
	}

	queries := 20
	cosineRecall := 0.0
	cosineTop1 := 0
	euclideanRecall := 0.0
	euclideanTop1 := 0
	for index := 0; index < queries; index++ {
		query := randomUnitVector(rng, 10)
		cosineExpected, cosineFirst := exactTopK(all, query, 10, store.DistanceMetricCosine)
		euclideanExpected, euclideanFirst := exactTopK(all, query, 10, store.DistanceMetricEuclidean)

		cosineResults, err := vectors.Search(ctx, query, 10, store.DistanceMetricCosine)
		if err != nil {
			t.Fatalf("Cosine Search error = %v", err)
		}
		euclideanResults, err := vectors.Search(ctx, query, 10, store.DistanceMetricEuclidean)
		if err != nil {
			t.Fatalf("Euclidean Search error = %v", err)
		}
		if len(cosineResults) == 0 || len(euclideanResults) == 0 {
			t.Fatalf("Expected non-empty search results")
		}

		cosineRecall += recallAtK(cosineExpected, cosineResults)
		euclideanRecall += recallAtK(euclideanExpected, euclideanResults)
		if cosineResults[0].Vector.ID == cosineFirst {
			cosineTop1++
		}
		if euclideanResults[0].Vector.ID == euclideanFirst {
			euclideanTop1++
		}
	}

	cosineRecall /= float64(queries)
	euclideanRecall /= float64(queries)
	if cosineRecall < 0.75 {
		t.Fatalf("cosine recall = %.3f, want >= 0.75", cosineRecall)
	}
	if euclideanRecall < 0.75 {
		t.Fatalf("euclidean recall = %.3f, want >= 0.75", euclideanRecall)
	}
	if cosineTop1 < queries-2 {
		t.Fatalf("cosine top-1 hits = %d, want at least %d", cosineTop1, queries-2)
	}
	if euclideanTop1 < queries-2 {
		t.Fatalf("euclidean top-1 hits = %d, want at least %d", euclideanTop1, queries-2)
	}
}

func randomUnitVector(rng *rand.Rand, dims int) []float32 {
	values := make([]float32, dims)
	var norm float64
	for index := 0; index < dims; index++ {
		value := rng.Float64()*2 - 1
		values[index] = float32(value)
		norm += value * value
	}
	scale := 1 / math.Sqrt(norm)
	for index := range values {
		values[index] *= float32(scale)
	}
	return values
}

func exactTopK(vectors []store.StoredVector, query []float32, k int, metric store.DistanceMetric) (map[string]struct{}, string) {
	type scored struct {
		id       string
		distance float32
	}
	scoredVectors := make([]scored, 0, len(vectors))
	for _, vector := range vectors {
		scoredVectors = append(scoredVectors, scored{id: vector.ID, distance: vectorDistance(query, vector.Values, metric)})
	}
	sort.Slice(scoredVectors, func(i, j int) bool {
		if scoredVectors[i].distance != scoredVectors[j].distance {
			return scoredVectors[i].distance < scoredVectors[j].distance
		}
		return scoredVectors[i].id < scoredVectors[j].id
	})
	out := make(map[string]struct{}, k)
	for _, item := range scoredVectors[:k] {
		out[item.id] = struct{}{}
	}
	return out, scoredVectors[0].id
}

func recallAtK(expected map[string]struct{}, actual []store.VectorResult) float64 {
	if len(expected) == 0 {
		return 1
	}
	matched := 0
	for _, result := range actual {
		if _, ok := expected[result.Vector.ID]; ok {
			matched++
		}
	}
	return float64(matched) / float64(len(expected))
}

func vectorDistance(query []float32, values []float32, metric store.DistanceMetric) float32 {
	switch metric {
	case store.DistanceMetricEuclidean:
		return euclideanDistance(query, values)
	case store.DistanceMetricDotProduct:
		return -dotProduct(query, values)
	case store.DistanceMetricCosine:
		fallthrough
	default:
		return 1 - cosineSimilarity(query, values)
	}
}

func formatID(prefix string, index int) string {
	return prefix + "-" + strconv.Itoa(index)
}
