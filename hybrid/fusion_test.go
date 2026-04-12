package hybrid

import (
	"context"
	"math"
	"testing"
)

func TestRRF(t *testing.T) {
	t.Parallel()

	fusioner := RRF{}
	results, err := fusioner.Fuse(context.Background(),
		ResultList{
			Backend: "lexical",
			Results: []SearchResult{
				{DocumentID: "doc-1", Score: 10, Snippet: "lexical doc-1"},
				{DocumentID: "doc-2", Score: 9, Snippet: "lexical doc-2"},
				{DocumentID: "doc-3", Score: 8, Snippet: "lexical doc-3"},
			},
		},
		ResultList{
			Backend: "semantic",
			Results: []SearchResult{
				{DocumentID: "doc-2", Score: 0.95, Snippet: "semantic doc-2"},
				{DocumentID: "doc-3", Score: 0.8, Snippet: "semantic doc-3"},
				{DocumentID: "doc-1", Score: 0.6, Snippet: "semantic doc-1"},
			},
		},
	)
	if err != nil {
		t.Fatalf("Fuse: %v", err)
	}

	if len(results) != 3 {
		t.Fatalf("expected 3 fused results, got %d", len(results))
	}

	wantOrder := []string{"doc-2", "doc-1", "doc-3"}
	for i, want := range wantOrder {
		if results[i].DocumentID != want {
			t.Fatalf("results[%d] = %q, want %q", i, results[i].DocumentID, want)
		}
	}

	wantTopScore := 1.0/61.0 + 1.0/62.0
	if math.Abs(results[0].Score-wantTopScore) > 1e-12 {
		t.Fatalf("top score = %f, want %f", results[0].Score, wantTopScore)
	}
	if got := results[0].BackendScores["lexical"]; got != 9 {
		t.Fatalf("lexical backend score = %f, want 9", got)
	}
	if got := results[0].BackendScores["semantic"]; got != 0.95 {
		t.Fatalf("semantic backend score = %f, want 0.95", got)
	}
}

func TestWeightedSum(t *testing.T) {
	t.Parallel()

	fusioner := WeightedSum{}
	results, err := fusioner.Fuse(context.Background(),
		ResultList{
			Backend: "lexical",
			Weight:  0.3,
			Results: []SearchResult{
				{DocumentID: "doc-a", Score: 0.9, Snippet: "lexical doc-a"},
				{DocumentID: "doc-b", Score: 0.6, Snippet: "lexical doc-b"},
			},
		},
		ResultList{
			Backend: "semantic",
			Weight:  0.7,
			Results: []SearchResult{
				{DocumentID: "doc-b", Score: 0.8, Snippet: "semantic doc-b"},
				{DocumentID: "doc-a", Score: 0.2, Snippet: "semantic doc-a"},
			},
		},
	)
	if err != nil {
		t.Fatalf("Fuse: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 fused results, got %d", len(results))
	}
	if results[0].DocumentID != "doc-b" {
		t.Fatalf("top document = %q, want doc-b", results[0].DocumentID)
	}
	if math.Abs(results[0].Score-0.7) > 1e-12 {
		t.Fatalf("doc-b score = %f, want 0.7", results[0].Score)
	}
	if results[1].DocumentID != "doc-a" {
		t.Fatalf("second document = %q, want doc-a", results[1].DocumentID)
	}
	if math.Abs(results[1].Score-0.3) > 1e-12 {
		t.Fatalf("doc-a score = %f, want 0.3", results[1].Score)
	}
}
