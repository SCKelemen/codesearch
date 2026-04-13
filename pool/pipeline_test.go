package pool

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"
)

func TestRunStage(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	in := NewQueue[int](10)
	out := NewQueue[string](10)

	stage := Stage[int, string]{
		Name:        "double",
		Concurrency: 2,
		Process: func(_ context.Context, n int) (string, error) {
			return fmt.Sprintf("%d", n*2), nil
		},
	}

	RunStage(ctx, stage, in, out)

	// Push items
	for i := 1; i <= 5; i++ {
		if err := in.Push(ctx, i); err != nil {
			t.Fatal(err)
		}
	}
	in.Close()

	// Collect results
	var results []string
	for s := range out.C() {
		results = append(results, s)
	}

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
	sort.Strings(results)
	want := []string{"10", "2", "4", "6", "8"}
	for i, r := range results {
		if r != want[i] {
			t.Errorf("results[%d] = %q, want %q", i, r, want[i])
		}
	}
}

func TestRunStageWithErrors(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	in := NewQueue[int](10)
	out := NewQueue[int](10)

	var mu sync.Mutex
	var errors []int

	stage := Stage[int, int]{
		Name:        "filter-even",
		Concurrency: 1,
		Process: func(_ context.Context, n int) (int, error) {
			if n%2 == 0 {
				return 0, fmt.Errorf("even number: %d", n)
			}
			return n, nil
		},
		OnError: func(item int, _ error) {
			mu.Lock()
			errors = append(errors, item)
			mu.Unlock()
		},
	}

	RunStage(ctx, stage, in, out)

	for i := 1; i <= 6; i++ {
		if err := in.Push(ctx, i); err != nil {
			t.Fatal(err)
		}
	}
	in.Close()

	var results []int
	for n := range out.C() {
		results = append(results, n)
	}

	sort.Ints(results)
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d: %v", len(results), results)
	}

	mu.Lock()
	sort.Ints(errors)
	mu.Unlock()
	if len(errors) != 3 {
		t.Fatalf("expected 3 errors, got %d: %v", len(errors), errors)
	}
}

func TestSink(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	in := NewQueue[int](10)
	go func() {
		for i := 1; i <= 5; i++ {
			_ = in.Push(ctx, i)
		}
		in.Close()
	}()

	var sum int
	err := Sink(ctx, in, func(_ context.Context, n int) error {
		sum += n
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum != 15 {
		t.Fatalf("sum = %d, want 15", sum)
	}
}

func TestFanOut(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	in := NewQueue[int](10)
	out1 := NewQueue[int](10)
	out2 := NewQueue[int](10)

	FanOut(ctx, in, out1, out2)

	go func() {
		for i := 1; i <= 3; i++ {
			_ = in.Push(ctx, i)
		}
		in.Close()
	}()

	var results1, results2 []int
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for n := range out1.C() {
			results1 = append(results1, n)
		}
	}()
	go func() {
		defer wg.Done()
		for n := range out2.C() {
			results2 = append(results2, n)
		}
	}()
	wg.Wait()

	sort.Ints(results1)
	sort.Ints(results2)
	if len(results1) != 3 || len(results2) != 3 {
		t.Fatalf("expected 3 items each, got %d and %d", len(results1), len(results2))
	}
}

func TestSource(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	out := NewQueue[string](10)
	Source(ctx, []string{"a", "b", "c"}, out)

	var results []string
	for s := range out.C() {
		results = append(results, s)
	}
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}
}

func TestPipelineChain(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	// Source -> Stage1 (double) -> Stage2 (to string) -> Sink
	q0 := NewQueue[int](10)
	q1 := NewQueue[int](10)
	q2 := NewQueue[string](10)

	RunStage(ctx, Stage[int, int]{
		Name:        "double",
		Concurrency: 2,
		Process:     func(_ context.Context, n int) (int, error) { return n * 2, nil },
	}, q0, q1)

	RunStage(ctx, Stage[int, string]{
		Name:        "stringify",
		Concurrency: 1,
		Process:     func(_ context.Context, n int) (string, error) { return fmt.Sprintf("val=%d", n), nil },
	}, q1, q2)

	Source(ctx, []int{1, 2, 3, 4, 5}, q0)

	var results []string
	if err := Sink(ctx, q2, func(_ context.Context, s string) error {
		results = append(results, s)
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
}

func TestStageContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())

	in := NewQueue[int](100)
	out := NewQueue[int](100)

	stage := Stage[int, int]{
		Name:        "slow",
		Concurrency: 2,
		Process: func(ctx context.Context, n int) (int, error) {
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(100 * time.Millisecond):
				return n, nil
			}
		},
	}

	RunStage(ctx, stage, in, out)

	for i := 0; i < 50; i++ {
		_ = in.Push(ctx, i)
	}
	in.Close()

	// Cancel after a short time
	time.AfterFunc(50*time.Millisecond, cancel)

	var count int
	for range out.C() {
		count++
	}
	// Should get fewer than 50 results due to cancellation
	if count >= 50 {
		t.Errorf("expected fewer than 50 results due to cancellation, got %d", count)
	}
}
