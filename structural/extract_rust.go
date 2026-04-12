package structural

import "strings"

const rustLanguage = "rust"

type rustContext int

const (
	rustContextTop rustContext = iota
	rustContextImpl
	rustContextTrait
)

// ExtractRustSymbols extracts Rust symbols using a lightweight structural parser.
func ExtractRustSymbols(filePath string, src []byte) ([]Symbol, error) {
	text := string(src)
	masked := rustMaskSource(text)
	collector := newRegexSymbolCollector(filePath, rustLanguage, text)
	parseRustItems(masked, collector, 0, len(masked), "", rustContextTop, false)
	return collector.symbols(), nil
}

func parseRustItems(masked string, collector *regexSymbolCollector, start int, end int, container string, context rustContext, containerExported bool) {
	for i := start; i < end; {
		i = rustSkipSpaceAndAttributes(masked, i, end)
		if i >= end {
			return
		}

		itemStart := i
		afterVisibility, exported := rustParseVisibility(masked, i, end)
		base := rustSkipSpaceAndAttributes(masked, afterVisibility, end)
		qualified := rustSkipQualifiers(masked, base, end)

		switch {
		case rustHasKeyword(masked, base, "mod"):
			i = rustParseModule(masked, collector, base, end, exported)
		case rustHasKeyword(masked, base, "struct"):
			i = rustParseStruct(masked, collector, base, end, exported)
		case rustHasKeyword(masked, base, "enum"):
			i = rustParseEnum(masked, collector, base, end, exported)
		case rustHasKeyword(masked, qualified, "trait"):
			i = rustParseTrait(masked, collector, qualified, end, exported)
		case rustHasKeyword(masked, base, "impl"):
			i = rustParseImpl(masked, collector, base, end)
		case rustHasKeyword(masked, qualified, "fn"):
			i = rustParseFunction(masked, collector, qualified, end, container, context, exported, containerExported)
		case rustHasKeyword(masked, base, "const"):
			i = rustParseValueLike(masked, collector, base, end, SymbolKindConstant, container, context, exported, containerExported)
		case rustHasKeyword(masked, base, "static"):
			i = rustParseStatic(masked, collector, base, end, container, context, exported, containerExported)
		case rustHasKeyword(masked, base, "type"):
			i = rustParseTypeAlias(masked, collector, base, end, container, context, exported, containerExported)
		case rustHasKeyword(masked, base, "macro_rules"):
			i = rustParseMacroRules(masked, collector, base, end)
		default:
			i = rustAdvance(masked, itemStart, end)
		}

		if i <= itemStart {
			i = itemStart + 1
		}
	}
}

func rustParseModule(masked string, collector *regexSymbolCollector, pos int, end int, exported bool) int {
	name, nameStart, nameEnd, ok := rustReadIdentifierAfterKeyword(masked, pos, end, "mod")
	if !ok {
		return rustAdvance(masked, pos, end)
	}
	collector.addOffsets(name, SymbolKindModule, "", nameStart, nameEnd, exported)

	afterName := rustSkipAfterTypeName(masked, nameEnd, end)
	term := rustFindTopLevelDelimiter(masked, afterName, end, "{;")
	if term < 0 {
		return end
	}
	if masked[term] == '{' {
		close := rustFindMatchingBrace(masked, term)
		if close >= 0 {
			return close + 1
		}
		return end
	}
	return term + 1
}

func rustParseStruct(masked string, collector *regexSymbolCollector, pos int, end int, exported bool) int {
	name, nameStart, nameEnd, ok := rustReadIdentifierAfterKeyword(masked, pos, end, "struct")
	if !ok {
		return rustAdvance(masked, pos, end)
	}
	collector.addOffsets(name, SymbolKindStruct, "", nameStart, nameEnd, exported)

	afterName := rustSkipAfterTypeName(masked, nameEnd, end)
	term := rustFindTopLevelDelimiter(masked, afterName, end, "{(;")
	if term < 0 {
		return end
	}

	switch masked[term] {
	case '{':
		close := rustFindMatchingBrace(masked, term)
		if close < 0 {
			return end
		}
		rustParseStructFields(masked, collector, term+1, close, name)
		return close + 1
	case '(':
		close := rustFindMatchingDelimiter(masked, term, '(', ')')
		if close < 0 {
			return end
		}
		semi := rustFindTopLevelDelimiter(masked, close+1, end, ";")
		if semi >= 0 {
			return semi + 1
		}
		return close + 1
	default:
		return term + 1
	}
}

