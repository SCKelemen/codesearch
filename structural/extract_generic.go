package structural

import (
	"path/filepath"
	"regexp"
	"strings"
)

var regexGenericPatterns = map[string][]genericPattern{
	"python": {
		{regexp.MustCompile(`(?m)^\s*class\s+([A-Za-z_][A-Za-z0-9_]*)\b`), SymbolKindClass},
		{regexp.MustCompile(`(?m)^\s*def\s+([A-Za-z_][A-Za-z0-9_]*)\b`), SymbolKindFunction},
		{regexp.MustCompile(`(?m)^\s*([A-Z][A-Z0-9_]*)\s*=`), SymbolKindConstant},
	},
	"java": {
		{regexp.MustCompile(`(?m)^\s*(?:public\s+)?(?:abstract\s+|final\s+)?class\s+([A-Za-z_][A-Za-z0-9_]*)\b`), SymbolKindClass},
		{regexp.MustCompile(`(?m)^\s*(?:public\s+)?interface\s+([A-Za-z_][A-Za-z0-9_]*)\b`), SymbolKindInterface},
		{regexp.MustCompile(`(?m)^\s*(?:public\s+)?enum\s+([A-Za-z_][A-Za-z0-9_]*)\b`), SymbolKindEnum},
		{regexp.MustCompile(`(?m)^\s*(?:public|protected|private)\s+(?:static\s+)?[A-Za-z_][A-Za-z0-9_<>,\[\]]*\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`), SymbolKindMethod},
		{regexp.MustCompile(`(?m)^\s*(?:public|protected|private)\s+(?:static\s+final\s+)?[A-Za-z_][A-Za-z0-9_<>,\[\]]*\s+([A-Z][A-Z0-9_]*)\s*[=;]`), SymbolKindConstant},
	},
	"rust": {
		{regexp.MustCompile(`(?m)^\s*(?:pub\s+)?mod\s+([A-Za-z_][A-Za-z0-9_]*)\b`), SymbolKindModule},
		{regexp.MustCompile(`(?m)^\s*(?:pub\s+)?struct\s+([A-Za-z_][A-Za-z0-9_]*)\b`), SymbolKindStruct},
		{regexp.MustCompile(`(?m)^\s*(?:pub\s+)?enum\s+([A-Za-z_][A-Za-z0-9_]*)\b`), SymbolKindEnum},
		{regexp.MustCompile(`(?m)^\s*(?:pub\s+)?trait\s+([A-Za-z_][A-Za-z0-9_]*)\b`), SymbolKindTrait},
		{regexp.MustCompile(`(?m)^\s*(?:pub\s+)?fn\s+([A-Za-z_][A-Za-z0-9_]*)\b`), SymbolKindFunction},
		{regexp.MustCompile(`(?m)^\s*(?:pub\s+)?const\s+([A-Z][A-Z0-9_]*)\b`), SymbolKindConstant},
	},
}

type genericPattern struct {
	re   *regexp.Regexp
	kind SymbolKind
}

// ExtractSymbolsGeneric routes to the appropriate structural extractor for the
// file's language.
func ExtractSymbolsGeneric(filePath string, src []byte) ([]Symbol, error) {
	switch languageFromPath(filePath) {
	case "typescript":
		return ExtractTypeScriptSymbols(filePath, src)
	case "javascript":
		return ExtractJavaScriptSymbols(filePath, src)
	case "sql":
		return ExtractSQLSymbols(filePath, src)
	case "rust":
		return ExtractRustSymbols(filePath, src)
	case "python", "java":
		return extractRegexGenericSymbols(filePath, src)
	default:
		return nil, nil
	}
}

func extractRegexGenericSymbols(filePath string, src []byte) ([]Symbol, error) {
	language := languageFromPath(filePath)
	patterns := regexGenericPatterns[language]
	if len(patterns) == 0 {
		return nil, nil
	}

	text := string(src)
	mapper := newOffsetMapper(text)
	var symbols []Symbol
	for _, pattern := range patterns {
		matches := pattern.re.FindAllStringSubmatchIndex(text, -1)
		for _, match := range matches {
			if len(match) < 4 {
				continue
			}
			nameStart, nameEnd := match[2], match[3]
			name := text[nameStart:nameEnd]
			rng := mapper.rangeFromOffsets(nameStart, nameEnd)
			symbols = append(symbols, Symbol{
				Name:     name,
				Kind:     pattern.kind,
				Language: language,
				Path:     filePath,
				Range:    rng,
				Exported: isGenericExported(language, name),
			})
		}
	}
	return symbols, nil
}

func languageFromPath(filePath string) string {
	switch strings.ToLower(filepath.Ext(filePath)) {
	case ".py":
		return "python"
	case ".ts", ".tsx":
		return "typescript"
	case ".js", ".jsx", ".mjs", ".cjs":
		return "javascript"
	case ".java":
		return "java"
	case ".rs":
		return "rust"
	case ".sql":
		return "sql"
	default:
		return ""
	}
}

func isGenericExported(language string, name string) bool {
	if name == "" {
		return false
	}
	first := name[0]
	switch language {
	case "python":
		return first != '_'
	case "java", "rust":
		return first >= 'A' && first <= 'Z'
	default:
		return false
	}
}
