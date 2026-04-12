package structural

import "path/filepath"

// ExtractSymbols extracts structural symbols for a supported file.
func ExtractSymbols(filePath string, src []byte) ([]Symbol, error) {
	switch filepath.Ext(filePath) {
	case ".go":
		return ExtractGoSymbols(filePath, src)
	default:
		return ExtractSymbolsGeneric(filePath, src)
	}
}
