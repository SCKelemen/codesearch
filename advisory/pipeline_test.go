package advisory

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// MemStore is an in-memory advisory store for testing.
type MemStore struct {
	mu         sync.RWMutex
	advisories map[string]Advisory
	batches    [][]Advisory
}

// NewMemStore returns an empty in-memory advisory store.
func NewMemStore() *MemStore {
	return &MemStore{advisories: make(map[string]Advisory)}
}

// Put stores one advisory in memory.
func (s *MemStore) Put(ctx context.Context, adv Advisory) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.advisories[adv.ID] = adv
	return nil
}

// PutBatch stores a batch of advisories in memory.
func (s *MemStore) PutBatch(ctx context.Context, advs []Advisory) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	batch := append([]Advisory(nil), advs...)
	s.batches = append(s.batches, batch)
	for _, adv := range advs {
		s.advisories[adv.ID] = adv
	}
	return nil
}

// Get returns a stored advisory by ID.
func (s *MemStore) Get(id string) *Advisory {
	s.mu.RLock()
	defer s.mu.RUnlock()
	adv, ok := s.advisories[id]
	if !ok {
		return nil
	}
	copy := adv
	return &copy
}

// Len returns the number of stored advisories.
func (s *MemStore) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.advisories)
}

func (s *MemStore) batchSizes() []int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	sizes := make([]int, 0, len(s.batches))
	for _, batch := range s.batches {
		sizes = append(sizes, len(batch))
	}
	return sizes
}

type mockFeed struct {
	name        string
	advisories  []Advisory
	err         error
	delay       time.Duration
	beforeFetch func(time.Time)
	afterFetch  func()
	calls       atomic.Int32
}

func (f *mockFeed) Name() string {
	return f.name
}

func (f *mockFeed) Fetch(ctx context.Context, since time.Time) ([]Advisory, error) {
	f.calls.Add(1)
	if f.beforeFetch != nil {
		f.beforeFetch(since)
	}
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if f.afterFetch != nil {
		defer f.afterFetch()
	}
	if f.err != nil {
		return nil, f.err
	}
	return append([]Advisory(nil), f.advisories...), nil
}

func TestPipelinePollOnceStoresAdvisories(t *testing.T) {
	t.Parallel()

	store := NewMemStore()
	feed := &mockFeed{
		name: "ghsa",
		advisories: []Advisory{
			{ID: "GHSA-1"},
			{ID: "GHSA-2"},
			{ID: "GHSA-3"},
		},
	}
	pipeline := NewPipeline(store, []Feed{feed}, PipelineOptions{Logger: slog.New(slog.NewTextHandler(nilDiscardWriter{}, nil))})

	if err := pipeline.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if got := store.Len(); got != 3 {
		t.Fatalf("store.Len() = %d, want 3", got)
	}
	if got := pipeline.LastPollTime(feed.Name()); got.IsZero() {
		t.Fatal("LastPollTime() = zero, want updated timestamp")
	}
	if store.Get("GHSA-2") == nil {
		t.Fatal("Get(GHSA-2) = nil, want stored advisory")
	}
}

func TestPipelinePollsFeedsInParallel(t *testing.T) {
	t.Parallel()

	store := NewMemStore()
	started := make(chan string, 2)
	release := make(chan struct{})
	feedA := &mockFeed{
		name:       "feed-a",
		advisories: []Advisory{{ID: "A-1"}},
		beforeFetch: func(time.Time) {
			started <- "feed-a"
			<-release
		},
	}
	feedB := &mockFeed{
		name:       "feed-b",
		advisories: []Advisory{{ID: "B-1"}},
		beforeFetch: func(time.Time) {
			started <- "feed-b"
			<-release
		},
	}
	pipeline := NewPipeline(store, []Feed{feedA, feedB}, PipelineOptions{Logger: slog.New(slog.NewTextHandler(nilDiscardWriter{}, nil))})

	errCh := make(chan error, 1)
	go func() {
		errCh <- pipeline.PollOnce(context.Background())
	}()

	first := <-started
	second := <-started
	if first == second {
		t.Fatalf("parallel starts = %q, %q, want distinct feeds", first, second)
	}
	close(release)
	if err := <-errCh; err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if got := store.Len(); got != 2 {
		t.Fatalf("store.Len() = %d, want 2", got)
	}
}

func TestPipelineContinuesAfterFeedError(t *testing.T) {
	t.Parallel()

	store := NewMemStore()
	wantErr := errors.New("feed failed")
	var ingestedMu sync.Mutex
	ingested := make([]string, 0, 1)
	var callbackFeed string
	var callbackErr error

	pipeline := NewPipeline(store, []Feed{
		&mockFeed{name: "bad", err: wantErr},
		&mockFeed{name: "good", advisories: []Advisory{{ID: "GOOD-1"}}},
	}, PipelineOptions{
		Logger: slog.New(slog.NewTextHandler(nilDiscardWriter{}, nil)),
		OnError: func(feed string, err error) {
			callbackFeed = feed
			callbackErr = err
		},
		OnIngested: func(feed string, count int) {
			ingestedMu.Lock()
			defer ingestedMu.Unlock()
			ingested = append(ingested, fmt.Sprintf("%s:%d", feed, count))
		},
	})

	err := pipeline.PollOnce(context.Background())
	if err == nil {
		t.Fatal("PollOnce() error = nil, want non-nil")
	}
	if !errors.Is(err, wantErr) {
		t.Fatalf("PollOnce() error = %v, want wrapped %v", err, wantErr)
	}
	if store.Len() != 1 {
		t.Fatalf("store.Len() = %d, want 1", store.Len())
	}
	if callbackFeed != "bad" || !errors.Is(callbackErr, wantErr) {
		t.Fatalf("OnError callback = (%q, %v), want (%q, %v)", callbackFeed, callbackErr, "bad", wantErr)
	}
	ingestedMu.Lock()
	defer ingestedMu.Unlock()
	if !reflect.DeepEqual(ingested, []string{"good:1"}) {
		t.Fatalf("OnIngested calls = %#v, want %#v", ingested, []string{"good:1"})
	}
}

