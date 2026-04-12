package index

import (
	"context"
	"errors"
	"testing"
)

type mockSource struct {
	files []FileEntry
	err   error
}

func (m *mockSource) List(_ context.Context) ([]FileEntry, error) {
	return m.files, m.err
}
func (m *mockSource) Watch(_ context.Context) (<-chan Change, error) {
	ch := make(chan Change)
	close(ch)
	return ch, nil
}

type mockSink struct {
	stored  []FileEntry
	deleted []string
	flushed bool
}

func (m *mockSink) Store(_ context.Context, entry FileEntry, _ map[string]any) error {
	m.stored = append(m.stored, entry)
	return nil
}
func (m *mockSink) Delete(_ context.Context, uri string) error {
	m.deleted = append(m.deleted, uri)
	return nil
}
func (m *mockSink) Flush(_ context.Context) error {
	m.flushed = true
	return nil
}

type mockHook struct {
	started   bool
	completed bool
	files     int
	errors    int
}

func (m *mockHook) OnStart(_ context.Context) error { m.started = true; return nil }
func (m *mockHook) OnFile(_ context.Context, _ FileEntry, err error) {
	m.files++
	if err != nil {
		m.errors++
	}
}
func (m *mockHook) OnComplete(_ context.Context, _ Stats) error { m.completed = true; return nil }

func TestPipelineRun(t *testing.T) {
	t.Parallel()
	source := &mockSource{files: []FileEntry{
		{URI: "file:///a.go", Content: []byte("package a")},
		{URI: "file:///b.go", Content: []byte("package b")},
	}}
	sink := &mockSink{}
	hook := &mockHook{}

	p := NewPipeline(source, sink, WithHook(hook))
	stats, err := p.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.FilesProcessed != 2 {
		t.Errorf("processed = %d, want 2", stats.FilesProcessed)
	}
	if !sink.flushed {
		t.Error("sink not flushed")
	}
	if !hook.started || !hook.completed {
		t.Error("hooks not called")
	}
	if hook.files != 2 {
		t.Errorf("hook files = %d, want 2", hook.files)
	}
}

func TestPipelineSourceError(t *testing.T) {
	t.Parallel()
	source := &mockSource{err: errors.New("source error")}
	sink := &mockSink{}
	p := NewPipeline(source, sink)
	_, err := p.Run(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPipelineWatch(t *testing.T) {
	t.Parallel()
	source := &mockSource{}
	sink := &mockSink{}
	p := NewPipeline(source, sink)
	err := p.Watch(context.Background())
	if err != nil {
		t.Fatalf("Watch: %v", err)
	}
}
