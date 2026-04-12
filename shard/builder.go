package shard

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/SCKelemen/codesearch/store"
	"github.com/SCKelemen/codesearch/store/memory"
	codesearchtrigram "github.com/SCKelemen/codesearch/trigram"
)

var _ store.ShardBuilder = (*Builder)(nil)

// Builder constructs immutable shard files backed by in-memory stores.
type Builder struct {
	meta      store.ShardMeta
	documents *memory.DocumentStore
	trigrams  *memory.TrigramStore
	vectors   *memory.VectorStore
	symbols   *memory.SymbolStore
}

func NewBuilder() *Builder {
	return &Builder{
		documents: memory.NewDocumentStore(),
		trigrams:  memory.NewTrigramStore(),
		vectors:   memory.NewVectorStore(),
		symbols:   memory.NewSymbolStore(),
	}
}

func (b *Builder) SetMeta(meta store.ShardMeta) {
	b.meta = meta
}

func (b *Builder) AddDocument(ctx context.Context, doc store.Document) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if doc.RepositoryID == "" {
		doc.RepositoryID = b.meta.RepositoryID
	}
	if doc.Branch == "" {
		doc.Branch = b.meta.Branch
	}
	if doc.ID == "" {
		doc.ID = makeDocumentID(doc.RepositoryID, doc.Branch, doc.Path)
	}
	if doc.Size == 0 {
		doc.Size = int64(len(doc.Content))
	}
	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = time.Now().UTC()
	}
	if doc.UpdatedAt.IsZero() {
		doc.UpdatedAt = doc.CreatedAt
	}
	return b.documents.Put(ctx, doc)
}

func (b *Builder) AddPostingList(ctx context.Context, list store.PostingList) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if existing, err := b.trigrams.Lookup(ctx, list.Trigram); err != nil {
		return err
	} else if existing != nil {
		seen := make(map[string]struct{}, len(existing.DocumentIDs)+len(list.DocumentIDs))
		merged := make([]string, 0, len(existing.DocumentIDs)+len(list.DocumentIDs))
		for _, documentID := range append(existing.DocumentIDs, list.DocumentIDs...) {
			if _, ok := seen[documentID]; ok || documentID == "" {
				continue
			}
			seen[documentID] = struct{}{}
			merged = append(merged, documentID)
		}
		list.DocumentIDs = merged
	}
	return b.trigrams.Put(ctx, list)
}

func (b *Builder) AddVector(ctx context.Context, vector store.StoredVector) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if vector.RepositoryID == "" {
		vector.RepositoryID = b.meta.RepositoryID
	}
	if vector.Branch == "" {
		vector.Branch = b.meta.Branch
	}
	if vector.DocumentID == "" && vector.Path != "" {
		vector.DocumentID = makeDocumentID(vector.RepositoryID, vector.Branch, vector.Path)
	}
	if vector.ID == "" {
		vector.ID = vector.DocumentID + ":" + vector.Model
	}
	if vector.CreatedAt.IsZero() {
		vector.CreatedAt = time.Now().UTC()
	}
	if vector.UpdatedAt.IsZero() {
		vector.UpdatedAt = vector.CreatedAt
	}
	return b.vectors.Put(ctx, vector)
}

func (b *Builder) AddSymbol(ctx context.Context, symbol store.Symbol) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if symbol.RepositoryID == "" {
		symbol.RepositoryID = b.meta.RepositoryID
	}
	if symbol.Branch == "" {
		symbol.Branch = b.meta.Branch
	}
	if symbol.DocumentID == "" && symbol.Path != "" {
		symbol.DocumentID = makeDocumentID(symbol.RepositoryID, symbol.Branch, symbol.Path)
	}
	if symbol.ID == "" {
		symbol.ID = fmt.Sprintf("%s:%s:%d:%d", symbol.Path, symbol.Name, symbol.Range.StartLine, symbol.Range.StartColumn)
	}
	return b.symbols.Put(ctx, symbol)
}

