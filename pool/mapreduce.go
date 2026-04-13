package pool

import (
	"context"
	"sync"
)

// MapReduceFunc defines the map function signature.
type MapReduceFunc[I any, M any] func(context.Context, I) (M, error)

// ReduceFunc defines the reduce function signature.
// It receives mapped values one at a time and accumulates into the output.
type ReduceFunc[M any, O any] func(context.Context, O, M) (O, error)

// MapReduce runs a map-reduce pipeline:
//   - Map: processes inputs in parallel with bounded concurrency
//   - Reduce: accumulates mapped results serially (in completion order)
//
// Returns the final accumulated value and any error from the reduce phase.
func MapReduce[I any, M any, O any](
	ctx context.Context,
	inputs []I,
	concurrency int,
	mapFn MapReduceFunc[I, M],
	reduceFn ReduceFunc[M, O],
	initial O,
) (O, error) {
	if concurrency < 1 {
		concurrency = 1
	}
	if len(inputs) == 0 {
		return initial, nil
	}

	mapped := make(chan Result[M], min(concurrency, len(inputs)))
	jobs := make(chan int, len(inputs))

	// Map phase: parallel
	var mapWg sync.WaitGroup
	for w := 0; w < min(concurrency, len(inputs)); w++ {
		mapWg.Add(1)
		go func() {
			defer mapWg.Done()
			for idx := range jobs {
				if ctx.Err() != nil {
					mapped <- Result[M]{Err: ctx.Err()}
					continue
				}
				val, err := mapFn(ctx, inputs[idx])
				mapped <- Result[M]{Value: val, Err: err}
			}
		}()
	}

	go func() {
		for i := range inputs {
			jobs <- i
		}
		close(jobs)
		mapWg.Wait()
		close(mapped)
	}()

	// Reduce phase: serial
	acc := initial
	var reduceErr error
	for r := range mapped {
		if r.Err != nil {
			continue // skip failed maps
		}
		acc, reduceErr = reduceFn(ctx, acc, r.Value)
		if reduceErr != nil {
			return acc, reduceErr
		}
	}

	return acc, nil
}

// MapCollect is a convenience wrapper that maps inputs in parallel and
// collects all successful results into a slice.
func MapCollect[I any, O any](
	ctx context.Context,
	inputs []I,
	concurrency int,
	fn func(context.Context, I) (O, error),
) ([]O, []error) {
	p := New[I, O](concurrency, fn)
	results := p.Run(ctx, inputs)

	var values []O
	var errs []error
	for _, r := range results {
		if r.Err != nil {
			errs = append(errs, r.Err)
		} else {
			values = append(values, r.Value)
		}
	}
	return values, errs
}
