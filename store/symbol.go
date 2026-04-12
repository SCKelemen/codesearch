package store

import "context"

// SymbolKind identifies the type of a symbol.
type SymbolKind int

const (
	SymbolKindUnknown       SymbolKind = 0
	SymbolKindFile          SymbolKind = 1
	SymbolKindModule        SymbolKind = 2
	SymbolKindNamespace     SymbolKind = 3
	SymbolKindPackage       SymbolKind = 4
	SymbolKindClass         SymbolKind = 5
	SymbolKindMethod        SymbolKind = 6
	SymbolKindProperty      SymbolKind = 7
	SymbolKindField         SymbolKind = 8
	SymbolKindConstructor   SymbolKind = 9
	SymbolKindEnum          SymbolKind = 10
	SymbolKindInterface     SymbolKind = 11
	SymbolKindFunction      SymbolKind = 12
	SymbolKindVariable      SymbolKind = 13
	SymbolKindConstant      SymbolKind = 14
	SymbolKindString        SymbolKind = 15
	SymbolKindNumber        SymbolKind = 16
	SymbolKindBoolean       SymbolKind = 17
	SymbolKindArray         SymbolKind = 18
	SymbolKindObject        SymbolKind = 19
	SymbolKindKey           SymbolKind = 20
	SymbolKindNull          SymbolKind = 21
	SymbolKindEnumMember    SymbolKind = 22
	SymbolKindStruct        SymbolKind = 23
	SymbolKindEvent         SymbolKind = 24
	SymbolKindOperator      SymbolKind = 25
	SymbolKindTypeParameter SymbolKind = 26
)

// Span identifies a region inside a document.
type Span struct {
	StartLine   int
	StartColumn int
	EndLine     int
	EndColumn   int
}

// Symbol describes a code symbol and its definition site.
type Symbol struct {
	ID           string
	Name         string
	Kind         SymbolKind
	RepositoryID string
	Branch       string
	DocumentID   string
	Path         string
	Language     string
	Container    string
	Signature    string
	Range        Span
	Exported     bool
	Definition   bool
	Metadata     map[string]string
}

// Reference describes a symbol reference inside a document.
type Reference struct {
	SymbolID   string
	DocumentID string
	Path       string
	Range      Span
	Definition bool
}

// SymbolStore stores symbols and reference relationships.
type SymbolStore interface {
	// Put creates or replaces a symbol.
	Put(ctx context.Context, symbol Symbol) error

	// PutReference creates or replaces a symbol reference.
	PutReference(ctx context.Context, ref Reference) error

	// Lookup returns a symbol by ID.
	Lookup(ctx context.Context, id string, opts ...LookupOption) (*Symbol, error)

	// List returns symbols and the next cursor.
	// An empty next cursor means there are no more results.
	List(ctx context.Context, opts ...ListOption) ([]Symbol, string, error)

	// Search returns symbols that match the supplied query.
	Search(ctx context.Context, query string, opts ...SearchOption) ([]Symbol, error)

	// References returns references for a symbol and the next cursor.
	// An empty next cursor means there are no more results.
	References(ctx context.Context, symbolID string, opts ...ListOption) ([]Reference, string, error)

	// Delete removes a symbol by ID.
	Delete(ctx context.Context, id string) error
}
