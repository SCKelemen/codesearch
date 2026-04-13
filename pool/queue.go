package pool

import (
	"context"
	"errors"
	"sync"
)

// ErrQueueClosed reports that an operation was attempted on a closed queue.
var ErrQueueClosed = errors.New("queue closed")

// Queue is a bounded async queue with backpressure.
// Producers block on Push when the queue is full.
// Consumers receive items via the channel returned by C.
type Queue[T any] struct {
	ch   chan T
	done chan struct{}
	once sync.Once
}

// NewQueue creates a queue with the given capacity.
// A capacity of 0 creates an unbuffered (synchronous) queue.
func NewQueue[T any](capacity int) *Queue[T] {
	if capacity < 0 {
		capacity = 0
	}
	return &Queue[T]{
		ch:   make(chan T, capacity),
		done: make(chan struct{}),
	}
}

// Push sends an item to the queue. It blocks if the queue is full.
// Returns an error if the context is cancelled while waiting or if the queue
// has been closed.
func (q *Queue[T]) Push(ctx context.Context, item T) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = ErrQueueClosed
		}
	}()

	select {
	case <-q.done:
		return ErrQueueClosed
	case <-ctx.Done():
		return ctx.Err()
	case q.ch <- item:
		return nil
	}
}

// C returns the receive-only channel for consuming items.
func (q *Queue[T]) C() <-chan T {
	return q.ch
}

// Close signals that no more items will be pushed.
// Consumers should drain C() after Close is called.
func (q *Queue[T]) Close() {
	q.once.Do(func() {
		close(q.done)
		close(q.ch)
	})
}

// Len returns the number of items currently buffered.
func (q *Queue[T]) Len() int {
	return len(q.ch)
}
