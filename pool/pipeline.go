package pool

import (
	"context"
	"sync"
)

// Stage is a named processing stage in a pipeline.
// It reads items from an input queue, processes them, and writes results to an output queue.
type Stage[I any, O any] struct {
	// Name identifies this stage for logging and debugging.
	Name string
	// Concurrency is the number of parallel workers for this stage.
	Concurrency int
	// Process transforms an input item into an output item.
	Process func(context.Context, I) (O, error)
	// OnError is called when Process returns an error. If nil, errors are silently dropped.
	OnError func(I, error)
}

// RunStage connects an input queue to an output queue through a processing stage.
// It spawns Concurrency workers that read from in, call Process, and push results to out.
// When in is drained and all workers finish, out is closed.
func RunStage[I any, O any](ctx context.Context, stage Stage[I, O], in *Queue[I], out *Queue[O]) {
	concurrency := stage.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}

	var wg sync.WaitGroup
	for w := 0; w < concurrency; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for item := range in.C() {
				if ctx.Err() != nil {
					return
				}
				result, err := stage.Process(ctx, item)
				if err != nil {
					if stage.OnError != nil {
						stage.OnError(item, err)
					}
					continue
				}
				if err := out.Push(ctx, result); err != nil {
					return
				}
			}
		}()
	}

	go func() {
		wg.Wait()
		out.Close()
	}()
}

// Sink drains a queue, calling fn for each item serially.
// Returns when the input queue is closed and all items are processed.
func Sink[T any](ctx context.Context, in *Queue[T], fn func(context.Context, T) error) error {
	for item := range in.C() {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err := fn(ctx, item); err != nil {
			return err
		}
	}
	return nil
}

// FanOut sends each item from in to all output queues.
// When in is drained, all output queues are closed.
func FanOut[T any](ctx context.Context, in *Queue[T], outs ...*Queue[T]) {
	go func() {
		for item := range in.C() {
			for _, out := range outs {
				if ctx.Err() != nil {
					return
				}
				_ = out.Push(ctx, item)
			}
		}
		for _, out := range outs {
			out.Close()
		}
	}()
}

// Source pushes all items into a queue and closes it.
func Source[T any](ctx context.Context, items []T, out *Queue[T]) {
	go func() {
		for _, item := range items {
			if ctx.Err() != nil {
				break
			}
			if err := out.Push(ctx, item); err != nil {
				break
			}
		}
		out.Close()
	}()
}
