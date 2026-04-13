// Package pool provides generic bounded-concurrency primitives for parallel workloads.
package pool

import (
	"context"
	"sync"
)

// Result pairs an output value with an optional error.
type Result[O any] struct {
	Value O
	Err   error
}

// Pool is a bounded worker pool that processes items of type I into results of type O.
// Workers run concurrently up to the configured concurrency limit.
type Pool[I any, O any] struct {
	concurrency int
	fn          func(context.Context, I) (O, error)
}

// New creates a Pool that runs fn with the given concurrency.
func New[I any, O any](concurrency int, fn func(context.Context, I) (O, error)) *Pool[I, O] {
	if concurrency < 1 {
		concurrency = 1
	}
	return &Pool[I, O]{concurrency: concurrency, fn: fn}
}

// Run processes all items and returns results preserving input order.
// It blocks until all items are processed or the context is cancelled.
func (p *Pool[I, O]) Run(ctx context.Context, items []I) []Result[O] {
	results := make([]Result[O], len(items))
	jobs := make(chan int, len(items))

	var wg sync.WaitGroup
	for w := 0; w < min(p.concurrency, len(items)); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				if ctx.Err() != nil {
					results[idx] = Result[O]{Err: ctx.Err()}
					continue
				}
				val, err := p.fn(ctx, items[idx])
				results[idx] = Result[O]{Value: val, Err: err}
			}
		}()
	}

	for i := range items {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	return results
}

// Stream processes items and sends results to the returned channel as they complete.
// The channel is closed when all items are processed.
func (p *Pool[I, O]) Stream(ctx context.Context, items []I) <-chan Result[O] {
	out := make(chan Result[O], p.concurrency)
	jobs := make(chan int, len(items))

	var wg sync.WaitGroup
	for w := 0; w < min(p.concurrency, len(items)); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for idx := range jobs {
				if ctx.Err() != nil {
					out <- Result[O]{Err: ctx.Err()}
					continue
				}
				val, err := p.fn(ctx, items[idx])
				out <- Result[O]{Value: val, Err: err}
			}
		}()
	}

	go func() {
		for i := range items {
			jobs <- i
		}
		close(jobs)
		wg.Wait()
		close(out)
	}()

	return out
}
