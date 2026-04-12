package index

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
)

type pipelineSource struct {
	listFn  func(context.Context) ([]FileEntry, error)
	watchFn func(context.Context) (<-chan Change, error)
}

func (s pipelineSource) List(ctx context.Context) ([]FileEntry, error) {
	return s.listFn(ctx)
}

func (s pipelineSource) Watch(ctx context.Context) (<-chan Change, error) {
	return s.watchFn(ctx)
}

type pipelineTransformer struct {
	fn func(context.Context, FileEntry) (map[string]any, error)
}

func (t pipelineTransformer) Transform(ctx context.Context, entry FileEntry) (map[string]any, error) {
	return t.fn(ctx, entry)
}

type pipelineSink struct {
	mu       sync.Mutex
	storeFn  func(context.Context, FileEntry, map[string]any) error
	deleteFn func(context.Context, string) error
	flushFn  func(context.Context) error
	stored   map[string]map[string]any
	deleted  []string
	flushes  int
}

func (s *pipelineSink) Store(ctx context.Context, entry FileEntry, data map[string]any) error {
	if s.storeFn != nil {
		if err := s.storeFn(ctx, entry, data); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.stored == nil {
		s.stored = make(map[string]map[string]any)
	}
	clone := make(map[string]any, len(data))
	for key, value := range data {
		clone[key] = value
	}
	s.stored[entry.URI] = clone
	return nil
}

func (s *pipelineSink) Delete(ctx context.Context, uri string) error {
	if s.deleteFn != nil {
		if err := s.deleteFn(ctx, uri); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deleted = append(s.deleted, uri)
	return nil
}

func (s *pipelineSink) Flush(ctx context.Context) error {
	if s.flushFn != nil {
		if err := s.flushFn(ctx); err != nil {
			return err
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.flushes++
	return nil
}

type pipelineHook struct {
	mu            sync.Mutex
	startErr      error
	completeErr   error
	started       int
	completed     int
	fileEntries   []string
	fileErrors    []error
	completeStats []Stats
}

func (h *pipelineHook) OnStart(context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.started++
	return h.startErr
}

func (h *pipelineHook) OnFile(_ context.Context, entry FileEntry, err error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.fileEntries = append(h.fileEntries, entry.URI)
	h.fileErrors = append(h.fileErrors, err)
}

func (h *pipelineHook) OnComplete(_ context.Context, stats Stats) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.completed++
	h.completeStats = append(h.completeStats, stats)
	return h.completeErr
}

func TestPipelineRunWithTransformersAndErrors(t *testing.T) {
	t.Parallel()

	originalTimeNow := timeNow
	timeNow = func() int64 { return 10_500 }
	defer func() { timeNow = originalTimeNow }()

	files := []FileEntry{
		{URI: "file:///ok.go", Path: "ok.go", Content: []byte("ok")},
		{URI: "file:///transform.go", Path: "transform.go", Content: []byte("transform")},
		{URI: "file:///store.go", Path: "store.go", Content: []byte("store")},
	}
	var listed bool
	source := pipelineSource{
		listFn: func(context.Context) ([]FileEntry, error) {
			listed = true
			return files, nil
		},
		watchFn: func(context.Context) (<-chan Change, error) {
			ch := make(chan Change)
			close(ch)
			return ch, nil
		},
	}
	transformerA := pipelineTransformer{fn: func(_ context.Context, entry FileEntry) (map[string]any, error) {
		if entry.URI == "file:///transform.go" {
			return nil, errors.New("transform failed")
		}
		return map[string]any{"path": entry.Path}, nil
	}}
	transformerB := pipelineTransformer{fn: func(_ context.Context, entry FileEntry) (map[string]any, error) {
		return map[string]any{"size": len(entry.Content)}, nil
	}}
	sink := &pipelineSink{storeFn: func(_ context.Context, entry FileEntry, _ map[string]any) error {
		if entry.URI == "file:///store.go" {
			return errors.New("store failed")
		}
		return nil
	}}
	hook := &pipelineHook{}

	pipeline := NewPipeline(source, sink, WithTransformer(transformerA), WithTransformer(transformerB), WithHook(hook))
	stats, err := pipeline.Run(context.Background())
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !listed {
		t.Fatal("source List was not called")
	}
	want := Stats{FilesProcessed: 1, FilesErrored: 2, BytesProcessed: 2, Duration: 0}
	if !reflect.DeepEqual(stats, want) {
		t.Fatalf("stats = %#v, want %#v", stats, want)
	}

	sink.mu.Lock()
	stored := sink.stored["file:///ok.go"]
	flushes := sink.flushes
	sink.mu.Unlock()
	if flushes != 1 {
		t.Fatalf("flushes = %d, want 1", flushes)
	}
	if !reflect.DeepEqual(stored, map[string]any{"path": "ok.go", "size": 2}) {
		t.Fatalf("stored data = %#v, want merged transformer output", stored)
	}

	hook.mu.Lock()
	defer hook.mu.Unlock()
	if hook.started != 1 || hook.completed != 1 {
		t.Fatalf("hook start/complete = %d/%d, want 1/1", hook.started, hook.completed)
	}
	if len(hook.fileEntries) != 3 {
		t.Fatalf("hook file callbacks = %d, want 3", len(hook.fileEntries))
	}
	if hook.fileErrors[0] != nil {
		t.Fatalf("first file error = %v, want nil", hook.fileErrors[0])
	}
	if hook.fileErrors[1] == nil || hook.fileErrors[2] == nil {
		t.Fatalf("expected transform and store errors, got %#v", hook.fileErrors)
	}
	if !reflect.DeepEqual(hook.completeStats, []Stats{want}) {
		t.Fatalf("complete stats = %#v, want %#v", hook.completeStats, []Stats{want})
	}
}

func TestPipelineRunLifecycleErrors(t *testing.T) {
	t.Parallel()

	t.Run("start error", func(t *testing.T) {
		t.Parallel()

		pipeline := NewPipeline(pipelineSource{
			listFn: func(context.Context) ([]FileEntry, error) {
				return nil, nil
			},
			watchFn: func(context.Context) (<-chan Change, error) {
				ch := make(chan Change)
				close(ch)
				return ch, nil
			},
		}, &pipelineSink{}, WithHook(&pipelineHook{startErr: errors.New("start failed")}))
		_, err := pipeline.Run(context.Background())
		if err == nil || err.Error() != "start failed" {
			t.Fatalf("Run error = %v, want start failed", err)
		}
	})

	t.Run("flush error", func(t *testing.T) {
		t.Parallel()

		pipeline := NewPipeline(pipelineSource{
			listFn: func(context.Context) ([]FileEntry, error) {
				return []FileEntry{{URI: "file:///a", Content: []byte("a")}}, nil
			},
			watchFn: func(context.Context) (<-chan Change, error) {
				ch := make(chan Change)
				close(ch)
				return ch, nil
			},
		}, &pipelineSink{flushFn: func(context.Context) error { return errors.New("flush failed") }})
		_, err := pipeline.Run(context.Background())
		if err == nil || err.Error() != "flush failed" {
			t.Fatalf("Run error = %v, want flush failed", err)
		}
	})

	t.Run("complete error", func(t *testing.T) {
		t.Parallel()

		pipeline := NewPipeline(pipelineSource{
			listFn: func(context.Context) ([]FileEntry, error) {
				return []FileEntry{{URI: "file:///a", Content: []byte("abc")}}, nil
			},
			watchFn: func(context.Context) (<-chan Change, error) {
				ch := make(chan Change)
				close(ch)
				return ch, nil
			},
		}, &pipelineSink{}, WithHook(&pipelineHook{completeErr: errors.New("complete failed")}))
		_, err := pipeline.Run(context.Background())
		if err == nil || err.Error() != "complete failed" {
			t.Fatalf("Run error = %v, want complete failed", err)
		}
	})
}

func TestPipelineWatchChangeHandlingAndCancellation(t *testing.T) {
	t.Parallel()

	t.Run("processes add modify and delete while reporting errors", func(t *testing.T) {
		t.Parallel()

		changes := make(chan Change, 4)
		changes <- Change{Type: ChangeAdded, Entry: FileEntry{URI: "file:///added.go", Path: "added.go", Content: []byte("add")}}
		changes <- Change{Type: ChangeModified, Entry: FileEntry{URI: "file:///broken.go", Path: "broken.go", Content: []byte("bad")}}
		changes <- Change{Type: ChangeDeleted, Entry: FileEntry{URI: "file:///gone.go", Path: "gone.go"}}
		changes <- Change{Type: ChangeType(99), Entry: FileEntry{URI: "file:///ignored.go", Path: "ignored.go"}}
		close(changes)

		sink := &pipelineSink{}
		hook := &pipelineHook{}
		pipeline := NewPipeline(pipelineSource{
			listFn:  func(context.Context) ([]FileEntry, error) { return nil, nil },
			watchFn: func(context.Context) (<-chan Change, error) { return changes, nil },
		}, sink, WithTransformer(pipelineTransformer{fn: func(_ context.Context, entry FileEntry) (map[string]any, error) {
			if entry.URI == "file:///broken.go" {
				return nil, errors.New("watch transform failed")
			}
			return map[string]any{"path": entry.Path}, nil
		}}), WithHook(hook))

		err := pipeline.Watch(context.Background())
		if err != nil {
			t.Fatalf("Watch returned error: %v", err)
		}

		sink.mu.Lock()
		stored := sink.stored
		deleted := append([]string(nil), sink.deleted...)
		sink.mu.Unlock()
		if !reflect.DeepEqual(stored["file:///added.go"], map[string]any{"path": "added.go"}) {
			t.Fatalf("stored add = %#v, want add payload", stored["file:///added.go"])
		}
		if len(deleted) != 1 || deleted[0] != "file:///gone.go" {
			t.Fatalf("deleted = %v, want gone.go", deleted)
		}

		hook.mu.Lock()
		defer hook.mu.Unlock()
		if len(hook.fileEntries) != 1 || hook.fileEntries[0] != "file:///broken.go" {
			t.Fatalf("hook file entries = %v, want only broken.go error", hook.fileEntries)
		}
		if hook.fileErrors[0] == nil || hook.fileErrors[0].Error() != "watch transform failed" {
			t.Fatalf("hook error = %v, want watch transform failed", hook.fileErrors[0])
		}
	})

	t.Run("returns source and context errors", func(t *testing.T) {
		t.Parallel()

		watchErr := errors.New("watch failed")
		pipeline := NewPipeline(pipelineSource{
			listFn:  func(context.Context) ([]FileEntry, error) { return nil, nil },
			watchFn: func(context.Context) (<-chan Change, error) { return nil, watchErr },
		}, &pipelineSink{})
		err := pipeline.Watch(context.Background())
		if !errors.Is(err, watchErr) {
			t.Fatalf("Watch error = %v, want %v", err, watchErr)
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		changes := make(chan Change)
		pipeline = NewPipeline(pipelineSource{
			listFn:  func(context.Context) ([]FileEntry, error) { return nil, nil },
			watchFn: func(context.Context) (<-chan Change, error) { return changes, nil },
		}, &pipelineSink{})
		err = pipeline.Watch(ctx)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Watch error = %v, want context canceled", err)
		}
	})
}