func (b *Builder) AddReference(ctx context.Context, ref store.Reference) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if ref.DocumentID == "" && ref.Path != "" {
		ref.DocumentID = makeDocumentID(b.meta.RepositoryID, b.meta.Branch, ref.Path)
	}
	return b.symbols.PutReference(ctx, ref)
}

// AddFile adds a document and indexes its unique trigrams.
func (b *Builder) AddFile(path string, content []byte, language string) (store.Document, error) {
	now := time.Now().UTC()
	sum := sha1.Sum(content)
	doc := store.Document{
		ID:           makeDocumentID(b.meta.RepositoryID, b.meta.Branch, path),
		RepositoryID: b.meta.RepositoryID,
		Branch:       b.meta.Branch,
		Path:         path,
		Language:     language,
		Content:      append([]byte(nil), content...),
		Size:         int64(len(content)),
		Checksum:     hex.EncodeToString(sum[:]),
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	if err := b.AddDocument(context.Background(), doc); err != nil {
		return store.Document{}, err
	}
	for _, tri := range codesearchtrigram.Extract(content) {
		if err := b.AddPostingList(context.Background(), store.PostingList{
			Trigram:     store.NewTrigram(tri[0], tri[1], tri[2]),
			DocumentIDs: []string{doc.ID},
		}); err != nil {
			return store.Document{}, err
		}
	}
	return doc, nil
}

// AddEmbedding adds a vector for a file and line range.
func (b *Builder) AddEmbedding(path string, startLine, endLine int, vector []float32) (store.StoredVector, error) {
	docID := makeDocumentID(b.meta.RepositoryID, b.meta.Branch, path)
	now := time.Now().UTC()
	stored := store.StoredVector{
		ID:           fmt.Sprintf("%s:%d:%d", path, startLine, endLine),
		DocumentID:   docID,
		RepositoryID: b.meta.RepositoryID,
		Branch:       b.meta.Branch,
		Path:         path,
		Model:        "unknown",
		Values:       append([]float32(nil), vector...),
		Metadata: map[string]string{
			"start_line": strconv.Itoa(startLine),
			"end_line":   strconv.Itoa(endLine),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := b.AddVector(context.Background(), stored); err != nil {
		return store.StoredVector{}, err
	}
	return stored, nil
}

func (b *Builder) Build(ctx context.Context) (store.IndexShard, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	meta := b.meta
	documents, _, err := b.documents.List(ctx)
	if err != nil {
		return nil, err
	}
	trigrams, _, err := b.trigrams.List(ctx)
	if err != nil {
		return nil, err
	}
	vectors, _, err := b.vectors.List(ctx)
	if err != nil {
		return nil, err
	}
	symbols, _, err := b.symbols.List(ctx)
	if err != nil {
		return nil, err
	}
	references := make([]store.Reference, 0)
	for _, symbol := range symbols {
		refs, _, err := b.symbols.References(ctx, symbol.ID)
		if err != nil {
			return nil, err
		}
		references = append(references, refs...)
	}
	meta.FileCount = int64(len(documents))
	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = time.Now().UTC()
	}
	raw, err := encodeShard(meta, documents, trigrams, vectors, symbols, references)
	if err != nil {
		return nil, err
	}
	meta.ByteSize = int64(len(raw))
	raw, err = encodeShard(meta, documents, trigrams, vectors, symbols, references)
	if err != nil {
		return nil, err
	}
	meta.ByteSize = int64(len(raw))

	return newLoadedShard(meta, raw, documents, trigrams, vectors, symbols, references), nil
}

// WriteTo streams the encoded shard to w.
func (b *Builder) WriteTo(w io.Writer) (int64, error) {
	shard, err := b.Build(context.Background())
	if err != nil {
		return 0, err
	}
	raw, err := shard.MarshalBinary()
	if err != nil {
		return 0, err
	}
	n, err := w.Write(raw)
	return int64(n), err
}

func makeDocumentID(repositoryID, branch, path string) string {
	parts := make([]string, 0, 3)
	if repositoryID != "" {
		parts = append(parts, repositoryID)
	}
	if branch != "" {
		parts = append(parts, branch)
	}
	parts = append(parts, path)
	return strings.Join(parts, ":")
}
