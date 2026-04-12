package structural

import (
	"regexp"
	"sort"
	"strings"
)

var (
	tsClassPattern                  = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:default\s+)?(?:abstract\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^>{}\n]+>)?(?:\s+extends\b[^\n{]+)?(?:\s+implements\b[^\n{]+)?\s*{`)
	tsInterfacePattern              = regexp.MustCompile(`(?m)^\s*(?:export\s+)?interface\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^>{}\n]+>)?`)
	tsTypePattern                   = regexp.MustCompile(`(?m)^\s*(?:export\s+)?type\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^>{}\n]+>)?\s*=`)
	tsEnumPattern                   = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:const\s+)?enum\s+([A-Za-z_$][A-Za-z0-9_$]*)\b`)
	tsFunctionPattern               = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^>{}\n]+>)?\s*\(`)
	tsArrowFunctionPattern          = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?::\s*[^=\n]+)?=\s*(?:async\s+)?(?:<[^=>\n]+>\s*)?(?:\([^\n]*?\)|[A-Za-z_$][A-Za-z0-9_$]*)\s*=>`)
	tsForwardRefPattern             = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?::\s*[^=\n]+)?=\s*(?:React\.)?forwardRef\b`)
	tsConstPattern                  = regexp.MustCompile(`(?m)^\s*(?:export\s+)?const\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?::\s*[^=\n]+)?=`)
	tsVarPattern                    = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?::\s*[^=\n]+)?=`)
	tsNamespacePattern              = regexp.MustCompile(`(?m)^\s*declare\s+(?:module|namespace)\s+([A-Za-z_$][A-Za-z0-9_$]*|"[^"]+"|'[^']+')`)
	tsExportDefaultReferencePattern = regexp.MustCompile(`(?m)^\s*export\s+default\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*;?`)
	tsClassMemberMethodPattern      = regexp.MustCompile(`(?m)^\s*(?:(?:public|private|protected|static|async|override|abstract|readonly)\s+)*(?:get\s+|set\s+)?([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:<[^>{}\n]+>)?\s*\([^\n]*?\)\s*(?::\s*[^\n{=;]+)?\s*(?:\{|;)`)
	tsClassPropertyPattern          = regexp.MustCompile(`(?m)^\s*(?:(?:public|private|protected|static|readonly|declare|abstract)\s+)+([A-Za-z_$][A-Za-z0-9_$]*)\s*[!?]?\s*(?::\s*[^=;\n{]+)?\s*(?:=|;)`)
)

// ExtractTypeScriptSymbols extracts TypeScript and TSX symbols using structural regexes.
func ExtractTypeScriptSymbols(filePath string, src []byte) ([]Symbol, error) {
	text := string(src)
	masked := maskTopLevelText(text)
	collector := newRegexSymbolCollector(filePath, "typescript", text)

	for _, match := range tsClassPattern.FindAllStringSubmatchIndex(masked, -1) {
		collector.addMatch(match, SymbolKindClass, "", matchContainsKeyword(masked, match, "export"))
		extractTypeScriptClassMembers(text, match, collector)
	}
	collector.addRegexMatches(masked, tsInterfacePattern, SymbolKindInterface, exportedFromMatch)
	collector.addRegexMatches(masked, tsTypePattern, SymbolKindType, exportedFromMatch)
	collector.addRegexMatches(masked, tsEnumPattern, SymbolKindEnum, exportedFromMatch)
	collector.addRegexMatches(masked, tsFunctionPattern, SymbolKindFunction, exportedFromMatch)
	collector.addRegexMatches(masked, tsArrowFunctionPattern, SymbolKindFunction, exportedFromMatch)
	collector.addRegexMatches(masked, tsForwardRefPattern, SymbolKindFunction, exportedFromMatch)
	collector.addRegexMatches(text, tsNamespacePattern, SymbolKindModule, neverExported)
	collector.addVariableMatches(masked, tsConstPattern, SymbolKindConstant)
	collector.addVariableMatches(masked, tsVarPattern, SymbolKindVariable)

	for _, match := range tsExportDefaultReferencePattern.FindAllStringSubmatchIndex(masked, -1) {
		collector.markExportedByName(text[match[2]:match[3]])
	}

	return collector.symbols(), nil
}

