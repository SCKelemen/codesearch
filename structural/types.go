// Package structural provides language-aware structural symbol extraction and search.
package structural

// SymbolKind describes the kind of symbol represented in the structural index.
type SymbolKind int

const (
	SymbolKindUnknown SymbolKind = iota
	SymbolKindPackage
	SymbolKindModule
	SymbolKindClass
	SymbolKindInterface
	SymbolKindStruct
	SymbolKindEnum
	SymbolKindTrait
	SymbolKindType
	SymbolKindFunction
	SymbolKindMethod
	SymbolKindField
	SymbolKindVariable
	SymbolKindConstant
)

// Range identifies a source span using 1-based line and column coordinates.
type Range struct {
	StartLine   int
	StartColumn int
	EndLine     int
	EndColumn   int
}

// Symbol represents a language-level declaration discovered in a source file.
type Symbol struct {
	Name      string
	Kind      SymbolKind
	Language  string
	Path      string
	Container string
	Range     Range
	Exported  bool
}

// SymbolQuery filters structural symbol search results.
type SymbolQuery struct {
	Name      string
	Kind      SymbolKind
	Language  string
	Path      string
	Container string
	Exported  *bool
}
