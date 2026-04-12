package file

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/SCKelemen/codesearch/store"
)

func TestRoundTripPersistence(t *testing.T) {
	ctx := context.Background()
	baseDir := t.TempDir()

	docDir := filepath.Join(baseDir, "documents")
	docStore, err := NewDocumentStore(docDir)
	if err != nil {
		t.Fatalf("new document store: %v", err)
	}
	document := store.Document{
		ID:           "doc-1",
		RepositoryID: "repo-1",
		Branch:       "main",
		Path:         "main.go",
		Language:     "go",
		Content:      []byte("package main\nfunc main() {}\n"),
		Metadata:     map[string]string{"team": "search"},
		CreatedAt:    time.Unix(100, 0).UTC(),
		UpdatedAt:    time.Unix(200, 0).UTC(),
	}
	if err := docStore.Put(ctx, document); err != nil {
		t.Fatalf("put document: %v", err)
	}
	if err := docStore.Close(); err != nil {
		t.Fatalf("close document store: %v", err)
	}

	docStore, err = NewDocumentStore(docDir)
	if err != nil {
		t.Fatalf("reopen document store: %v", err)
	}
	loadedDocument, err := docStore.Lookup(ctx, document.ID)
	if err != nil {
		t.Fatalf("lookup document: %v", err)
	}
	if loadedDocument == nil || loadedDocument.Path != document.Path || string(loadedDocument.Content) != string(document.Content) {
		t.Fatalf("unexpected document after reopen: %#v", loadedDocument)
	}
	if err := docStore.Close(); err != nil {
		t.Fatalf("close reopened document store: %v", err)
	}

	trigramDir := filepath.Join(baseDir, "trigrams")
	trigramStore, err := NewTrigramStore(trigramDir)
	if err != nil {
		t.Fatalf("new trigram store: %v", err)
	}
	tri := store.NewTrigram('g', 'o', ' ')
	postingList := store.PostingList{Trigram: tri, DocumentIDs: []string{"doc-1", "doc-2"}}
	if err := trigramStore.Put(ctx, postingList); err != nil {
		t.Fatalf("put posting list: %v", err)
	}
	if err := trigramStore.Close(); err != nil {
		t.Fatalf("close trigram store: %v", err)
	}

	trigramStore, err = NewTrigramStore(trigramDir)
	if err != nil {
		t.Fatalf("reopen trigram store: %v", err)
	}
	loadedPostingList, err := trigramStore.Lookup(ctx, tri)
	if err != nil {
		t.Fatalf("lookup posting list: %v", err)
	}
	if loadedPostingList == nil || len(loadedPostingList.DocumentIDs) != 2 {
		t.Fatalf("unexpected posting list after reopen: %#v", loadedPostingList)
	}
	if err := trigramStore.Close(); err != nil {
		t.Fatalf("close reopened trigram store: %v", err)
	}

	vectorDir := filepath.Join(baseDir, "vectors")
	vectorStore, err := NewVectorStore(vectorDir)
	if err != nil {
		t.Fatalf("new vector store: %v", err)
	}
	vector := store.StoredVector{
		ID:           "vec-1",
		DocumentID:   "doc-1",
		RepositoryID: "repo-1",
		Branch:       "main",
		Path:         "main.go",
		Model:        "test-embedding",
		Values:       []float32{1, 0, 0},
		Metadata:     map[string]string{"kind": "chunk"},
		CreatedAt:    time.Unix(300, 0).UTC(),
		UpdatedAt:    time.Unix(400, 0).UTC(),
	}
	if err := vectorStore.Put(ctx, vector); err != nil {
		t.Fatalf("put vector: %v", err)
	}
	if err := vectorStore.Close(); err != nil {
		t.Fatalf("close vector store: %v", err)
	}

	vectorStore, err = NewVectorStore(vectorDir)
	if err != nil {
		t.Fatalf("reopen vector store: %v", err)
	}
	loadedVector, err := vectorStore.Lookup(ctx, vector.ID)
	if err != nil {
		t.Fatalf("lookup vector: %v", err)
	}
	if loadedVector == nil || len(loadedVector.Values) != len(vector.Values) || loadedVector.Model != vector.Model {
		t.Fatalf("unexpected vector after reopen: %#v", loadedVector)
	}
	results, err := vectorStore.Search(ctx, []float32{1, 0, 0}, 1, store.DistanceMetricCosine)
	if err != nil {
		t.Fatalf("search vectors: %v", err)
	}
	if len(results) != 1 || results[0].Vector.ID != vector.ID {
		t.Fatalf("unexpected vector search results after reopen: %#v", results)
	}
	if err := vectorStore.Close(); err != nil {
		t.Fatalf("close reopened vector store: %v", err)
	}

	symbolDir := filepath.Join(baseDir, "symbols")
	symbolStore, err := NewSymbolStore(symbolDir)
	if err != nil {
		t.Fatalf("new symbol store: %v", err)
	}
	symbol := store.Symbol{
		ID:           "sym-1",
		Name:         "main",
		Kind:         store.SymbolKindFunction,
		RepositoryID: "repo-1",
		Branch:       "main",
		DocumentID:   "doc-1",
		Path:         "main.go",
		Language:     "go",
		Signature:    "func main()",
		Definition:   true,
		Metadata:     map[string]string{"visibility": "public"},
	}
	reference := store.Reference{
		SymbolID:   symbol.ID,
		DocumentID: "doc-1",
		Path:       "main.go",
		Range: store.Span{
			StartLine:   1,
			StartColumn: 1,
			EndLine:     1,
			EndColumn:   5,
		},
		Definition: true,
	}
	if err := symbolStore.Put(ctx, symbol); err != nil {
		t.Fatalf("put symbol: %v", err)
	}
	if err := symbolStore.PutReference(ctx, reference); err != nil {
		t.Fatalf("put symbol reference: %v", err)
	}
	if err := symbolStore.Close(); err != nil {
		t.Fatalf("close symbol store: %v", err)
	}

	symbolStore, err = NewSymbolStore(symbolDir)
	if err != nil {
		t.Fatalf("reopen symbol store: %v", err)
	}
	loadedSymbol, err := symbolStore.Lookup(ctx, symbol.ID)
	if err != nil {
		t.Fatalf("lookup symbol: %v", err)
	}
	if loadedSymbol == nil || loadedSymbol.Name != symbol.Name {
		t.Fatalf("unexpected symbol after reopen: %#v", loadedSymbol)
	}
	references, next, err := symbolStore.References(ctx, symbol.ID)
	if err != nil {
		t.Fatalf("list references: %v", err)
	}
	if next != "" || len(references) != 1 || references[0].Path != reference.Path {
		t.Fatalf("unexpected symbol references after reopen: %#v, next=%q", references, next)
	}
	if err := symbolStore.Close(); err != nil {
		t.Fatalf("close reopened symbol store: %v", err)
	}
}
