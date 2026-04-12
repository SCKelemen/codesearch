package embedding

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// BatchConfig configures batch embedding behavior.
type BatchConfig struct {
	BatchSize     int
	MaxConcurrent int
	RateLimit     time.Duration
}

// BatchEmbedder wraps an Embedder with batching, concurrency control, and rate limiting.
type BatchEmbedder struct {
	embedder  Embedder
	batchSize int
	rateLimit time.Duration
	semaphore chan struct{}

	mu        sync.Mutex
	nextStart time.Time
}

// NewBatchEmbedder creates a new batch wrapper.
func NewBatchEmbedder(embedder Embedder, cfg BatchConfig) *BatchEmbedder {
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = 1
	}

	maxConcurrent := cfg.MaxConcurrent
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}

	return &BatchEmbedder{
		embedder:  embedder,
		batchSize: batchSize,
		rateLimit: cfg.RateLimit,
		semaphore: make(chan struct{}, maxConcurrent),
	}
}

// Embed embeds inputs in batches while preserving output order.
func (b *BatchEmbedder) Embed(ctx context.Context, inputs []string) ([][]float32, error) {
	if len(inputs) == 0 {
		return nil, nil
	}

	results := make([][]float32, len(inputs))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var (
		wg       sync.WaitGroup
		errOnce  sync.Once
		firstErr error
	)

	setErr := func(err error) {
		errOnce.Do(func() {
			firstErr = err
			cancel()
		})
	}

	for start := 0; start < len(inputs); start += b.batchSize {
		end := start + b.batchSize
		if end > len(inputs) {
			end = len(inputs)
		}

		batchStart := start
		batchInputs := append([]string(nil), inputs[start:end]...)

		wg.Add(1)
		go func() {
			defer wg.Done()

			if err := b.acquireSemaphore(ctx); err != nil {
				setErr(err)
				return
			}
			defer b.releaseSemaphore()

			if err := b.waitForRateLimit(ctx); err != nil {
				setErr(err)
				return
			}

			embeddings, err := b.embedder.Embed(ctx, batchInputs)
			if err != nil {
				setErr(err)
				return
			}
			if len(embeddings) != len(batchInputs) {
				setErr(fmt.Errorf("embedder returned %d embeddings for %d inputs", len(embeddings), len(batchInputs)))
				return
			}

			for i, embedding := range embeddings {
				results[batchStart+i] = embedding
			}
		}()
	}

	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	return results, nil
}

// Dimensions returns the underlying embedder dimensions.
func (b *BatchEmbedder) Dimensions() int {
	return b.embedder.Dimensions()
}

// Model returns the underlying embedder model name.
func (b *BatchEmbedder) Model() string {
	return b.embedder.Model()
}

func (b *BatchEmbedder) acquireSemaphore(ctx context.Context) error {
	select {
	case b.semaphore <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (b *BatchEmbedder) releaseSemaphore() {
	<-b.semaphore
}

func (b *BatchEmbedder) waitForRateLimit(ctx context.Context) error {
	if b.rateLimit <= 0 {
		return nil
	}

	b.mu.Lock()
	now := time.Now()
	wait := time.Duration(0)
	if b.nextStart.After(now) {
		wait = b.nextStart.Sub(now)
		b.nextStart = b.nextStart.Add(b.rateLimit)
	} else {
		b.nextStart = now.Add(b.rateLimit)
	}
	b.mu.Unlock()

	if wait <= 0 {
		return nil
	}

	timer := time.NewTimer(wait)
	defer timer.Stop()

	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
