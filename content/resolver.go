// Package content provides a URI-based content resolution interface.
//
// Resolvers translate opaque URIs into file content, supporting any scheme:
// local files (file://), HTTP resources (https://), custom protocols
// (pierre://, git://), or anything else.
package content

import (
	"context"
	"io"
)

// Resolver resolves URIs to content. Implementations handle specific
// URI schemes (file://, https://, pierre://, etc.).
type Resolver interface {
	// Resolve returns a reader for the content at the given URI.
	Resolve(ctx context.Context, uri string) (io.ReadCloser, error)

	// Schemes returns the URI schemes this resolver handles.
	Schemes() []string
}

// Entry represents a file or directory in a content tree.
type Entry struct {
	URI   string
	Name  string
	IsDir bool
	Size  int64
}

// TreeLister lists entries in a content tree at a given URI prefix.
type TreeLister interface {
	ListTree(ctx context.Context, uri string) ([]Entry, error)
}
