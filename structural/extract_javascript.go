package structural

import "regexp"

var (
	jsClassPattern                  = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:default\s+)?class\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*(?:extends\b[^\n{]+)?\s*{`)
	jsFunctionPattern               = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:default\s+)?(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`)
	jsArrowFunctionPattern          = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:async\s+)?(?:\([^\n]*?\)|[A-Za-z_$][A-Za-z0-9_$]*)\s*=>`)
	jsForwardRefPattern             = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:const|let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*(?:React\.)?forwardRef\b`)
	jsConstPattern                  = regexp.MustCompile(`(?m)^\s*(?:export\s+)?const\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=`)
	jsVarPattern                    = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:let|var)\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*=`)
	jsExportDefaultReferencePattern = regexp.MustCompile(`(?m)^\s*export\s+default\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*;?`)
	jsModuleExportsReferencePattern = regexp.MustCompile(`(?m)^\s*module\.exports\s*=\s*([A-Za-z_$][A-Za-z0-9_$]*)\s*;?`)
	jsModuleExportsFunctionPattern  = regexp.MustCompile(`(?m)^\s*module\.exports\s*=\s*(?:async\s+)?function\s+([A-Za-z_$][A-Za-z0-9_$]*)\s*\(`)
	jsModuleExportsClassPattern     = regexp.MustCompile(`(?m)^\s*module\.exports\s*=\s*class\s+([A-Za-z_$][A-Za-z0-9_$]*)\b`)
	jsNamedExportsPattern           = regexp.MustCompile(`(?m)^\s*exports\.([A-Za-z_$][A-Za-z0-9_$]*)\s*=\s*`)
	jsClassMemberMethodPattern      = regexp.MustCompile(`(?m)^\s*(?:(?:static|async)\s+)*(?:get\s+|set\s+)?([A-Za-z_$][A-Za-z0-9_$]*)\s*\([^\n]*?\)\s*{`)
	jsClassPropertyPattern          = regexp.MustCompile(`(?m)^\s*(?:(?:static)\s+)?([A-Za-z_$][A-Za-z0-9_$]*)\s*=`)
)

// ExtractJavaScriptSymbols extracts JavaScript and JSX symbols using structural regexes.
func ExtractJavaScriptSymbols(filePath string, src []byte) ([]Symbol, error) {
	text := string(src)
	masked := maskTopLevelText(text)
	collector := newRegexSymbolCollector(filePath, "javascript", text)

	for _, match := range jsClassPattern.FindAllStringSubmatchIndex(masked, -1) {
		collector.addMatch(match, SymbolKindClass, "", matchContainsKeyword(masked, match, "export"))
		extractJavaScriptClassMembers(text, match, collector)
	}
	collector.addRegexMatches(masked, jsFunctionPattern, SymbolKindFunction, exportedFromMatch)
	collector.addRegexMatches(masked, jsArrowFunctionPattern, SymbolKindFunction, exportedFromMatch)
	collector.addRegexMatches(masked, jsForwardRefPattern, SymbolKindFunction, exportedFromMatch)
	collector.addRegexMatches(masked, jsModuleExportsFunctionPattern, SymbolKindFunction, alwaysExported)
	collector.addRegexMatches(masked, jsModuleExportsClassPattern, SymbolKindClass, alwaysExported)
	collector.addVariableMatches(masked, jsConstPattern, SymbolKindConstant)
	collector.addVariableMatches(masked, jsVarPattern, SymbolKindVariable)

	for _, match := range jsExportDefaultReferencePattern.FindAllStringSubmatchIndex(masked, -1) {
		collector.markExportedByName(text[match[2]:match[3]])
	}
	for _, match := range jsModuleExportsReferencePattern.FindAllStringSubmatchIndex(masked, -1) {
		collector.markExportedByName(text[match[2]:match[3]])
	}
	for _, match := range jsNamedExportsPattern.FindAllStringSubmatchIndex(masked, -1) {
		collector.addOrMarkMatch(masked, match, SymbolKindVariable, alwaysExported)
	}

	return collector.symbols(), nil
}

func extractJavaScriptClassMembers(text string, classMatch []int, collector *regexSymbolCollector) {
	className := collector.nameFromMatch(classMatch)
	bodyStart, bodyEnd := classBodyRange(text, classMatch)
	if bodyStart < 0 || bodyEnd < 0 || bodyEnd <= bodyStart {
		return
	}

	bodyMasked := maskTopLevelText(text[bodyStart:bodyEnd])
	for _, match := range jsClassMemberMethodPattern.FindAllStringSubmatchIndex(bodyMasked, -1) {
		name := bodyMasked[match[2]:match[3]]
		if name == "constructor" {
			continue
		}
		collector.addOffsets(name, SymbolKindMethod, className, bodyStart+match[2], bodyStart+match[3], false)
	}
	for _, match := range jsClassPropertyPattern.FindAllStringSubmatchIndex(bodyMasked, -1) {
		collector.addOffsets(bodyMasked[match[2]:match[3]], SymbolKindField, className, bodyStart+match[2], bodyStart+match[3], false)
	}
}
