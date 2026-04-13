package pool_test

import (
	"context"
	"errors"
	"testing"

	"github.com/SCKelemen/codesearch/pool"
)

func TestMapReduce(t *testing.T) {
	t.Parallel()
	// Sum of squares
	result, err := pool.MapReduce(
		context.Background(),
		[]int{1, 2, 3, 4, 5},
		3,
		func(_ context.Context, x int) (int, error) { return x * x, nil },
		func(_ context.Context, acc int, val int) (int, error) { return acc + val, nil },
		0,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 55 {
		t.Errorf("result = %d, want 55", result)
	}
}

func TestMapReduceSkipsMapErrors(t *testing.T) {
	t.Parallel()
	errBad := errors.New("bad")
	result, err := pool.MapReduce(
		context.Background(),
		[]int{1, 2, 3, 4},
		2,
		func(_ context.Context, x int) (int, error) {
			if x%2 == 0 {
				return 0, errBad
			}
			return x, nil
		},
		func(_ context.Context, acc int, val int) (int, error) { return acc + val, nil },
		0,
	)
	if err != nil {
		t.Fatalf("unexpected reduce error: %v", err)
	}
	// Only 1 + 3 = 4 (evens skipped)
	if result != 4 {
		t.Errorf("result = %d, want 4", result)
	}
}

func TestMapReduceReduceError(t *testing.T) {
	t.Parallel()
	errStop := errors.New("stop")
	_, err := pool.MapReduce(
		context.Background(),
		[]int{1, 2, 3},
		2,
		func(_ context.Context, x int) (int, error) { return x, nil },
		func(_ context.Context, acc int, val int) (int, error) {
			if acc > 2 {
				return acc, errStop
			}
			return acc + val, nil
		},
		0,
	)
	if !errors.Is(err, errStop) {
		t.Errorf("expected errStop, got %v", err)
	}
}

func TestMapReduceEmpty(t *testing.T) {
	t.Parallel()
	result, err := pool.MapReduce(
		context.Background(),
		[]int{},
		4,
		func(_ context.Context, x int) (int, error) { return x, nil },
		func(_ context.Context, acc int, val int) (int, error) { return acc + val, nil },
		42,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != 42 {
		t.Errorf("result = %d, want 42 (initial)", result)
	}
}

func TestMapCollect(t *testing.T) {
	t.Parallel()
	vals, errs := pool.MapCollect(
		context.Background(),
		[]string{"a", "bb", "ccc"},
		2,
		func(_ context.Context, s string) (int, error) { return len(s), nil },
	)
	if len(errs) != 0 {
		t.Errorf("unexpected errors: %v", errs)
	}
	// Results preserve order
	if len(vals) != 3 || vals[0] != 1 || vals[1] != 2 || vals[2] != 3 {
		t.Errorf("unexpected values: %v", vals)
	}
}

func TestMapCollectWithErrors(t *testing.T) {
	t.Parallel()
	errBad := errors.New("bad")
	vals, errs := pool.MapCollect(
		context.Background(),
		[]int{1, 2, 3},
		2,
		func(_ context.Context, x int) (int, error) {
			if x == 2 {
				return 0, errBad
			}
			return x * 10, nil
		},
	)
	if len(errs) != 1 {
		t.Errorf("expected 1 error, got %d", len(errs))
	}
	if len(vals) != 2 {
		t.Errorf("expected 2 values, got %d", len(vals))
	}
}
