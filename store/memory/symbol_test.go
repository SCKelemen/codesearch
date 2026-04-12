package memory

import (
	"context"
	"testing"

	"github.com/SCKelemen/codesearch/store"
)

func TestSymbolStoreCRUDSearchAndReferences(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	symbols := NewSymbolStore()

	seed := []store.Symbol{
		{ID: "sym-1", Name: "HandleRequest", Kind: store.SymbolKindFunction, RepositoryID: "repo-1", Branch: "main", DocumentID: "doc-1", Path: "api/handler.go", Language: "go", Container: "server", Metadata: map[string]string{"team": "api"}},
		{ID: "sym-2", Name: "Request", Kind: store.SymbolKindStruct, RepositoryID: "repo-1", Branch: "main", DocumentID: "doc-1", Path: "api/types.go", Language: "go", Container: "server", Metadata: map[string]string{"team": "api"}},
		{ID: "sym-3", Name: "helper", Kind: store.SymbolKindFunction, RepositoryID: "repo-2", Branch: "dev", DocumentID: "doc-2", Path: "internal/helper.py", Language: "python", Container: "helpers", Metadata: map[string]string{"team": "tooling"}},
	}
	for _, symbol := range seed {
		if err := symbols.Put(ctx, symbol); err != nil {
			t.Fatalf("Put(%s) error = %v", symbol.ID, err)
		}
	}

	if err := symbols.PutReference(ctx, store.Reference{SymbolID: "sym-1", DocumentID: "doc-1", Path: "api/handler.go", Range: store.Span{StartLine: 10, StartColumn: 2, EndLine: 10, EndColumn: 15}, Definition: true}); err != nil {
		t.Fatalf("PutReference definition error = %v", err)
	}
	if err := symbols.PutReference(ctx, store.Reference{SymbolID: "sym-1", DocumentID: "doc-3", Path: "cmd/main.go", Range: store.Span{StartLine: 20, StartColumn: 4, EndLine: 20, EndColumn: 17}, Definition: false}); err != nil {
		t.Fatalf("PutReference usage error = %v", err)
	}

	got, err := symbols.Lookup(ctx, "sym-1", store.WithRepositoryID("repo-1"), store.WithKinds(store.SymbolKindFunction))
	if err != nil {
		t.Fatalf("Lookup error = %v", err)
	}
	if got == nil || got.Name != "HandleRequest" {
		t.Fatalf("Lookup = %v, want HandleRequest", got)
	}

	page, next, err := symbols.List(ctx, store.WithRepositoryID("repo-1"), store.WithLimit(1))
	if err != nil {
		t.Fatalf("List page 1 error = %v", err)
	}
	if len(page) != 1 || page[0].ID != "sym-1" || next != "1" {
		t.Fatalf("List page 1 = (%v, %q), want sym-1 and cursor 1", page, next)
	}

	page, next, err = symbols.List(ctx, store.WithRepositoryID("repo-1"), store.WithCursor(next), store.WithLimit(2))
	if err != nil {
		t.Fatalf("List page 2 error = %v", err)
	}
	if len(page) != 1 || page[0].ID != "sym-2" || next != "" {
		t.Fatalf("List page 2 = (%v, %q), want sym-2 and empty cursor", page, next)
	}

	results, err := symbols.Search(ctx, "request", store.WithRepositoryID("repo-1"), store.WithMetadata("team", "api"))
	if err != nil {
		t.Fatalf("Search error = %v", err)
	}
	if len(results) != 2 || results[0].ID != "sym-2" {
		t.Fatalf("Search results = %v, want exact match sym-2 first", results)
	}

	refs, next, err := symbols.References(ctx, "sym-1", store.WithLimit(1))
	if err != nil {
		t.Fatalf("References page 1 error = %v", err)
	}
	if len(refs) != 1 || refs[0].Path != "api/handler.go" || next != "1" {
		t.Fatalf("References page 1 = (%v, %q), want first reference and cursor 1", refs, next)
	}

	refs, next, err = symbols.References(ctx, "sym-1", store.WithCursor(next), store.WithLimit(1))
	if err != nil {
		t.Fatalf("References page 2 error = %v", err)
	}
	if len(refs) != 1 || refs[0].Path != "cmd/main.go" || next != "" {
		t.Fatalf("References page 2 = (%v, %q), want second reference and empty cursor", refs, next)
	}

	if err := symbols.Delete(ctx, "sym-1"); err != nil {
		t.Fatalf("Delete error = %v", err)
	}
	got, err = symbols.Lookup(ctx, "sym-1")
	if err != nil {
		t.Fatalf("Lookup after delete error = %v", err)
	}
	if got != nil {
		t.Fatalf("Lookup after delete = %v, want nil", got)
	}
}
