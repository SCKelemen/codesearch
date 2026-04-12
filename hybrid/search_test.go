package hybrid

import (
	"context"
	"errors"
	"testing"
	"time"
)

type searchOutcome struct {
	results []FusedResult
	err     error
}

func TestHybridSearcherSearchRunsBackendsInParallel(t *testing.T) {
	t.Parallel()

	release := make(chan struct{})
	lexical := &mockBackend{
		results: []SearchResult{
			{DocumentID: "shared", Score: 10, Snippet: "lexical shared"},
			{DocumentID: "lexical-only", Score: 5, Snippet: "lexical only"},
		},
		started: make(chan struct{}),
		release: release,
	}
	semantic := &mockBackend{
		results: []SearchResult{
			{DocumentID: "shared", Score: 0.8, Snippet: "semantic shared"},
			{DocumentID: "semantic-only", Score: 0.3, Snippet: "semantic only"},
		},
		started: make(chan struct{}),
		release: release,
	}

	searcher := NewHybridSearcher(lexical, semantic, SearcherConfig{
		Fusioner:        WeightedSum{},
		LexicalWeight:   1,
		SemanticWeight:  1,
		LexicalBackend:  "lexical",
		SemanticBackend: "semantic",
	})

	outcomeCh := make(chan searchOutcome, 1)
	go func() {
		results, err := searcher.Search(context.Background(), SearchRequest{Mode: Hybrid})
		outcomeCh <- searchOutcome{results: results, err: err}
	}()

	waitForStart(t, lexical.started)
	waitForStart(t, semantic.started)
	close(release)

	outcome := waitForOutcome(t, outcomeCh)
	if outcome.err != nil {
		t.Fatalf("Search: %v", outcome.err)
	}
	if lexical.calls != 1 {
		t.Fatalf("lexical calls = %d, want 1", lexical.calls)
	}
	if semantic.calls != 1 {
		t.Fatalf("semantic calls = %d, want 1", semantic.calls)
	}
	if len(outcome.results) != 3 {
		t.Fatalf("expected 3 fused results, got %d", len(outcome.results))
	}
	if outcome.results[0].DocumentID != "shared" {
		t.Fatalf("top document = %q, want shared", outcome.results[0].DocumentID)
	}
}

func TestHybridSearcherRespectsModeAndMaxResults(t *testing.T) {
	t.Parallel()

	lexical := &mockBackend{results: []SearchResult{{DocumentID: "doc-1", Score: 2}, {DocumentID: "doc-2", Score: 1}}}
	semantic := &mockBackend{results: []SearchResult{{DocumentID: "doc-3", Score: 9}}}
	searcher := NewHybridSearcher(lexical, semantic, DefaultSearcherConfig())

	results, err := searcher.Search(context.Background(), SearchRequest{Mode: LexicalOnly, MaxResults: 1})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if lexical.calls != 1 {
		t.Fatalf("lexical calls = %d, want 1", lexical.calls)
	}
	if semantic.calls != 0 {
		t.Fatalf("semantic calls = %d, want 0", semantic.calls)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].DocumentID != "doc-1" {
		t.Fatalf("results[0] = %q, want doc-1", results[0].DocumentID)
	}
}

func TestHybridSearcherPropagatesBackendErrors(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("backend failed")
	searcher := NewHybridSearcher(&mockBackend{err: wantErr}, &mockBackend{}, DefaultSearcherConfig())

	_, err := searcher.Search(context.Background(), SearchRequest{Mode: LexicalOnly})
	if !errors.Is(err, wantErr) {
		t.Fatalf("Search error = %v, want %v", err, wantErr)
	}
}

type mockBackend struct {
	results []SearchResult
	err     error
	started chan struct{}
	release <-chan struct{}
	calls   int
}

func (m *mockBackend) Search(_ context.Context, _ SearchRequest) ([]SearchResult, error) {
	m.calls++
	if m.started != nil {
		close(m.started)
	}
	if m.release != nil {
		<-m.release
	}
	if m.err != nil {
		return nil, m.err
	}
	return m.results, nil
}

func waitForStart(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for backend start")
	}
}

func waitForOutcome(t *testing.T, ch <-chan searchOutcome) searchOutcome {
	t.Helper()
	select {
	case outcome := <-ch:
		return outcome
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for search completion")
		return searchOutcome{}
	}
}