func TestPipelineStartStopLifecycle(t *testing.T) {
	t.Parallel()

	store := NewMemStore()
	feed := &mockFeed{name: "life", advisories: []Advisory{{ID: "L-1"}}}
	pipeline := NewPipeline(store, []Feed{feed}, PipelineOptions{
		PollInterval: time.Hour,
		Logger:       slog.New(slog.NewTextHandler(nilDiscardWriter{}, nil)),
	})

	pipeline.Start(context.Background())
	waitForCondition(t, time.Second, pipeline.Running)
	waitForCondition(t, time.Second, func() bool { return store.Len() == 1 })
	pipeline.Stop()
	if pipeline.Running() {
		t.Fatal("Running() = true after Stop(), want false")
	}
	callsAfterStop := feed.calls.Load()
	time.Sleep(20 * time.Millisecond)
	if got := feed.calls.Load(); got != callsAfterStop {
		t.Fatalf("fetch calls after Stop() = %d, want %d", got, callsAfterStop)
	}
}

func TestPipelinePollIntervalRunsMultiplePolls(t *testing.T) {
	t.Parallel()

	store := NewMemStore()
	var seq atomic.Int32
	feed := &mockFeed{
		name:       "tick",
		advisories: []Advisory{{ID: "seed"}},
		afterFetch: func() {},
	}
	feed.beforeFetch = func(time.Time) {
		n := seq.Add(1)
		feed.advisories = []Advisory{{ID: fmt.Sprintf("tick-%d", n)}}
	}

	pipeline := NewPipeline(store, []Feed{feed}, PipelineOptions{
		PollInterval: 15 * time.Millisecond,
		Logger:       slog.New(slog.NewTextHandler(nilDiscardWriter{}, nil)),
	})

	pipeline.Start(context.Background())
	waitForCondition(t, 2*time.Second, func() bool { return feed.calls.Load() >= 3 && store.Len() >= 3 })
	pipeline.Stop()
	if got := store.Len(); got < 3 {
		t.Fatalf("store.Len() = %d, want at least 3", got)
	}
}

func TestPipelineBatchesLargeWrites(t *testing.T) {
	t.Parallel()

	advisories := make([]Advisory, 0, 5)
	for i := range 5 {
		advisories = append(advisories, Advisory{ID: fmt.Sprintf("ADV-%d", i)})
	}
	store := NewMemStore()
	pipeline := NewPipeline(store, []Feed{&mockFeed{name: "batch", advisories: advisories}}, PipelineOptions{
		BatchSize: 2,
		Logger:    slog.New(slog.NewTextHandler(nilDiscardWriter{}, nil)),
	})

	if err := pipeline.PollOnce(context.Background()); err != nil {
		t.Fatalf("PollOnce() error = %v", err)
	}
	if got := store.batchSizes(); !reflect.DeepEqual(got, []int{2, 2, 1}) {
		t.Fatalf("batch sizes = %#v, want %#v", got, []int{2, 2, 1})
	}
}

func TestPipelineCallbacksFire(t *testing.T) {
	t.Parallel()

	store := NewMemStore()
	var mu sync.Mutex
	ingested := make([]string, 0, 1)
	errored := make([]string, 0, 1)
	wantErr := errors.New("boom")
	pipeline := NewPipeline(store, []Feed{
		&mockFeed{name: "ok", advisories: []Advisory{{ID: "OK-1"}, {ID: "OK-2"}}},
		&mockFeed{name: "err", err: wantErr},
	}, PipelineOptions{
		Logger: slog.New(slog.NewTextHandler(nilDiscardWriter{}, nil)),
		OnError: func(feed string, err error) {
			mu.Lock()
			defer mu.Unlock()
			errored = append(errored, fmt.Sprintf("%s:%t", feed, errors.Is(err, wantErr)))
		},
		OnIngested: func(feed string, count int) {
			mu.Lock()
			defer mu.Unlock()
			ingested = append(ingested, fmt.Sprintf("%s:%d", feed, count))
		},
	})

	if err := pipeline.PollOnce(context.Background()); err == nil {
		t.Fatal("PollOnce() error = nil, want non-nil")
	}
	mu.Lock()
	defer mu.Unlock()
	if !reflect.DeepEqual(ingested, []string{"ok:2"}) {
		t.Fatalf("OnIngested calls = %#v, want %#v", ingested, []string{"ok:2"})
	}
	if !reflect.DeepEqual(errored, []string{"err:true"}) {
		t.Fatalf("OnError calls = %#v, want %#v", errored, []string{"err:true"})
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("condition was not met before timeout")
}

type nilDiscardWriter struct{}

func (nilDiscardWriter) Write(p []byte) (int, error) {
	return len(p), nil
}
