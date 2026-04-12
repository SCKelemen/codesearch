package embedding

import (
	"context"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type trackingEmbedder struct {
	mu            sync.Mutex
	calls         [][]string
	callStarts    []time.Time
	concurrent    int
	maxConcurrent int
	sleep         time.Duration
}

func (t *trackingEmbedder) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	t.mu.Lock()
	t.calls = append(t.calls, append([]string(nil), inputs...))
	t.callStarts = append(t.callStarts, time.Now())
	t.concurrent++
	if t.concurrent > t.maxConcurrent {
		t.maxConcurrent = t.concurrent
	}
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		t.concurrent--
		t.mu.Unlock()
	}()

	if t.sleep > 0 {
		select {
		case <-time.After(t.sleep):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}

	vectors := make([][]float32, len(inputs))
	for i, input := range inputs {
		vectors[i] = []float32{float32(len(input))}
	}
	return vectors, nil
}

func (t *trackingEmbedder) Dimensions() int { return 1 }
func (t *trackingEmbedder) Model() string   { return "tracking" }

func TestBatchEmbedderBatchesAndPreservesOrder(t *testing.T) {
	inner := &trackingEmbedder{}
	embedder := NewBatchEmbedder(inner, BatchConfig{BatchSize: 2, MaxConcurrent: 1})

	inputs := []string{"a", "bb", "ccc", "dddd", "eeeee"}
	vectors, err := embedder.Embed(context.Background(), inputs)
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}

	wantVectors := [][]float32{{1}, {2}, {3}, {4}, {5}}
	if !reflect.DeepEqual(vectors, wantVectors) {
		t.Fatalf("unexpected vectors: got %v want %v", vectors, wantVectors)
	}

	inner.mu.Lock()
	gotCalls := append([][]string(nil), inner.calls...)
	inner.mu.Unlock()

	if len(gotCalls) != 3 {
		t.Fatalf("expected 3 batch calls, got %d", len(gotCalls))
	}

	gotSets := make(map[string]struct{}, len(gotCalls))
	for _, call := range gotCalls {
		gotSets[strings.Join(call, ",")] = struct{}{}
	}
	for _, want := range []string{"a,bb", "ccc,dddd", "eeeee"} {
		if _, ok := gotSets[want]; !ok {
			t.Fatalf("missing expected batch %q in %v", want, gotCalls)
		}
	}
}

func TestBatchEmbedderRespectsMaxConcurrent(t *testing.T) {
	inner := &trackingEmbedder{sleep: 40 * time.Millisecond}
	embedder := NewBatchEmbedder(inner, BatchConfig{BatchSize: 1, MaxConcurrent: 2})

	_, err := embedder.Embed(context.Background(), []string{"a", "b", "c", "d"})
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}

	inner.mu.Lock()
	defer inner.mu.Unlock()
	if inner.maxConcurrent > 2 {
		t.Fatalf("expected at most 2 concurrent calls, got %d", inner.maxConcurrent)
	}
	if inner.maxConcurrent != 2 {
		t.Fatalf("expected concurrency limit to be reached, got %d", inner.maxConcurrent)
	}
}

func TestBatchEmbedderAppliesRateLimit(t *testing.T) {
	inner := &trackingEmbedder{}
	embedder := NewBatchEmbedder(inner, BatchConfig{BatchSize: 1, MaxConcurrent: 3, RateLimit: 30 * time.Millisecond})

	_, err := embedder.Embed(context.Background(), []string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("Embed returned error: %v", err)
	}

	inner.mu.Lock()
	starts := append([]time.Time(nil), inner.callStarts...)
	inner.mu.Unlock()

	if len(starts) != 3 {
		t.Fatalf("expected 3 calls, got %d", len(starts))
	}

	for i := 1; i < len(starts); i++ {
		delta := starts[i].Sub(starts[i-1])
		if delta < 20*time.Millisecond {
			t.Fatalf("expected at least 20ms between calls, got %v", delta)
		}
	}
}
