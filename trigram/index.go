package trigram

import "context"

type Posting struct {
	WorkspaceID  string
	RepositoryID string
	IndexID      string
	FilePath     string
	Language     string
}

type Index interface {
	Add(ctx context.Context, trigram Trigram, posting Posting) error
	Query(ctx context.Context, trigrams []Trigram) ([]Posting, error)
	Remove(ctx context.Context, indexID string) error
}
