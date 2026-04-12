package content

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// FileResolver resolves file:// URIs to local filesystem content.
type FileResolver struct{}

// NewFileResolver creates a resolver for local files.
func NewFileResolver() *FileResolver {
	return &FileResolver{}
}

// Resolve opens a local file from a file:// URI.
func (r *FileResolver) Resolve(_ context.Context, uri string) (io.ReadCloser, error) {
	path, err := uriToPath(uri)
	if err != nil {
		return nil, err
	}
	return os.Open(path)
}

// Schemes returns ["file"].
func (r *FileResolver) Schemes() []string {
	return []string{"file"}
}

func uriToPath(uri string) (string, error) {
	if strings.HasPrefix(uri, "file://") {
		u, err := url.Parse(uri)
		if err != nil {
			return "", fmt.Errorf("parse file URI: %w", err)
		}
		return filepath.FromSlash(u.Path), nil
	}
	return uri, nil
}
