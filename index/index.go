// Package index provides the indexing pipeline framework.
//
// It defines interfaces for building code search indexes from arbitrary
// sources. Consumers implement Source and Sink to create custom pipelines.
// The framework handles orchestration, hooks, and change detection.
//
// Architecture:
//
//	Source → Transformer → Sink
//	  ↑                     ↓
//	Hook (onChange)     Hook (onIndex)
package index

import (
	"context"
	"fmt"
)

// FileEntry represents a file to be indexed.
type FileEntry struct {
	URI      string
	Path     string // relative path within the repository
	Language string
	Content  []byte
	Metadata map[string]string
}

// ChangeType indicates what happened to a file.
type ChangeType int

const (
	ChangeAdded ChangeType = iota
	ChangeModified
	ChangeDeleted
)

// Change represents a file change event.
type Change struct {
	Type  ChangeType
	Entry FileEntry
}

// Source provides files to index. Implementations could read from
// local disk, git repositories, HTTP APIs, or any other source.
type Source interface {
	// List returns all files available from this source.
	List(ctx context.Context) ([]FileEntry, error)

	// Watch sends change events as files are added, modified, or deleted.
	// Implementations should close the channel when done or context is cancelled.
	Watch(ctx context.Context) (<-chan Change, error)
}

// Transformer processes files before indexing. Use for extracting
// trigrams, building suffix arrays, parsing LSIF, etc.
type Transformer interface {
	// Transform processes a file entry and returns derived data.
	// The returned map contains named outputs (e.g., "trigrams", "symbols").
	Transform(ctx context.Context, entry FileEntry) (map[string]any, error)
}

// Sink receives processed data for storage. Implementations could
// write to Spanner, local files, in-memory indexes, etc.
type Sink interface {
	// Store persists the indexed data for a file.
	Store(ctx context.Context, entry FileEntry, data map[string]any) error

	// Delete removes indexed data for a file.
	Delete(ctx context.Context, uri string) error

	// Flush ensures all pending writes are committed.
	Flush(ctx context.Context) error
}

// Hook is called at pipeline lifecycle events.
type Hook interface {
	// OnStart is called when the pipeline begins.
	OnStart(ctx context.Context) error

	// OnFile is called after each file is processed.
	OnFile(ctx context.Context, entry FileEntry, err error)

	// OnComplete is called when the pipeline finishes.
	OnComplete(ctx context.Context, stats Stats) error
}

// Stats holds pipeline execution statistics.
type Stats struct {
	FilesProcessed int
	FilesErrored   int
	BytesProcessed int64
	Duration       int64 // milliseconds
}

// Pipeline orchestrates the indexing process.
type Pipeline struct {
	source       Source
	transformers []Transformer
	sink         Sink
	hooks        []Hook
}

// NewPipeline creates an indexing pipeline.
func NewPipeline(source Source, sink Sink, opts ...PipelineOption) *Pipeline {
	p := &Pipeline{
		source: source,
		sink:   sink,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// PipelineOption configures a pipeline.
type PipelineOption func(*Pipeline)

// WithTransformer adds a transformer to the pipeline.
func WithTransformer(t Transformer) PipelineOption {
	return func(p *Pipeline) {
		p.transformers = append(p.transformers, t)
	}
}

// WithHook adds a lifecycle hook to the pipeline.
func WithHook(h Hook) PipelineOption {
	return func(p *Pipeline) {
		p.hooks = append(p.hooks, h)
	}
}

// Run executes the pipeline: lists files from source, transforms them,
// and stores results in the sink.
func (p *Pipeline) Run(ctx context.Context) (Stats, error) {
	var stats Stats
	start := timeNow()

	for _, h := range p.hooks {
		if err := h.OnStart(ctx); err != nil {
			return stats, err
		}
	}

	files, err := p.source.List(ctx)
	if err != nil {
		return stats, err
	}

	for _, entry := range files {
		if err := ctx.Err(); err != nil {
			return stats, err
		}

		data, processErr := p.processFile(ctx, entry)
		if processErr != nil {
			stats.FilesErrored++
			for _, h := range p.hooks {
				h.OnFile(ctx, entry, processErr)
			}
			continue
		}

		if storeErr := p.sink.Store(ctx, entry, data); storeErr != nil {
			stats.FilesErrored++
			for _, h := range p.hooks {
				h.OnFile(ctx, entry, storeErr)
			}
			continue
		}

		stats.FilesProcessed++
		stats.BytesProcessed += int64(len(entry.Content))
		for _, h := range p.hooks {
			h.OnFile(ctx, entry, nil)
		}
	}

	if err := p.sink.Flush(ctx); err != nil {
		return stats, err
	}

	stats.Duration = timeNow() - start
	for _, h := range p.hooks {
		if err := h.OnComplete(ctx, stats); err != nil {
			return stats, err
		}
	}

	return stats, nil
}

// Watch starts the pipeline in watch mode, processing changes as they arrive.
func (p *Pipeline) Watch(ctx context.Context) error {
	changes, err := p.source.Watch(ctx)
	if err != nil {
		return fmt.Errorf("watch source: %w", err)
	}

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case change, ok := <-changes:
			if !ok {
				return nil
			}
			if err := p.processChange(ctx, change); err != nil {
				// Log but don't stop
				for _, h := range p.hooks {
					h.OnFile(ctx, change.Entry, err)
				}
			}
		}
	}
}

func (p *Pipeline) processFile(ctx context.Context, entry FileEntry) (map[string]any, error) {
	merged := make(map[string]any)
	for _, t := range p.transformers {
		data, err := t.Transform(ctx, entry)
		if err != nil {
			return nil, err
		}
		for k, v := range data {
			merged[k] = v
		}
	}
	return merged, nil
}

func (p *Pipeline) processChange(ctx context.Context, change Change) error {
	switch change.Type {
	case ChangeDeleted:
		return p.sink.Delete(ctx, change.Entry.URI)
	case ChangeAdded, ChangeModified:
		data, err := p.processFile(ctx, change.Entry)
		if err != nil {
			return fmt.Errorf("process file %q: %w", change.Entry.URI, err)
		}
		return p.sink.Store(ctx, change.Entry, data)
	}
	return nil
}

// timeNow returns current time in milliseconds. Extracted for testing.
var timeNow = func() int64 {
	return 0 // Will be set properly in init or by caller
}
