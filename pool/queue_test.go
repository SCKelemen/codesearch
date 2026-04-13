package pool_test

import (
	"context"
	"testing"
	"time"

	"github.com/SCKelemen/codesearch/pool"
)

func TestQueuePushAndConsume(t *testing.T) {
	t.Parallel()
	q := pool.NewQueue[int](5)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := q.Push(ctx, i); err != nil {
			t.Fatalf("push %d: %v", i, err)
		}
	}
	q.Close()

	var got []int
	for v := range q.C() {
		got = append(got, v)
	}
	if len(got) != 5 {
		t.Fatalf("expected 5 items, got %d", len(got))
	}
}

func TestQueueBackpressure(t *testing.T) {
	t.Parallel()
	q := pool.NewQueue[int](1)
	ctx := context.Background()

	if err := q.Push(ctx, 1); err != nil {
		t.Fatalf("first push: %v", err)
	}

	// Second push should block since capacity is 1
	done := make(chan struct{})
	go func() {
		_ = q.Push(ctx, 2)
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("push should have blocked")
	case <-time.After(50 * time.Millisecond):
		// expected - push is blocking
	}

	// Drain one to unblock
	<-q.C()
	select {
	case <-done:
		// unblocked
	case <-time.After(time.Second):
		t.Fatal("push should have unblocked after drain")
	}
	q.Close()
}

func TestQueueCancelledPush(t *testing.T) {
	t.Parallel()
	q := pool.NewQueue[int](0) // unbuffered = every push blocks
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := q.Push(ctx, 42)
	if err == nil {
		t.Fatal("expected error from cancelled context")
	}
	q.Close()
}

func TestQueueCloseIdempotent(t *testing.T) {
	t.Parallel()
	q := pool.NewQueue[int](1)
	q.Close()
	q.Close() // should not panic
}

func TestQueueLen(t *testing.T) {
	t.Parallel()
	q := pool.NewQueue[int](10)
	ctx := context.Background()
	_ = q.Push(ctx, 1)
	_ = q.Push(ctx, 2)
	_ = q.Push(ctx, 3)
	if q.Len() != 3 {
		t.Errorf("Len() = %d, want 3", q.Len())
	}
	q.Close()
}

func TestQueuePushAfterClose(t *testing.T) {
	t.Parallel()
	q := pool.NewQueue[int](5)
	q.Close()
	err := q.Push(context.Background(), 1)
	if err == nil {
		t.Fatal("expected error pushing to closed queue")
	}
}
