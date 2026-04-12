package trigram

import "context"

// Posting represents a trigram occurrence in a document at a specific position.
type Posting struct {
	WorkspaceID  string
	RepositoryID string
	IndexID      string
	FilePath     string
	Language     string
}

// Index defines the interface for a trigram index that supports lookups and iteration.
type Index interface {
	Add(ctx context.Context, trigram Trigram, posting Posting) error
	Query(ctx context.Context, trigrams []Trigram) ([]Posting, error)
	Remove(ctx context.Context, indexID string) error
}
