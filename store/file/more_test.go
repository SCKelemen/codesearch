package file

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/SCKelemen/codesearch/store"
)

func TestDocumentStoreRoundTripSearchDeleteAndConcurrentPuts(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	dir := t.TempDir()
	docStore, err := NewDocumentStore(dir, WithFlushStrategy(FlushManual))
	if err != nil {
		t.Fatalf("NewDocumentStore returned error: %v", err)
	}

	now := time.Unix(1_700_000_000, 0).UTC()
	docs := []store.Document{
		{ID: "doc-1", RepositoryID: "repo", Branch: "main", Path: "src/main.go", Language: "go", Content: []byte("package main\n"), Metadata: map[string]string{"team": "search"}, CreatedAt: now, UpdatedAt: now},
		{ID: "doc-2", RepositoryID: "repo", Branch: "main", Path: "src/helper.go", Language: "go", Content: []byte("helper needle\n"), Metadata: map[string]string{"team": "search"}, CreatedAt: now, UpdatedAt: now},
		{ID: "doc-3", RepositoryID: "repo", Branch: "dev", Path: "scripts/task.py", Language: "python", Content: []byte("print('needle')\n"), Metadata: map[string]string{"team": "ml"}, CreatedAt: now, UpdatedAt: now},
	}
	var wg sync.WaitGroup
	for _, doc := range docs {
		doc := doc
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := docStore.Put(ctx, doc); err != nil {
				t.Errorf("Put(%s) returned error: %v", doc.ID, err)
			}
		}()
	}
	wg.Wait()

	if _, err := os.Stat(filepath.Join(dir, documentFilename)); !os.IsNotExist(err) {
		t.Fatalf("documents file stat error = %v, want not exists before Flush", err)
	}
	page, next, err := docStore.List(ctx, store.WithRepositoryID("repo"), store.WithLimit(2))
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}
	if len(page) != 2 || next != "2" {
		t.Fatalf("List = (%v, %q), want 2 docs and next=2", page, next)
	}
	matches, err := docStore.Search(ctx, "needle", store.WithRepositoryID("repo"))
	if err != nil {
		t.Fatalf("Search returned error: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("Search len = %d, want 2", len(matches))
	}
	if err := docStore.Flush(); err != nil {
		t.Fatalf("Flush returned error: %v", err)
	}
	if err := docStore.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}

	docStore, err = NewDocumentStore(dir, WithFlushStrategy(FlushManual))
	if err != nil {
		t.Fatalf("Reopen NewDocumentStore returned error: %v", err)
	}
	loaded, err := docStore.Lookup(ctx, "doc-2", store.WithRepositoryID("repo"), store.WithDocumentID("doc-2"))
	if err != nil {
		t.Fatalf("Lookup returned error: %v", err)
	}
	if loaded == nil || loaded.Path != "src/helper.go" {
		t.Fatalf("Lookup = %#v, want src/helper.go", loaded)
	}
	if err := docStore.Delete(ctx, "doc-3"); err != nil {
		t.Fatalf("Delete returned error: %v", err)
	}
	if err := docStore.Flush(); err != nil {
		t.Fatalf("Flush after delete returned error: %v", err)
	}
	if err := docStore.Close(); err != nil {
		t.Fatalf("Close after delete returned error: %v", err)
	}

	docStore, err = NewDocumentStore(dir)
	if err != nil {
		t.Fatalf("Final reopen NewDocumentStore returned error: %v", err)
	}
	defer docStore.Close()
	missing, err := docStore.Lookup(ctx, "doc-3")
	if err != nil {
		t.Fatalf("Lookup after delete returned error: %v", err)
	}
	if missing != nil {
		t.Fatalf("Lookup after delete = %#v, want nil", missing)
	}
}

