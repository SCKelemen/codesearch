package memory

import (
	"context"
	"testing"
	"time"

	"github.com/SCKelemen/codesearch/store"
)

func TestDocumentStoreCRUDAndSearch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	docs := NewDocumentStore()
	now := time.Now().UTC()

	seed := []store.Document{
		{ID: "doc-1", RepositoryID: "repo-1", Branch: "main", Path: "src/main.go", Language: "go", Content: []byte("package main\nfunc Hello() {}"), Metadata: map[string]string{"team": "core"}, CreatedAt: now, UpdatedAt: now},
		{ID: "doc-2", RepositoryID: "repo-1", Branch: "main", Path: "README.md", Language: "markdown", Content: []byte("Hello world"), Metadata: map[string]string{"team": "docs"}, CreatedAt: now, UpdatedAt: now},
		{ID: "doc-3", RepositoryID: "repo-2", Branch: "dev", Path: "pkg/lib.py", Language: "python", Content: []byte("def hello():\n    return 'hi'"), Metadata: map[string]string{"team": "core"}, CreatedAt: now, UpdatedAt: now},
	}
	for _, doc := range seed {
		if err := docs.Put(ctx, doc); err != nil {
			t.Fatalf("Put(%s) error = %v", doc.ID, err)
		}
	}

	got, err := docs.Lookup(ctx, "doc-1", store.WithRepositoryID("repo-1"), store.WithPathPrefix("src/"))
	if err != nil {
		t.Fatalf("Lookup error = %v", err)
	}
	if got == nil || got.Path != "src/main.go" {
		t.Fatalf("Lookup returned %v, want src/main.go", got)
	}

	missing, err := docs.Lookup(ctx, "doc-1", store.WithRepositoryID("repo-2"))
	if err != nil {
		t.Fatalf("Lookup with filter error = %v", err)
	}
	if missing != nil {
		t.Fatalf("Lookup with mismatched filter = %v, want nil", missing)
	}

	page, next, err := docs.List(ctx, store.WithRepositoryID("repo-1"), store.WithLimit(1))
	if err != nil {
		t.Fatalf("List page 1 error = %v", err)
	}
	if len(page) != 1 || page[0].ID != "doc-1" || next != "1" {
		t.Fatalf("List page 1 = (%v, %q), want doc-1 and cursor 1", page, next)
	}

	page, next, err = docs.List(ctx, store.WithRepositoryID("repo-1"), store.WithCursor(next), store.WithLimit(2))
	if err != nil {
		t.Fatalf("List page 2 error = %v", err)
	}
	if len(page) != 1 || page[0].ID != "doc-2" || next != "" {
		t.Fatalf("List page 2 = (%v, %q), want doc-2 and empty cursor", page, next)
	}

	results, err := docs.Search(ctx, "hello", store.WithRepositoryID("repo-1"), store.WithMetadata("team", "core"))
	if err != nil {
		t.Fatalf("Search error = %v", err)
	}
	if len(results) != 1 || results[0].ID != "doc-1" {
		t.Fatalf("Search results = %v, want doc-1", results)
	}

	if err := docs.Delete(ctx, "doc-1"); err != nil {
		t.Fatalf("Delete error = %v", err)
	}
	got, err = docs.Lookup(ctx, "doc-1")
	if err != nil {
		t.Fatalf("Lookup after delete error = %v", err)
	}
	if got != nil {
		t.Fatalf("Lookup after delete = %v, want nil", got)
	}
}