func extractTypeScriptClassMembers(text string, classMatch []int, collector *regexSymbolCollector) {
	className := collector.nameFromMatch(classMatch)
	bodyStart, bodyEnd := classBodyRange(text, classMatch)
	if bodyStart < 0 || bodyEnd < 0 || bodyEnd <= bodyStart {
		return
	}

	bodyMasked := maskTopLevelText(text[bodyStart:bodyEnd])
	for _, match := range tsClassMemberMethodPattern.FindAllStringSubmatchIndex(bodyMasked, -1) {
		name := bodyMasked[match[2]:match[3]]
		if name == "constructor" {
			continue
		}
		collector.addOffsets(name, SymbolKindMethod, className, bodyStart+match[2], bodyStart+match[3], false)
	}
	for _, match := range tsClassPropertyPattern.FindAllStringSubmatchIndex(bodyMasked, -1) {
		collector.addOffsets(bodyMasked[match[2]:match[3]], SymbolKindField, className, bodyStart+match[2], bodyStart+match[3], false)
	}
}

type exportResolver func(string, []int) bool

type regexSymbolCollector struct {
	filePath string
	language string
	text     string
	mapper   offsetMapper
	symbolsV []Symbol
	seen     map[string]int
}

func newRegexSymbolCollector(filePath string, language string, text string) *regexSymbolCollector {
	return &regexSymbolCollector{
		filePath: filePath,
		language: language,
		text:     text,
		mapper:   newOffsetMapper(text),
		seen:     make(map[string]int),
	}
}

func (c *regexSymbolCollector) symbols() []Symbol {
	symbols := append([]Symbol(nil), c.symbolsV...)
	sort.SliceStable(symbols, func(i int, j int) bool {
		if symbols[i].Range.StartLine != symbols[j].Range.StartLine {
			return symbols[i].Range.StartLine < symbols[j].Range.StartLine
		}
		if symbols[i].Range.StartColumn != symbols[j].Range.StartColumn {
			return symbols[i].Range.StartColumn < symbols[j].Range.StartColumn
		}
		if symbols[i].Container != symbols[j].Container {
			return symbols[i].Container < symbols[j].Container
		}
		if symbols[i].Kind != symbols[j].Kind {
			return symbols[i].Kind < symbols[j].Kind
		}
		return symbols[i].Name < symbols[j].Name
	})
	return symbols
}

func (c *regexSymbolCollector) addRegexMatches(masked string, re *regexp.Regexp, kind SymbolKind, resolve exportResolver) {
	for _, match := range re.FindAllStringSubmatchIndex(masked, -1) {
		c.addMatch(match, kind, "", resolve(masked, match))
	}
}

func (c *regexSymbolCollector) addVariableMatches(masked string, re *regexp.Regexp, kind SymbolKind) {
	for _, match := range re.FindAllStringSubmatchIndex(masked, -1) {
		name := c.nameFromMatch(match)
		if c.hasSymbol(name, SymbolKindFunction, "") || c.hasSymbol(name, SymbolKindClass, "") {
			continue
		}
		c.addMatch(match, kind, "", exportedFromMatch(masked, match))
	}
}

func (c *regexSymbolCollector) addOrMarkMatch(masked string, match []int, kind SymbolKind, resolve exportResolver) {
	name := c.nameFromMatch(match)
	if name == "" {
		return
	}
	if c.markExportedByName(name) {
		return
	}
	c.addMatch(match, kind, "", resolve(masked, match))
}

func (c *regexSymbolCollector) addMatch(match []int, kind SymbolKind, container string, exported bool) {
	if len(match) < 4 {
		return
	}
	c.addOffsets(c.text[match[2]:match[3]], kind, container, match[2], match[3], exported)
}

func (c *regexSymbolCollector) addOffsets(name string, kind SymbolKind, container string, start int, end int, exported bool) {
	if name == "" || start < 0 || end <= start || end > len(c.text) {
		return
	}
	if len(name) >= 2 && ((name[0] == '\'' && name[len(name)-1] == '\'') || (name[0] == '"' && name[len(name)-1] == '"')) {
		name = name[1 : len(name)-1]
	}
	key := symbolKey(name, kind, container, start, end)
	if idx, ok := c.seen[key]; ok {
		if exported {
			c.symbolsV[idx].Exported = true
		}
		return
	}
	c.seen[key] = len(c.symbolsV)
	c.symbolsV = append(c.symbolsV, Symbol{
		Name:      name,
		Kind:      kind,
		Language:  c.language,
		Path:      c.filePath,
		Container: container,
		Range:     c.mapper.rangeFromOffsets(start, end),
		Exported:  exported,
	})
}

func (c *regexSymbolCollector) hasSymbol(name string, kind SymbolKind, container string) bool {
	for _, symbol := range c.symbolsV {
		if symbol.Name == name && symbol.Kind == kind && symbol.Container == container {
			return true
		}
	}
	return false
}