func rustParseEnum(masked string, collector *regexSymbolCollector, pos int, end int, exported bool) int {
	name, nameStart, nameEnd, ok := rustReadIdentifierAfterKeyword(masked, pos, end, "enum")
	if !ok {
		return rustAdvance(masked, pos, end)
	}
	collector.addOffsets(name, SymbolKindEnum, "", nameStart, nameEnd, exported)

	afterName := rustSkipAfterTypeName(masked, nameEnd, end)
	open := rustFindTopLevelDelimiter(masked, afterName, end, "{")
	if open < 0 {
		return end
	}
	close := rustFindMatchingBrace(masked, open)
	if close < 0 {
		return end
	}
	rustParseEnumVariants(masked, collector, open+1, close, name, exported)
	return close + 1
}

func rustParseTrait(masked string, collector *regexSymbolCollector, pos int, end int, exported bool) int {
	name, nameStart, nameEnd, ok := rustReadIdentifierAfterKeyword(masked, pos, end, "trait")
	if !ok {
		return rustAdvance(masked, pos, end)
	}
	collector.addOffsets(name, SymbolKindTrait, "", nameStart, nameEnd, exported)

	afterName := rustSkipAfterTypeName(masked, nameEnd, end)
	open := rustFindTopLevelDelimiter(masked, afterName, end, "{")
	if open < 0 {
		return end
	}
	close := rustFindMatchingBrace(masked, open)
	if close < 0 {
		return end
	}
	parseRustItems(masked, collector, open+1, close, name, rustContextTrait, exported)
	return close + 1
}

func rustParseImpl(masked string, collector *regexSymbolCollector, pos int, end int) int {
	afterImpl := rustSkipSpaceAndAttributes(masked, pos+len("impl"), end)
	if afterImpl < end && masked[afterImpl] == '<' {
		afterImpl = rustSkipBalanced(masked, afterImpl, end, '<', '>')
	}

	open := rustFindTopLevelDelimiter(masked, afterImpl, end, "{")
	if open < 0 {
		return end
	}
	close := rustFindMatchingBrace(masked, open)
	if close < 0 {
		return end
	}

	target := rustImplContainer(masked[afterImpl:open])
	parseRustItems(masked, collector, open+1, close, target, rustContextImpl, false)
	return close + 1
}

func rustParseFunction(masked string, collector *regexSymbolCollector, pos int, end int, container string, context rustContext, exported bool, containerExported bool) int {
	name, nameStart, nameEnd, ok := rustReadIdentifierAfterKeyword(masked, pos, end, "fn")
	if !ok {
		return rustAdvance(masked, pos, end)
	}

	kind := SymbolKindFunction
	if context == rustContextImpl || context == rustContextTrait {
		kind = SymbolKindMethod
		if !exported && context == rustContextTrait {
			exported = containerExported
		}
	}
	collector.addOffsets(name, kind, container, nameStart, nameEnd, exported)

	afterName := rustSkipAfterTypeName(masked, nameEnd, end)
	term := rustFindTopLevelDelimiter(masked, afterName, end, "{;")
	if term < 0 {
		return end
	}
	if masked[term] == '{' {
		close := rustFindMatchingBrace(masked, term)
		if close >= 0 {
			return close + 1
		}
		return end
	}
	return term + 1
}

