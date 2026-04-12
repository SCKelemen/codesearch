package structural

type offsetMapper struct {
	lineStarts []int
}

func newOffsetMapper(text string) offsetMapper {
	lineStarts := []int{0}
	for idx, r := range text {
		if r == '\n' {
			lineStarts = append(lineStarts, idx+1)
		}
	}
	return offsetMapper{lineStarts: lineStarts}
}

func (m offsetMapper) rangeFromOffsets(start int, end int) Range {
	startLine, startColumn := m.lineAndColumn(start)
	endLine, endColumn := m.lineAndColumn(end)
	return Range{
		StartLine:   startLine,
		StartColumn: startColumn,
		EndLine:     endLine,
		EndColumn:   endColumn,
	}
}

func (m offsetMapper) lineAndColumn(offset int) (int, int) {
	line := 0
	for idx := 0; idx < len(m.lineStarts); idx++ {
		if m.lineStarts[idx] > offset {
			break
		}
		line = idx
	}
	lineStart := m.lineStarts[line]
	return line + 1, offset - lineStart + 1
}
