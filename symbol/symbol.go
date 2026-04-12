// Package symbol provides a searchable index of code symbols.
//
// It indexes definitions, references, and hover information from
// LSIF data, enabling type-aware code search. Symbols can be queried
// by name (exact, prefix, or fuzzy), kind, and scope.
package symbol

// Kind represents an LSP symbol kind.
// Values match the LSP specification SymbolKind enum.
type Kind int

const (
	KindUnknown       Kind = 0
	KindFile          Kind = 1
	KindModule        Kind = 2
	KindNamespace     Kind = 3
	KindPackage       Kind = 4
	KindClass         Kind = 5
	KindMethod        Kind = 6
	KindProperty      Kind = 7
	KindField         Kind = 8
	KindConstructor   Kind = 9
	KindEnum          Kind = 10
	KindInterface     Kind = 11
	KindFunction      Kind = 12
	KindVariable      Kind = 13
	KindConstant      Kind = 14
	KindString        Kind = 15
	KindNumber        Kind = 16
	KindBoolean       Kind = 17
	KindArray         Kind = 18
	KindObject        Kind = 19
	KindKey           Kind = 20
	KindNull          Kind = 21
	KindEnumMember    Kind = 22
	KindStruct        Kind = 23
	KindEvent         Kind = 24
	KindOperator      Kind = 25
	KindTypeParameter Kind = 26
)

// Location identifies a position in a document.
type Location struct {
	URI       string // document URI (opaque, any scheme)
	StartLine int    // 0-based
	StartCol  int    // 0-based, byte offset
	EndLine   int
	EndCol    int
}

// Symbol represents a code symbol with its metadata.
type Symbol struct {
	Name           string
	Kind           Kind
	Language       string
	Location       Location
	IsExported     bool
	IsDefinition   bool
	Hover          string     // hover/documentation text
	ReferenceCount int        // number of known references
	References     []Location // reference locations (may be partial)
}

// Index is a searchable collection of symbols.
type Index struct {
	symbols []Symbol
	byName  map[string][]int // name -> indices into symbols
	byURI   map[string][]int // URI -> indices into symbols
	byKind  map[Kind][]int   // kind -> indices into symbols
}

// NewIndex creates an empty symbol index.
func NewIndex() *Index {
	return &Index{
		byName: make(map[string][]int),
		byURI:  make(map[string][]int),
		byKind: make(map[Kind][]int),
	}
}

// Add adds a symbol to the index.
func (idx *Index) Add(sym Symbol) {
	i := len(idx.symbols)
	idx.symbols = append(idx.symbols, sym)
	idx.byName[sym.Name] = append(idx.byName[sym.Name], i)
	idx.byURI[sym.Location.URI] = append(idx.byURI[sym.Location.URI], i)
	idx.byKind[sym.Kind] = append(idx.byKind[sym.Kind], i)
}

// LookupName returns all symbols with the exact name.
func (idx *Index) LookupName(name string) []Symbol {
	indices := idx.byName[name]
	return idx.collect(indices)
}

// LookupURI returns all symbols in the given document.
func (idx *Index) LookupURI(uri string) []Symbol {
	indices := idx.byURI[uri]
	return idx.collect(indices)
}

// LookupKind returns all symbols of the given kind.
func (idx *Index) LookupKind(kind Kind) []Symbol {
	indices := idx.byKind[kind]
	return idx.collect(indices)
}

// Definitions returns only definition symbols matching the name.
func (idx *Index) Definitions(name string) []Symbol {
	indices := idx.byName[name]
	var results []Symbol
	for _, i := range indices {
		if idx.symbols[i].IsDefinition {
			results = append(results, idx.symbols[i])
		}
	}
	return results
}

// Count returns the total number of indexed symbols.
func (idx *Index) Count() int {
	return len(idx.symbols)
}

// All returns all indexed symbols.
func (idx *Index) All() []Symbol {
	result := make([]Symbol, len(idx.symbols))
	copy(result, idx.symbols)
	return result
}

// Search finds symbols matching the given filter.
func (idx *Index) Search(filter Filter) []Symbol {
	var candidates []int

	// Start with the most selective index
	if filter.Name != "" {
		candidates = idx.byName[filter.Name]
	} else if filter.Kind != KindUnknown {
		candidates = idx.byKind[filter.Kind]
	} else if filter.URI != "" {
		candidates = idx.byURI[filter.URI]
	} else {
		// Full scan
		candidates = make([]int, len(idx.symbols))
		for i := range candidates {
			candidates[i] = i
		}
	}

	var results []Symbol
	for _, i := range candidates {
		sym := idx.symbols[i]
		if filter.matches(sym) {
			results = append(results, sym)
		}
	}
	return results
}

func (idx *Index) collect(indices []int) []Symbol {
	if len(indices) == 0 {
		return nil
	}
	results := make([]Symbol, len(indices))
	for i, idx2 := range indices {
		results[i] = idx.symbols[idx2]
	}
	return results
}

// Filter constrains symbol search.
type Filter struct {
	Name         string
	Kind         Kind
	URI          string
	Language     string
	IsExported   *bool
	IsDefinition *bool
}

func (f Filter) matches(sym Symbol) bool {
	if f.Name != "" && sym.Name != f.Name {
		return false
	}
	if f.Kind != KindUnknown && sym.Kind != f.Kind {
		return false
	}
	if f.URI != "" && sym.Location.URI != f.URI {
		return false
	}
	if f.Language != "" && sym.Language != f.Language {
		return false
	}
	if f.IsExported != nil && sym.IsExported != *f.IsExported {
		return false
	}
	if f.IsDefinition != nil && sym.IsDefinition != *f.IsDefinition {
		return false
	}
	return true
}