func rustParseValueLike(masked string, collector *regexSymbolCollector, pos int, end int, kind SymbolKind, container string, context rustContext, exported bool, containerExported bool) int {
	name, nameStart, nameEnd, ok := rustReadIdentifierAfterKeyword(masked, pos, end, "const")
	if !ok {
		return rustAdvance(masked, pos, end)
	}
	if context == rustContextTrait && !exported {
		exported = containerExported
	}
	collector.addOffsets(name, kind, container, nameStart, nameEnd, exported)

	term := rustFindTopLevelDelimiter(masked, nameEnd, end, ";")
	if term < 0 {
		return end
	}
	return term + 1
}

func rustParseStatic(masked string, collector *regexSymbolCollector, pos int, end int, container string, context rustContext, exported bool, containerExported bool) int {
	afterStatic := rustSkipSpaceAndAttributes(masked, pos+len("static"), end)
	if rustHasKeyword(masked, afterStatic, "mut") {
		afterStatic = rustSkipSpaceAndAttributes(masked, afterStatic+len("mut"), end)
	}
	name, next, ok := rustReadIdentifier(masked, afterStatic, end)
	if !ok {
		return rustAdvance(masked, pos, end)
	}
	if context == rustContextTrait && !exported {
		exported = containerExported
	}
	collector.addOffsets(name, SymbolKindVariable, container, afterStatic, next, exported)
	term := rustFindTopLevelDelimiter(masked, next, end, ";")
	if term < 0 {
		return end
	}
	return term + 1
}

func rustParseTypeAlias(masked string, collector *regexSymbolCollector, pos int, end int, container string, context rustContext, exported bool, containerExported bool) int {
	name, nameStart, nameEnd, ok := rustReadIdentifierAfterKeyword(masked, pos, end, "type")
	if !ok {
		return rustAdvance(masked, pos, end)
	}
	if context == rustContextTrait && !exported {
		exported = containerExported
	}
	collector.addOffsets(name, SymbolKindType, container, nameStart, nameEnd, exported)
	term := rustFindTopLevelDelimiter(masked, nameEnd, end, ";")
	if term < 0 {
		return end
	}
	return term + 1
}

func rustParseMacroRules(masked string, collector *regexSymbolCollector, pos int, end int) int {
	afterMacro := rustSkipSpaceAndAttributes(masked, pos+len("macro_rules"), end)
	if afterMacro >= end || masked[afterMacro] != '!' {
		return rustAdvance(masked, pos, end)
	}
	afterBang := rustSkipSpaceAndAttributes(masked, afterMacro+1, end)
	name, next, ok := rustReadIdentifier(masked, afterBang, end)
	if !ok {
		return rustAdvance(masked, pos, end)
	}
	collector.addOffsets(name, SymbolKindFunction, "", afterBang, next, false)

	open := rustFindTopLevelDelimiter(masked, next, end, "{[(")
	if open < 0 {
		return end
	}

	close := -1
	switch masked[open] {
	case '{':
		close = rustFindMatchingBrace(masked, open)
	case '[':
		close = rustFindMatchingDelimiter(masked, open, '[', ']')
	case '(':
		close = rustFindMatchingDelimiter(masked, open, '(', ')')
	}
	if close < 0 {
		return end
	}
	return close + 1
}

func rustParseStructFields(masked string, collector *regexSymbolCollector, start int, end int, container string) {
	for i := start; i < end; {
		i = rustSkipSpaceAndAttributes(masked, i, end)
		if i >= end {
			return
		}

		afterVisibility, exported := rustParseVisibility(masked, i, end)
		fieldStart := rustSkipSpaceAndAttributes(masked, afterVisibility, end)
		name, next, ok := rustReadIdentifier(masked, fieldStart, end)
		if !ok {
			i = rustAdvanceToNextComma(masked, i, end)
			continue
		}
		colon := rustFindTopLevelDelimiter(masked, next, end, ":,")
		if colon >= 0 && masked[colon] == ':' {
			collector.addOffsets(name, SymbolKindField, container, fieldStart, next, exported)
		}
		i = rustAdvanceToNextComma(masked, i, end)
	}
}

