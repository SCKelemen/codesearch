package structural

import (
	"regexp"
	"sort"
	"strings"
)

var sqlStatementPatterns = []struct {
	re           *regexp.Regexp
	kind         SymbolKind
	extractField bool
}{
	{regexp.MustCompile(`(?is)\bcreate\s+(?:or\s+replace\s+)?(?:temp(?:orary)?\s+)?table\s+(?:if\s+not\s+exists\s+)?`), SymbolKindStruct, true},
	{regexp.MustCompile(`(?is)\balter\s+table\s+(?:if\s+exists\s+)?`), SymbolKindStruct, false},
	{regexp.MustCompile(`(?is)\bcreate\s+(?:or\s+replace\s+)?(?:materialized\s+)?view\s+(?:if\s+not\s+exists\s+)?`), SymbolKindType, false},
	{regexp.MustCompile(`(?is)\bcreate\s+(?:unique\s+)?index\s+(?:if\s+not\s+exists\s+)?`), SymbolKindType, false},
	{regexp.MustCompile(`(?is)\bcreate\s+(?:or\s+replace\s+)?function\s+`), SymbolKindFunction, false},
	{regexp.MustCompile(`(?is)\bcreate\s+(?:or\s+replace\s+)?procedure\s+`), SymbolKindFunction, false},
	{regexp.MustCompile(`(?is)\bcreate\s+(?:or\s+replace\s+)?trigger\s+`), SymbolKindFunction, false},
	{regexp.MustCompile(`(?is)\bcreate\s+type\s+(?:if\s+not\s+exists\s+)?`), SymbolKindType, false},
}

var sqlColumnConstraintKeywords = map[string]struct{}{
	"constraint": {},
	"primary":    {},
	"foreign":    {},
	"unique":     {},
	"check":      {},
	"key":        {},
	"index":      {},
	"fulltext":   {},
	"spatial":    {},
}

// ExtractSQLSymbols extracts structural symbols from SQL DDL source.
func ExtractSQLSymbols(filePath string, src []byte) ([]Symbol, error) {
	text := string(src)
	cleaned := stripSQLComments(text)
	mapper := newOffsetMapper(text)

	type symbolWithOffset struct {
		offset int
		symbol Symbol
	}

	var collected []symbolWithOffset
	for _, pattern := range sqlStatementPatterns {
		matches := pattern.re.FindAllStringIndex(cleaned, -1)
		for _, match := range matches {
			schema, name, nameStart, nameEnd := parseSQLQualifiedName(cleaned, match[1])
			if name == "" {
				continue
			}

			collected = append(collected, symbolWithOffset{
				offset: nameStart,
				symbol: Symbol{
					Name:      name,
					Kind:      pattern.kind,
					Language:  "sql",
					Path:      filePath,
					Container: schema,
					Range:     mapper.rangeFromOffsets(nameStart, nameEnd),
				},
			})

			if !pattern.extractField {
				continue
			}

			bodyStart := skipSQLSpace(cleaned, nameEnd)
			if bodyStart >= len(cleaned) || cleaned[bodyStart] != '(' {
				continue
			}

			bodyEnd := findMatchingSQLParen(cleaned, bodyStart)
			if bodyEnd <= bodyStart {
				continue
			}

			for _, field := range extractSQLColumnSymbols(filePath, cleaned, name, bodyStart+1, bodyEnd, mapper) {
				collected = append(collected, symbolWithOffset{
					offset: field.Range.StartLine*1_000_000 + field.Range.StartColumn,
					symbol: field,
				})
			}
		}
	}

	sort.SliceStable(collected, func(i int, j int) bool {
		if collected[i].symbol.Range.StartLine != collected[j].symbol.Range.StartLine {
			return collected[i].symbol.Range.StartLine < collected[j].symbol.Range.StartLine
		}
		if collected[i].symbol.Range.StartColumn != collected[j].symbol.Range.StartColumn {
			return collected[i].symbol.Range.StartColumn < collected[j].symbol.Range.StartColumn
		}
		return collected[i].offset < collected[j].offset
	})

	symbols := make([]Symbol, 0, len(collected))
	for _, item := range collected {
		symbols = append(symbols, item.symbol)
	}
	return symbols, nil
}

func stripSQLComments(text string) string {
	buf := []byte(text)
	for i := 0; i < len(buf); i++ {
		switch buf[i] {
		case '\'':
			for i++; i < len(buf); i++ {
				if buf[i] == '\'' {
					if i+1 < len(buf) && buf[i+1] == '\'' {
						i++
						continue
					}
					break
				}
			}
		case '"':
			for i++; i < len(buf); i++ {
				if buf[i] == '"' {
					if i+1 < len(buf) && buf[i+1] == '"' {
						i++
						continue
					}
					break
				}
			}
		case '`':
			for i++; i < len(buf); i++ {
				if buf[i] == '`' {
					break
				}
			}
		case '[':
			for i++; i < len(buf); i++ {
				if buf[i] == ']' {
					break
				}
			}
		case '#':
			for j := i; j < len(buf) && buf[j] != '\n'; j++ {
				buf[j] = ' '
			}
		case '-':
			if i+1 < len(buf) && buf[i+1] == '-' {
				for j := i; j < len(buf) && buf[j] != '\n'; j++ {
					buf[j] = ' '
				}
			}
		case '/':
			if i+1 < len(buf) && buf[i+1] == '*' {
				buf[i] = ' '
				buf[i+1] = ' '
				i += 2
				for ; i < len(buf); i++ {
					if i+1 < len(buf) && buf[i] == '*' && buf[i+1] == '/' {
						buf[i] = ' '
						buf[i+1] = ' '
						i++
						break
					}
					if buf[i] != '\n' {
						buf[i] = ' '
					}
				}
			}
		}
	}
	return string(buf)
}