func (c *regexSymbolCollector) markExportedByName(name string) bool {
	marked := false
	for idx := range c.symbolsV {
		if c.symbolsV[idx].Name == name && c.symbolsV[idx].Container == "" {
			c.symbolsV[idx].Exported = true
			marked = true
		}
	}
	return marked
}

func (c *regexSymbolCollector) nameFromMatch(match []int) string {
	if len(match) < 4 {
		return ""
	}
	return c.text[match[2]:match[3]]
}

func classBodyRange(text string, classMatch []int) (int, int) {
	if len(classMatch) < 2 {
		return -1, -1
	}
	segment := text[classMatch[0]:classMatch[1]]
	open := strings.LastIndex(segment, "{")
	if open < 0 {
		return -1, -1
	}
	open += classMatch[0]
	close := findMatchingBrace(text, open)
	if close < 0 {
		return -1, -1
	}
	return open + 1, close
}

func symbolKey(name string, kind SymbolKind, container string, start int, end int) string {
	return name + "|" + container + "|" + string(rune(kind)) + "|" + strconvItoa(start) + "|" + strconvItoa(end)
}

func matchContainsKeyword(text string, match []int, keyword string) bool {
	if len(match) < 2 {
		return false
	}
	return strings.Contains(text[match[0]:match[1]], keyword)
}

func exportedFromMatch(text string, match []int) bool {
	return matchContainsKeyword(text, match, "export")
}

func alwaysExported(_ string, _ []int) bool {
	return true
}

func neverExported(_ string, _ []int) bool {
	return false
}

func maskTopLevelText(text string) string {
	masked := []byte(text)
	depth := 0
	inLineComment := false
	inBlockComment := false
	var quote byte
	escaped := false

	for i := 0; i < len(masked); i++ {
		ch := masked[i]

		if quote != 0 {
			if ch != '\n' && ch != '\r' {
				masked[i] = ' '
			}
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}

		if inLineComment {
			if ch != '\n' && ch != '\r' {
				masked[i] = ' '
			} else {
				inLineComment = false
			}
			continue
		}

		if inBlockComment {
			if ch != '\n' && ch != '\r' {
				masked[i] = ' '
			}
			if ch == '*' && i+1 < len(masked) && masked[i+1] == '/' {
				masked[i+1] = ' '
				inBlockComment = false
				i++
			}
			continue
		}

		if ch == '/' && i+1 < len(masked) {
			next := masked[i+1]
			if next == '/' {
				masked[i], masked[i+1] = ' ', ' '
				inLineComment = true
				i++
				continue
			}
			if next == '*' {
				masked[i], masked[i+1] = ' ', ' '
				inBlockComment = true
				i++
				continue
			}
		}

		if ch == '\'' || ch == '"' || ch == '`' {
			if ch != '\n' && ch != '\r' {
				masked[i] = ' '
			}
			quote = ch
			escaped = false
			continue
		}

		switch ch {
		case '{':
			if depth > 0 {
				masked[i] = ' '
			}
			depth++
		case '}':
			depth--
			if depth > 0 {
				masked[i] = ' '
			}
			if depth < 0 {
				depth = 0
			}
		default:
			if depth > 0 && ch != '\n' && ch != '\r' {
				masked[i] = ' '
			}
		}
	}

	return string(masked)
}

func findMatchingBrace(text string, open int) int {
	depth := 0
	inLineComment := false
	inBlockComment := false
	var quote byte
	escaped := false

	for i := open; i < len(text); i++ {
		ch := text[i]

		if quote != 0 {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quote {
				quote = 0
			}
			continue
		}

		if inLineComment {
			if ch == '\n' {
				inLineComment = false
			}
			continue
		}

		if inBlockComment {
			if ch == '*' && i+1 < len(text) && text[i+1] == '/' {
				inBlockComment = false
				i++
			}
			continue
		}

		if ch == '/' && i+1 < len(text) {
			next := text[i+1]
			if next == '/' {
				inLineComment = true
				i++
				continue
			}
			if next == '*' {
				inBlockComment = true
				i++
				continue
			}
		}

		if ch == '\'' || ch == '"' || ch == '`' {
			quote = ch
			escaped = false
			continue
		}

		switch ch {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i
			}
		}
	}

	return -1
}

func strconvItoa(v int) string {
	if v == 0 {
		return "0"
	}
	negative := v < 0
	if negative {
		v = -v
	}
	var digits [20]byte
	idx := len(digits)
	for v > 0 {
		idx--
		digits[idx] = byte('0' + (v % 10))
		v /= 10
	}
	if negative {
		idx--
		digits[idx] = '-'
	}
	return string(digits[idx:])
}