func rustParseEnumVariants(masked string, collector *regexSymbolCollector, start int, end int, container string, exported bool) {
	for i := start; i < end; {
		i = rustSkipSpaceAndAttributes(masked, i, end)
		if i >= end {
			return
		}
		name, next, ok := rustReadIdentifier(masked, i, end)
		if ok {
			collector.addOffsets(name, SymbolKindEnumMember, container, i, next, exported)
		}
		i = rustAdvanceToNextComma(masked, i, end)
	}
}

func rustImplContainer(header string) string {
	header = strings.TrimSpace(header)
	if header == "" {
		return ""
	}
	if idx := rustLastTopLevelFor(header); idx >= 0 {
		header = header[idx+len("for"):]
	}
	if idx := rustTopLevelWhere(header); idx >= 0 {
		header = header[:idx]
	}
	return rustLastTypeIdentifier(header)
}

func rustLastTopLevelFor(text string) int {
	angleDepth := 0
	parenDepth := 0
	bracketDepth := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '<':
			angleDepth++
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		}
		if angleDepth == 0 && parenDepth == 0 && bracketDepth == 0 && rustHasKeyword(text, i, "for") {
			return i
		}
	}
	return -1
}

func rustTopLevelWhere(text string) int {
	angleDepth := 0
	parenDepth := 0
	bracketDepth := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '<':
			angleDepth++
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		}
		if angleDepth == 0 && parenDepth == 0 && bracketDepth == 0 && rustHasKeyword(text, i, "where") {
			return i
		}
	}
	return -1
}

func rustLastTypeIdentifier(text string) string {
	angleDepth := 0
	parenDepth := 0
	bracketDepth := 0
	for i := len(text) - 1; i >= 0; i-- {
		switch text[i] {
		case '>':
			angleDepth++
		case '<':
			if angleDepth > 0 {
				angleDepth--
			}
		case ')':
			parenDepth++
		case '(':
			if parenDepth > 0 {
				parenDepth--
			}
		case ']':
			bracketDepth++
		case '[':
			if bracketDepth > 0 {
				bracketDepth--
			}
		default:
			if angleDepth == 0 && parenDepth == 0 && bracketDepth == 0 && rustIsIdentContinue(text[i]) {
				start := i
				for start >= 0 && rustIsIdentContinue(text[start]) {
					start--
				}
				name := text[start+1 : i+1]
				if start >= 1 && text[start] == '#' && text[start-1] == 'r' {
					name = text[start-1 : i+1]
				}
				name = rustNormalizeIdentifier(name)
				if name != "" && !rustIsKeyword(name) {
					return name
				}
				i = start
			}
		}
	}
	return ""
}

func rustReadIdentifierAfterKeyword(text string, pos int, end int, keyword string) (string, int, int, bool) {
	after := rustSkipSpaceAndAttributes(text, pos+len(keyword), end)
	name, next, ok := rustReadIdentifier(text, after, end)
	if !ok {
		return "", 0, 0, false
	}
	return name, after, next, true
}

func rustReadIdentifier(text string, pos int, end int) (string, int, bool) {
	pos = rustSkipSpaceAndAttributes(text, pos, end)
	if pos >= end {
		return "", pos, false
	}
	start := pos
	if strings.HasPrefix(text[pos:end], "r#") {
		pos += 2
		start = pos - 2
	}
	if pos >= end || !rustIsIdentStart(text[pos]) {
		return "", start, false
	}
	pos++
	for pos < end && rustIsIdentContinue(text[pos]) {
		pos++
	}
	return rustNormalizeIdentifier(text[start:pos]), pos, true
}

func rustNormalizeIdentifier(name string) string {
	return strings.TrimPrefix(name, "r#")
}

func rustIsKeyword(name string) bool {
	switch name {
	case "crate", "self", "Self", "super", "dyn":
		return true
	default:
		return false
	}
}