func parseSQLQualifiedName(text string, offset int) (schema string, name string, start int, end int) {
	parts := make([]string, 0, 3)
	partStarts := make([]int, 0, 3)
	partEnds := make([]int, 0, 3)
	pos := skipSQLSpace(text, offset)

	for {
		ident, identStart, identEnd := parseSQLIdentifier(text, pos)
		if ident == "" {
			break
		}
		parts = append(parts, ident)
		partStarts = append(partStarts, identStart)
		partEnds = append(partEnds, identEnd)
		pos = skipSQLSpace(text, identEnd)
		if pos >= len(text) || text[pos] != '.' {
			break
		}
		pos = skipSQLSpace(text, pos+1)
	}

	if len(parts) == 0 {
		return "", "", 0, 0
	}
	if len(parts) == 1 {
		return "", parts[0], partStarts[0], partEnds[0]
	}
	return strings.Join(parts[:len(parts)-1], "."), parts[len(parts)-1], partStarts[len(parts)-1], partEnds[len(parts)-1]
}

func parseSQLIdentifier(text string, offset int) (name string, start int, end int) {
	pos := skipSQLSpace(text, offset)
	if pos >= len(text) {
		return "", 0, 0
	}

	switch text[pos] {
	case '"', '`':
		quote := text[pos]
		for i := pos + 1; i < len(text); i++ {
			if text[i] != quote {
				continue
			}
			if i+1 < len(text) && text[i+1] == quote {
				i++
				continue
			}
			return strings.ReplaceAll(text[pos+1:i], string([]byte{quote, quote}), string(quote)), pos, i + 1
		}
	case '[':
		for i := pos + 1; i < len(text); i++ {
			if text[i] == ']' {
				return text[pos+1 : i], pos, i + 1
			}
		}
	default:
		if !isSQLIdentifierStart(text[pos]) {
			return "", 0, 0
		}
		i := pos + 1
		for i < len(text) && isSQLIdentifierPart(text[i]) {
			i++
		}
		return text[pos:i], pos, i
	}

	return "", 0, 0
}

func isSQLIdentifierStart(ch byte) bool {
	return ch == '_' || (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z')
}

func isSQLIdentifierPart(ch byte) bool {
	return isSQLIdentifierStart(ch) || (ch >= '0' && ch <= '9') || ch == '$'
}

func skipSQLSpace(text string, offset int) int {
	for offset < len(text) {
		switch text[offset] {
		case ' ', '\t', '\n', '\r', '\f':
			offset++
		default:
			return offset
		}
	}
	return offset
}

func findMatchingSQLParen(text string, open int) int {
	depth := 0
	for i := open; i < len(text); i++ {
		switch text[i] {
		case '\'':
			for i++; i < len(text); i++ {
				if text[i] == '\'' {
					if i+1 < len(text) && text[i+1] == '\'' {
						i++
						continue
					}
					break
				}
			}
		case '"':
			for i++; i < len(text); i++ {
				if text[i] == '"' {
					if i+1 < len(text) && text[i+1] == '"' {
						i++
						continue
					}
					break
				}
			}
		case '`':
			for i++; i < len(text); i++ {
				if text[i] == '`' {
					break
				}
			}
		case '[':
			for i++; i < len(text); i++ {
				if text[i] == ']' {
					break
				}
			}
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func extractSQLColumnSymbols(filePath string, text string, tableName string, start int, end int, mapper offsetMapper) []Symbol {
	var symbols []Symbol
	segmentStart := start
	depth := 0

	flush := func(segmentEnd int) {
		segmentSymbol := sqlColumnSymbol(filePath, text, tableName, segmentStart, segmentEnd, mapper)
		if segmentSymbol.Name != "" {
			symbols = append(symbols, segmentSymbol)
		}
	}

	for i := start; i < end; i++ {
		switch text[i] {
		case '\'':
			for i++; i < end; i++ {
				if text[i] == '\'' {
					if i+1 < end && text[i+1] == '\'' {
						i++
						continue
					}
					break
				}
			}
		case '"':
			for i++; i < end; i++ {
				if text[i] == '"' {
					if i+1 < end && text[i+1] == '"' {
						i++
						continue
					}
					break
				}
			}
		case '`':
			for i++; i < end; i++ {
				if text[i] == '`' {
					break
				}
			}
		case '[':
			for i++; i < end; i++ {
				if text[i] == ']' {
					break
				}
			}
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				flush(i)
				segmentStart = i + 1
			}
		}
	}
	flush(end)

	return symbols
}

func sqlColumnSymbol(filePath string, text string, tableName string, start int, end int, mapper offsetMapper) Symbol {
	pos := skipSQLSpace(text, start)
	if pos >= end {
		return Symbol{}
	}

	name, nameStart, nameEnd := parseSQLIdentifier(text, pos)
	if name == "" || nameEnd > end {
		return Symbol{}
	}
	if _, isConstraint := sqlColumnConstraintKeywords[strings.ToLower(name)]; isConstraint {
		return Symbol{}
	}

	remainderStart := skipSQLSpace(text, nameEnd)
	if remainderStart >= end {
		return Symbol{}
	}

	typeName, _, _ := parseSQLIdentifier(text, remainderStart)
	if typeName == "" {
		return Symbol{}
	}

	return Symbol{
		Name:      name,
		Kind:      SymbolKindField,
		Language:  "sql",
		Path:      filePath,
		Container: tableName,
		Range:     mapper.rangeFromOffsets(nameStart, nameEnd),
	}
}
