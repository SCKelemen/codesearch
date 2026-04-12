package structural

import "path"

// SymbolIndex stores symbols and supports basic filtering.
type SymbolIndex struct {
	symbols []Symbol
}

// Add inserts a symbol into the index.
func (idx *SymbolIndex) Add(symbol Symbol) {
	idx.symbols = append(idx.symbols, symbol)
}

// Search returns symbols that match the provided query.
//
// Path filtering uses path.Match glob semantics. Invalid patterns return no
// results instead of panicking.
func (idx *SymbolIndex) Search(query SymbolQuery) []Symbol {
	results := make([]Symbol, 0, len(idx.symbols))
	for _, symbol := range idx.symbols {
		if query.Name != "" && symbol.Name != query.Name {
			continue
		}
		if query.Kind != SymbolKindUnknown && symbol.Kind != query.Kind {
			continue
		}
		if query.Language != "" && symbol.Language != query.Language {
			continue
		}
		if query.Container != "" && symbol.Container != query.Container {
			continue
		}
		if query.Exported != nil && symbol.Exported != *query.Exported {
			continue
		}
		if query.Path != "" {
			matched, err := path.Match(query.Path, symbol.Path)
			if err != nil || !matched {
				continue
			}
		}
		results = append(results, symbol)
	}
	return results
}