func rustParseVisibility(text string, pos int, end int) (int, bool) {
	pos = rustSkipSpaceAndAttributes(text, pos, end)
	if !rustHasKeyword(text, pos, "pub") {
		return pos, false
	}
	pos = rustSkipSpace(text, pos+len("pub"), end)
	if pos < end && text[pos] == '(' {
		pos = rustSkipBalanced(text, pos, end, '(', ')')
	}
	return pos, true
}

func rustSkipQualifiers(text string, pos int, end int) int {
	for {
		pos = rustSkipSpaceAndAttributes(text, pos, end)
		switched := false
		for _, qualifier := range []string{"async", "const", "unsafe", "default", "extern"} {
			if rustHasKeyword(text, pos, qualifier) {
				pos = rustSkipSpaceAndAttributes(text, pos+len(qualifier), end)
				switched = true
				break
			}
		}
		if !switched {
			return pos
		}
	}
}

func rustSkipAfterTypeName(text string, pos int, end int) int {
	pos = rustSkipSpaceAndAttributes(text, pos, end)
	if pos < end && text[pos] == '<' {
		pos = rustSkipBalanced(text, pos, end, '<', '>')
	}
	return rustSkipSpaceAndAttributes(text, pos, end)
}

func rustSkipSpaceAndAttributes(text string, pos int, end int) int {
	for {
		pos = rustSkipSpace(text, pos, end)
		if pos >= end || text[pos] != '#' {
			return pos
		}
		next := pos + 1
		if next < end && text[next] == '!' {
			next++
		}
		if next >= end || text[next] != '[' {
			return pos
		}
		pos = rustSkipBalanced(text, next, end, '[', ']')
	}
}

func rustSkipSpace(text string, pos int, end int) int {
	for pos < end {
		switch text[pos] {
		case ' ', '\t', '\n', '\r', '\f', '\v':
			pos++
		default:
			return pos
		}
	}
	return pos
}

func rustSkipBalanced(text string, pos int, end int, open byte, close byte) int {
	if pos >= end || text[pos] != open {
		return pos
	}
	depth := 0
	for pos < end {
		switch text[pos] {
		case open:
			depth++
		case close:
			depth--
			if depth == 0 {
				return pos + 1
			}
		}
		pos++
	}
	return end
}

func rustFindTopLevelDelimiter(text string, start int, end int, delimiters string) int {
	angleDepth := 0
	parenDepth := 0
	bracketDepth := 0
	for i := start; i < end; i++ {
		if angleDepth == 0 && parenDepth == 0 && bracketDepth == 0 && strings.IndexByte(delimiters, text[i]) >= 0 {
			return i
		}
		switch text[i] {
		case '<':
			angleDepth++
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		}
	}
	return -1
}

