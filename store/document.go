package store

import (
	"context"
	"time"
)

// Document describes a single indexed document.
type Document struct {
	ID           string
	RepositoryID string
	Branch       string
	Path         string
	Language     string
	Content      []byte
	Size         int64
	Checksum     string
	Metadata     map[string]string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// DocumentStore stores and retrieves indexed documents.
type DocumentStore interface {
	// Put creates or replaces a document.
	Put(ctx context.Context, doc Document) error

	// Lookup returns a document by ID.
	Lookup(ctx context.Context, id string, opts ...LookupOption) (*Document, error)

	// List returns documents that match the supplied options and the next cursor.
	// An empty next cursor means there are no more results.
	List(ctx context.Context, opts ...ListOption) ([]Document, string, error)

	// Search returns documents that match the supplied query.
	Search(ctx context.Context, query string, opts ...SearchOption) ([]Document, error)

	// Delete removes a document by ID.
	Delete(ctx context.Context, id string) error
}
