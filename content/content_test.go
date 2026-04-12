package content

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileResolver_Resolve(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewFileResolver()
	rc, err := r.Resolve(context.Background(), "file://"+path)
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "hello world" {
		t.Errorf("got %q, want %q", string(data), "hello world")
	}
}

func TestFileResolver_Resolve_BareFilePath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "bare.txt")
	if err := os.WriteFile(path, []byte("bare content"), 0644); err != nil {
		t.Fatal(err)
	}

	r := NewFileResolver()
	rc, err := r.Resolve(context.Background(), path)
	if err != nil {
		t.Fatalf("Resolve bare path: %v", err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(data) != "bare content" {
		t.Errorf("got %q, want %q", string(data), "bare content")
	}
}

func TestFileResolver_Resolve_NotFound(t *testing.T) {
	t.Parallel()

	r := NewFileResolver()
	_, err := r.Resolve(context.Background(), "file:///nonexistent/path/file.txt")
	if err == nil {
		t.Fatal("expected error for nonexistent file")
	}
}

func TestFileResolver_Schemes(t *testing.T) {
	t.Parallel()

	r := NewFileResolver()
	schemes := r.Schemes()
	if len(schemes) != 1 || schemes[0] != "file" {
		t.Errorf("Schemes: got %v, want [file]", schemes)
	}
}

type mockResolver struct {
	schemes []string
	content string
}

func (m *mockResolver) Resolve(_ context.Context, _ string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(m.content)), nil
}

func (m *mockResolver) Schemes() []string {
	return m.schemes
}

func TestMultiResolver_Dispatch(t *testing.T) {
	t.Parallel()

	httpResolver := &mockResolver{schemes: []string{"http", "https"}, content: "http content"}
	customResolver := &mockResolver{schemes: []string{"custom"}, content: "custom content"}

	multi := NewMultiResolver(httpResolver, customResolver)

	rc, err := multi.Resolve(context.Background(), "https://example.com/file")
	if err != nil {
		t.Fatalf("Resolve https: %v", err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if string(data) != "http content" {
		t.Errorf("https: got %q, want %q", string(data), "http content")
	}

	rc, err = multi.Resolve(context.Background(), "custom://host/path")
	if err != nil {
		t.Fatalf("Resolve custom: %v", err)
	}
	data, _ = io.ReadAll(rc)
	rc.Close()
	if string(data) != "custom content" {
		t.Errorf("custom: got %q, want %q", string(data), "custom content")
	}
}

func TestMultiResolver_UnknownScheme(t *testing.T) {
	t.Parallel()

	multi := NewMultiResolver(&mockResolver{schemes: []string{"http"}, content: ""})
	_, err := multi.Resolve(context.Background(), "ftp://host/file")
	if err == nil {
		t.Fatal("expected error for unknown scheme")
	}
	if !strings.Contains(err.Error(), "ftp") {
		t.Errorf("error should mention scheme: %v", err)
	}
}

func TestMultiResolver_Schemes(t *testing.T) {
	t.Parallel()

	multi := NewMultiResolver(
		&mockResolver{schemes: []string{"http", "https"}},
		&mockResolver{schemes: []string{"custom"}},
	)
	schemes := multi.Schemes()
	if len(schemes) != 3 {
		t.Errorf("Schemes: got %d, want 3", len(schemes))
	}
}

func TestMultiResolver_NoSchemeDefaultsToFile(t *testing.T) {
	t.Parallel()

	fileResolver := &mockResolver{schemes: []string{"file"}, content: "file content"}
	multi := NewMultiResolver(fileResolver)

	rc, err := multi.Resolve(context.Background(), "/some/local/path")
	if err != nil {
		t.Fatalf("Resolve bare path: %v", err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	if string(data) != "file content" {
		t.Errorf("got %q, want %q", string(data), "file content")
	}
}

func TestExtractScheme(t *testing.T) {
	t.Parallel()

	tests := []struct {
		uri  string
		want string
	}{
		{"https://example.com", "https"},
		{"http://host/path", "http"},
		{"file:///path", "file"},
		{"custom+scheme://host", "custom+scheme"},
		{"/bare/path", "file"},
		{"relative/path", "file"},
	}

	for _, tt := range tests {
		got := extractScheme(tt.uri)
		if got != tt.want {
			t.Errorf("extractScheme(%q) = %q, want %q", tt.uri, got, tt.want)
		}
	}
}