func TestTrigramVectorAndSymbolStoreRoundTrips(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	now := time.Unix(1_700_000_100, 0).UTC()

	t.Run("trigram store", func(t *testing.T) {
		t.Parallel()

		dir := filepath.Join(t.TempDir(), "trigrams")
		grams, err := NewTrigramStore(dir, WithFlushStrategy(FlushManual))
		if err != nil {
			t.Fatalf("NewTrigramStore returned error: %v", err)
		}
		tri := store.NewTrigram('n', 'e', 'e')
		if err := grams.Put(ctx, store.PostingList{Trigram: tri, DocumentIDs: []string{"doc-2", "doc-1", "doc-2"}}); err != nil {
			t.Fatalf("Put returned error: %v", err)
		}
		lists, _, err := grams.List(ctx)
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if len(lists) != 1 || !reflect.DeepEqual(lists[0].DocumentIDs, []string{"doc-1", "doc-2"}) {
			t.Fatalf("List = %v, want sorted unique doc IDs", lists)
		}
		results, err := grams.Search(ctx, []store.Trigram{tri}, store.WithDocumentID("doc-2"))
		if err != nil {
			t.Fatalf("Search returned error: %v", err)
		}
		if len(results) != 1 || results[0].DocumentID != "doc-2" {
			t.Fatalf("Search = %v, want doc-2", results)
		}
		if err := grams.Flush(); err != nil {
			t.Fatalf("Flush returned error: %v", err)
		}
		if err := grams.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
		grams, err = NewTrigramStore(dir)
		if err != nil {
			t.Fatalf("Reopen NewTrigramStore returned error: %v", err)
		}
		defer grams.Close()
		list, err := grams.Lookup(ctx, tri)
		if err != nil {
			t.Fatalf("Lookup returned error: %v", err)
		}
		if list == nil || !reflect.DeepEqual(list.DocumentIDs, []string{"doc-1", "doc-2"}) {
			t.Fatalf("Lookup = %#v, want persisted posting list", list)
		}
		if err := grams.Delete(ctx, tri); err != nil {
			t.Fatalf("Delete returned error: %v", err)
		}
	})

	t.Run("vector store with batched flush", func(t *testing.T) {
		t.Parallel()

		dir := filepath.Join(t.TempDir(), "vectors")
		vectors, err := NewVectorStore(dir, WithFlushStrategy(FlushBatched), WithBatchWindow(20*time.Millisecond))
		if err != nil {
			t.Fatalf("NewVectorStore returned error: %v", err)
		}
		entries := []store.StoredVector{
			{ID: "vec-1", DocumentID: "doc-1", RepositoryID: "repo", Branch: "main", Path: "src/main.go", Model: "test", Values: []float32{1, 0}, Metadata: map[string]string{"team": "search"}, CreatedAt: now, UpdatedAt: now},
			{ID: "vec-2", DocumentID: "doc-2", RepositoryID: "repo", Branch: "main", Path: "src/helper.go", Model: "test", Values: []float32{0.9, 0.1}, Metadata: map[string]string{"team": "search"}, CreatedAt: now, UpdatedAt: now},
		}
		for _, entry := range entries {
			if err := vectors.Put(ctx, entry); err != nil {
				t.Fatalf("Put(%s) returned error: %v", entry.ID, err)
			}
		}
		waitForFile(t, filepath.Join(dir, vectorFilename), 200*time.Millisecond)
		page, next, err := vectors.List(ctx, store.WithRepositoryID("repo"), store.WithLimit(1))
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if len(page) != 1 || next != "1" {
			t.Fatalf("List = (%v, %q), want one result and next cursor", page, next)
		}
		results, err := vectors.Search(ctx, []float32{1, 0}, 2, store.DistanceMetricCosine, store.WithMetadata("team", "search"))
		if err != nil {
			t.Fatalf("Search returned error: %v", err)
		}
		if len(results) != 2 || results[0].Vector.ID != "vec-1" {
			t.Fatalf("Search = %v, want vec-1 first", results)
		}
		if err := vectors.Flush(); err != nil {
			t.Fatalf("Flush returned error: %v", err)
		}
		if err := vectors.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
		vectors, err = NewVectorStore(dir)
		if err != nil {
			t.Fatalf("Reopen NewVectorStore returned error: %v", err)
		}
		defer vectors.Close()
		lookup, err := vectors.Lookup(ctx, "vec-2")
		if err != nil {
			t.Fatalf("Lookup returned error: %v", err)
		}
		if lookup == nil || lookup.Path != "src/helper.go" {
			t.Fatalf("Lookup = %#v, want helper vector", lookup)
		}
		if err := vectors.Delete(ctx, "vec-2"); err != nil {
			t.Fatalf("Delete returned error: %v", err)
		}
	})

	t.Run("symbol store", func(t *testing.T) {
		t.Parallel()

		dir := filepath.Join(t.TempDir(), "symbols")
		symbols, err := NewSymbolStore(dir, WithFlushStrategy(FlushImmediate))
		if err != nil {
			t.Fatalf("NewSymbolStore returned error: %v", err)
		}
		symbol := store.Symbol{ID: "sym-1", Name: "Main", Kind: store.SymbolKindFunction, RepositoryID: "repo", Branch: "main", DocumentID: "doc-1", Path: "src/main.go", Language: "go", Signature: "func Main()", Metadata: map[string]string{"team": "search"}}
		ref := store.Reference{SymbolID: symbol.ID, DocumentID: "doc-2", Path: "src/helper.go", Range: store.Span{StartLine: 1, StartColumn: 1, EndLine: 1, EndColumn: 4}}
		if err := symbols.Put(ctx, symbol); err != nil {
			t.Fatalf("Put returned error: %v", err)
		}
		if err := symbols.PutReference(ctx, ref); err != nil {
			t.Fatalf("PutReference returned error: %v", err)
		}
		listed, _, err := symbols.List(ctx, store.WithKinds(store.SymbolKindFunction))
		if err != nil {
			t.Fatalf("List returned error: %v", err)
		}
		if len(listed) != 1 || listed[0].ID != symbol.ID {
			t.Fatalf("List = %v, want sym-1", listed)
		}
		matches, err := symbols.Search(ctx, "main", store.WithMetadata("team", "search"))
		if err != nil {
			t.Fatalf("Search returned error: %v", err)
		}
		if len(matches) != 1 || matches[0].Name != "Main" {
			t.Fatalf("Search = %v, want Main", matches)
		}
		refs, next, err := symbols.References(ctx, symbol.ID, store.WithPathPrefix("src/"), store.WithLimit(1))
		if err != nil {
			t.Fatalf("References returned error: %v", err)
		}
		if len(refs) != 1 || next != "" || refs[0].Path != ref.Path {
			t.Fatalf("References = (%v, %q), want helper reference", refs, next)
		}
		if err := symbols.Flush(); err != nil {
			t.Fatalf("Flush returned error: %v", err)
		}
		if err := symbols.Close(); err != nil {
			t.Fatalf("Close returned error: %v", err)
		}
		symbols, err = NewSymbolStore(dir)
		if err != nil {
			t.Fatalf("Reopen NewSymbolStore returned error: %v", err)
		}
		defer symbols.Close()
		lookup, err := symbols.Lookup(ctx, symbol.ID)
		if err != nil {
			t.Fatalf("Lookup returned error: %v", err)
		}
		if lookup == nil || lookup.Name != symbol.Name {
			t.Fatalf("Lookup = %#v, want persisted symbol", lookup)
		}
		if err := symbols.Delete(ctx, symbol.ID); err != nil {
			t.Fatalf("Delete returned error: %v", err)
		}
	})
}

func TestPersistenceHelpersAndErrorPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	var target map[string]int
	found, err := loadGobFile(filepath.Join(dir, "missing.gob"), &target)
	if err != nil || found {
		t.Fatalf("loadGobFile missing = (%v, %v), want (false, nil)", found, err)
	}

	emptyPath := filepath.Join(dir, "empty.gob")
	if err := os.WriteFile(emptyPath, nil, 0o600); err != nil {
		t.Fatalf("WriteFile empty returned error: %v", err)
	}
	found, err = loadGobFile(emptyPath, &target)
	if err != nil || !found {
		t.Fatalf("loadGobFile empty = (%v, %v), want (true, nil)", found, err)
	}

	if err := saveGobFile(filepath.Join(dir, "state.gob"), map[string]int{"value": 7}); err != nil {
		t.Fatalf("saveGobFile returned error: %v", err)
	}
	found, err = loadGobFile(filepath.Join(dir, "state.gob"), &target)
	if err != nil || !found || target["value"] != 7 {
		t.Fatalf("loadGobFile saved = (%v, %v, %v), want true, nil, value=7", found, err, target)
	}

	corruptPath := filepath.Join(dir, "corrupt.gob")
	if err := os.WriteFile(corruptPath, []byte("not-a-gob"), 0o600); err != nil {
		t.Fatalf("WriteFile corrupt returned error: %v", err)
	}
	if _, err := loadGobFile(corruptPath, &target); err == nil || !strings.Contains(err.Error(), "decode") {
		t.Fatalf("loadGobFile corrupt error = %v, want decode error", err)
	}

	cfg := defaultConfig()
	WithBatchWindow(0)(&cfg)
	if cfg.batchWindow != defaultBatchWindow {
		t.Fatalf("WithBatchWindow(0) batchWindow = %v, want %v", cfg.batchWindow, defaultBatchWindow)
	}
	WithFlushStrategy(FlushManual)(&cfg)
	if cfg.flushStrategy != FlushManual {
		t.Fatalf("WithFlushStrategy set flushStrategy = %v, want FlushManual", cfg.flushStrategy)
	}

	p := &persistence{config: config{flushStrategy: FlushStrategy(99)}}
	if err := p.markDirty(); err == nil || !strings.Contains(err.Error(), "unknown flush strategy") {
		t.Fatalf("markDirty error = %v, want unknown flush strategy", err)
	}
	if err := (&fileLock{}).Close(); err != nil {
		t.Fatalf("fileLock.Close returned error: %v", err)
	}
	if err := atomicWriteFile(filepath.Join(dir, "nested", "value.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatalf("atomicWriteFile returned error: %v", err)
	}
	content, err := os.ReadFile(filepath.Join(dir, "nested", "value.txt"))
	if err != nil || string(content) != "hello" {
		t.Fatalf("ReadFile nested value = (%q, %v), want hello", content, err)
	}
}

func waitForFile(t *testing.T, path string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		if _, err := os.Stat(path); err == nil {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", path)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
