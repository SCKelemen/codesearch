package memory

import (
	"context"
	"testing"

	"github.com/SCKelemen/codesearch/store"
)

func TestTrigramStoreCRUDAndSearch(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	grams := NewTrigramStore()

	aaa, _ := store.ParseTrigram("aaa")
	aab, _ := store.ParseTrigram("aab")
	aac, _ := store.ParseTrigram("aac")

	seed := []store.PostingList{
		{Trigram: aaa, DocumentIDs: []string{"doc-2", "doc-1", "doc-1"}},
		{Trigram: aab, DocumentIDs: []string{"doc-1", "doc-3"}},
		{Trigram: aac, DocumentIDs: []string{"doc-3"}},
	}
	for _, list := range seed {
		if err := grams.Put(ctx, list); err != nil {
			t.Fatalf("Put(%v) error = %v", list.Trigram, err)
		}
	}

	list, err := grams.Lookup(ctx, aaa)
	if err != nil {
		t.Fatalf("Lookup error = %v", err)
	}
	if list == nil || len(list.DocumentIDs) != 2 || list.DocumentIDs[0] != "doc-1" || list.DocumentIDs[1] != "doc-2" {
		t.Fatalf("Lookup = %v, want sorted unique doc IDs", list)
	}

	page, next, err := grams.List(ctx, store.WithLimit(2))
	if err != nil {
		t.Fatalf("List page 1 error = %v", err)
	}
	if len(page) != 2 || next != "2" {
		t.Fatalf("List page 1 = (%v, %q), want 2 results and cursor 2", page, next)
	}

	page, next, err = grams.List(ctx, store.WithCursor(next), store.WithLimit(2))
	if err != nil {
		t.Fatalf("List page 2 error = %v", err)
	}
	if len(page) != 1 || next != "" {
		t.Fatalf("List page 2 = (%v, %q), want 1 result and empty cursor", page, next)
	}

	results, err := grams.Search(ctx, []store.Trigram{aaa, aab, aac}, store.WithLimit(3))
	if err != nil {
		t.Fatalf("Search error = %v", err)
	}
	if len(results) != 3 || results[0].MatchedTrigrams != 2 || results[1].MatchedTrigrams != 2 {
		t.Fatalf("Search results = %v, want the top two candidates to have 2 matches", results)
	}

	filtered, err := grams.Search(ctx, []store.Trigram{aaa, aab}, store.WithDocumentID("doc-2"))
	if err != nil {
		t.Fatalf("Search with filter error = %v", err)
	}
	if len(filtered) != 1 || filtered[0].DocumentID != "doc-2" || filtered[0].MatchedTrigrams != 1 {
		t.Fatalf("Search with filter = %v, want doc-2 with 1 match", filtered)
	}

	if err := grams.Delete(ctx, aab); err != nil {
		t.Fatalf("Delete error = %v", err)
	}
	list, err = grams.Lookup(ctx, aab)
	if err != nil {
		t.Fatalf("Lookup after delete error = %v", err)
	}
	if list != nil {
		t.Fatalf("Lookup after delete = %v, want nil", list)
	}
}
