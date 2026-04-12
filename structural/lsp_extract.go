package structural

import (
	"context"
	"go/ast"
	"path/filepath"

	"github.com/SCKelemen/codesearch/lsp"
)

// ExtractWithLSP uses a language server to extract symbols from a file.
// This provides compiler-quality symbol data (exact positions, types, nesting)
// instead of regex approximations.
//
// If the multiplexer has no client for this file type, falls back to
// regex-based ExtractSymbols.
func ExtractWithLSP(ctx context.Context, mux *lsp.Multiplexer, filePath string, src []byte) ([]Symbol, error) {
	if mux == nil {
		return ExtractSymbols(filePath, src)
	}
	client := mux.ClientForFile(filePath)
	if client == nil {
		return ExtractSymbols(filePath, src)
	}
	if ctx == nil {
		return ExtractSymbols(filePath, src)
	}

	absPath, err := filepath.Abs(filePath)
	if err != nil {
		absPath = filePath
	}
	uri := lsp.FileURI(absPath)

	if err := client.OpenFile(ctx, uri, string(src)); err != nil {
		return ExtractSymbols(filePath, src)
	}

	lspSymbols, err := client.DocumentSymbols(ctx, uri)
	if err != nil {
		return ExtractSymbols(filePath, src)
	}

	language := languageFromPath(filePath)
	symbols := flattenLSPSymbols(lspSymbols, "")
	for i := range symbols {
		symbols[i].Language = language
		symbols[i].Path = filePath
		symbols[i].Exported = lspSymbolExported(language, symbols[i].Name)
	}

	return symbols, nil
}

func lspKindToSymbolKind(lspKind int) SymbolKind {
	switch lspKind {
	case 2, 3:
		return SymbolKindModule
	case 4:
		return SymbolKindPackage
	case 5:
		return SymbolKindClass
	case 6, 9:
		return SymbolKindMethod
	case 7, 8, 20:
		return SymbolKindField
	case 10:
		return SymbolKindEnum
	case 11:
		return SymbolKindInterface
	case 12, 24, 25:
		return SymbolKindFunction
	case 13, 15, 16, 17, 18, 19, 21:
		return SymbolKindVariable
	case 14:
		return SymbolKindConstant
	case 22:
		return SymbolKindEnumMember
	case 23:
		return SymbolKindStruct
	case 26:
		return SymbolKindType
	default:
		return SymbolKindUnknown
	}
}

func flattenLSPSymbols(symbols []lsp.Symbol, container string) []Symbol {
	flattened := make([]Symbol, 0, len(symbols))
	for _, symbol := range symbols {
		rng := symbol.SelectionRange
		if rng == (lsp.Range{}) {
			rng = symbol.Range
		}
		flattened = append(flattened, Symbol{
			Name:      symbol.Name,
			Kind:      lspKindToSymbolKind(symbol.Kind),
			Container: container,
			Range:     lspRangeToRange(rng),
		})
		flattened = append(flattened, flattenLSPSymbols(symbol.Children, symbol.Name)...)
	}
	return flattened
}

func lspRangeToRange(r lsp.Range) Range {
	return Range{
		StartLine:   r.Start.Line + 1,
		StartColumn: r.Start.Character + 1,
		EndLine:     r.End.Line + 1,
		EndColumn:   r.End.Character + 1,
	}
}

func lspSymbolExported(language string, name string) bool {
	switch language {
	case "go":
		return ast.IsExported(name)
	case "python", "java", "rust":
		return isGenericExported(language, name)
	default:
		return false
	}
}