func rustFindMatchingDelimiter(text string, open int, openDelim byte, closeDelim byte) int {
	if open < 0 || open >= len(text) || text[open] != openDelim {
		return -1
	}
	depth := 0
	for i := open; i < len(text); i++ {
		switch text[i] {
		case openDelim:
			depth++
		case closeDelim:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func rustFindMatchingBrace(text string, open int) int {
	if open < 0 || open >= len(text) || text[open] != '{' {
		return -1
	}
	depth := 0
	for i := open; i < len(text); i++ {
		switch text[i] {
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

func rustAdvance(text string, pos int, end int) int {
	for i := pos; i < end; i++ {
		switch text[i] {
		case ';', '{', '}':
			return i + 1
		}
	}
	return end
}

func rustAdvanceToNextComma(text string, pos int, end int) int {
	angleDepth := 0
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	for i := pos; i < end; i++ {
		switch text[i] {
		case '<':
			angleDepth++
		case '>':
			if angleDepth > 0 {
				angleDepth--
			}
		case '(':
			parenDepth++
		case ')':
			if parenDepth > 0 {
				parenDepth--
			}
		case '[':
			bracketDepth++
		case ']':
			if bracketDepth > 0 {
				bracketDepth--
			}
		case '{':
			braceDepth++
		case '}':
			if braceDepth > 0 {
				braceDepth--
			}
		case ',':
			if angleDepth == 0 && parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
				return i + 1
			}
		}
	}
	return end
}

func rustHasKeyword(text string, pos int, keyword string) bool {
	if pos < 0 || pos+len(keyword) > len(text) || !strings.HasPrefix(text[pos:], keyword) {
		return false
	}
	if pos > 0 && rustIsIdentContinue(text[pos-1]) {
		return false
	}
	after := pos + len(keyword)
	if after < len(text) && rustIsIdentContinue(text[after]) {
		return false
	}
	return true
}

func rustIsIdentStart(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch == '_'
}

func rustIsIdentContinue(ch byte) bool {
	return rustIsIdentStart(ch) || (ch >= '0' && ch <= '9')
}

func rustMaskSource(text string) string {
	masked := []byte(text)
	for i := 0; i < len(masked); {
		switch {
		case i+1 < len(masked) && masked[i] == '/' && masked[i+1] == '/':
			end := i + 2
			for end < len(masked) && masked[end] != '\n' {
				end++
			}
			rustBlank(masked, i, end)
			i = end
		case i+1 < len(masked) && masked[i] == '/' && masked[i+1] == '*':
			end := rustScanBlockComment(text, i)
			rustBlank(masked, i, end)
			i = end
		case rustRawStringStart(text, i):
			end := rustScanRawString(text, i)
			rustBlank(masked, i, end)
			i = end
		case rustByteStringStart(text, i):
			end := rustScanQuotedLiteral(text, i+1, '"')
			rustBlank(masked, i, end)
			i = end
		case masked[i] == '"':
			end := rustScanQuotedLiteral(text, i, '"')
			rustBlank(masked, i, end)
			i = end
		default:
			i++
		}
	}
	return string(masked)
}

func rustBlank(masked []byte, start int, end int) {
	if end > len(masked) {
		end = len(masked)
	}
	for i := start; i < end; i++ {
		if masked[i] != '\n' && masked[i] != '\r' {
			masked[i] = ' '
		}
	}
}

func rustScanBlockComment(text string, start int) int {
	depth := 0
	for i := start; i < len(text)-1; i++ {
		if text[i] == '/' && text[i+1] == '*' {
			depth++
			i++
			continue
		}
		if text[i] == '*' && text[i+1] == '/' {
			depth--
			i++
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(text)
}

func rustRawStringStart(text string, start int) bool {
	if start >= len(text) {
		return false
	}
	prefix := start
	if text[prefix] == 'b' {
		prefix++
	}
	if prefix >= len(text) || text[prefix] != 'r' {
		return false
	}
	prefix++
	for prefix < len(text) && text[prefix] == '#' {
		prefix++
	}
	return prefix < len(text) && text[prefix] == '"'
}

func rustByteStringStart(text string, start int) bool {
	return start+1 < len(text) && text[start] == 'b' && text[start+1] == '"'
}

func rustScanRawString(text string, start int) int {
	pos := start
	if text[pos] == 'b' {
		pos++
	}
	pos++
	hashes := 0
	for pos < len(text) && text[pos] == '#' {
		hashes++
		pos++
	}
	if pos >= len(text) || text[pos] != '"' {
		return start + 1
	}
	pos++
	for pos < len(text) {
		if text[pos] != '"' {
			pos++
			continue
		}
		match := true
		for h := 0; h < hashes; h++ {
			if pos+1+h >= len(text) || text[pos+1+h] != '#' {
				match = false
				break
			}
		}
		if match {
			return pos + 1 + hashes
		}
		pos++
	}
	return len(text)
}

func rustScanQuotedLiteral(text string, start int, quote byte) int {
	if start >= len(text) {
		return start
	}
	pos := start + 1
	escaped := false
	for pos < len(text) {
		ch := text[pos]
		if ch == '\n' || ch == '\r' {
			return start
		}
		if escaped {
			escaped = false
			pos++
			continue
		}
		if ch == '\\' {
			escaped = true
			pos++
			continue
		}
		if ch == quote {
			return pos + 1
		}
		pos++
	}
	return start
}
