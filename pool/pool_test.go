package pool_test

import (
	"context"
	"errors"
	"sort"
	"sync/atomic"
	"testing"
	"time"

	"github.com/SCKelemen/codesearch/pool"
)

func TestPoolRun(t *testing.T) {
	t.Parallel()
	p := pool.New[int, int](4, func(_ context.Context, x int) (int, error) {
		return x * 2, nil
	})
	results := p.Run(context.Background(), []int{1, 2, 3, 4, 5})
	if len(results) != 5 {
		t.Fatalf("expected 5 results, got %d", len(results))
	}
	for i, r := range results {
		if r.Err != nil {
			t.Errorf("result[%d] unexpected error: %v", i, r.Err)
		}
		want := (i + 1) * 2
		if r.Value != want {
			t.Errorf("result[%d] = %d, want %d", i, r.Value, want)
		}
	}
}

func TestPoolRunPreservesOrder(t *testing.T) {
	t.Parallel()
	p := pool.New[int, int](2, func(_ context.Context, x int) (int, error) {
		time.Sleep(time.Duration(5-x) * time.Millisecond) // reverse delay
		return x, nil
	})
	results := p.Run(context.Background(), []int{1, 2, 3, 4, 5})
	for i, r := range results {
		if r.Value != i+1 {
			t.Errorf("result[%d] = %d, want %d (order not preserved)", i, r.Value, i+1)
		}
	}
}

func TestPoolRunWithErrors(t *testing.T) {
	t.Parallel()
	errBad := errors.New("bad")
	p := pool.New[int, int](2, func(_ context.Context, x int) (int, error) {
		if x%2 == 0 {
			return 0, errBad
		}
		return x, nil
	})
	results := p.Run(context.Background(), []int{1, 2, 3, 4})
	if results[0].Err != nil || results[0].Value != 1 {
		t.Errorf("result[0] unexpected: %+v", results[0])
	}
	if results[1].Err == nil {
		t.Error("result[1] expected error")
	}
}

func TestPoolRunCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(context.Background())
	var started atomic.Int32
	p := pool.New[int, int](2, func(ctx context.Context, x int) (int, error) {
		started.Add(1)
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(time.Second):
			return x, nil
		}
	})
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	results := p.Run(ctx, []int{1, 2, 3, 4, 5, 6, 7, 8})
	var cancelled int
	for _, r := range results {
		if r.Err != nil {
			cancelled++
		}
	}
	if cancelled == 0 {
		t.Error("expected at least one cancelled result")
	}
}

func TestPoolStream(t *testing.T) {
	t.Parallel()
	p := pool.New[int, int](3, func(_ context.Context, x int) (int, error) {
		return x * 10, nil
	})
	ch := p.Stream(context.Background(), []int{1, 2, 3})
	var vals []int
	for r := range ch {
		if r.Err != nil {
			t.Errorf("unexpected error: %v", r.Err)
		}
		vals = append(vals, r.Value)
	}
	sort.Ints(vals)
	if len(vals) != 3 || vals[0] != 10 || vals[1] != 20 || vals[2] != 30 {
		t.Errorf("unexpected values: %v", vals)
	}
}

func TestPoolConcurrencyBound(t *testing.T) {
	t.Parallel()
	var active atomic.Int32
	var maxActive atomic.Int32
	p := pool.New[int, int](3, func(_ context.Context, x int) (int, error) {
		cur := active.Add(1)
		for {
			old := maxActive.Load()
			if cur <= old || maxActive.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
		active.Add(-1)
		return x, nil
	})
	p.Run(context.Background(), []int{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	if m := maxActive.Load(); m > 3 {
		t.Errorf("max active workers = %d, want <= 3", m)
	}
}

func TestPoolEmpty(t *testing.T) {
	t.Parallel()
	p := pool.New[int, int](4, func(_ context.Context, x int) (int, error) {
		return x, nil
	})
	results := p.Run(context.Background(), nil)
	if len(results) != 0 {
		t.Errorf("expected 0 results, got %d", len(results))
	}
}
