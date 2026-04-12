package exact

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

var exactBenchMatches []Match
var exactBenchCount int

func BenchmarkExactMatch(b *testing.B) {
	content := benchmarkExactContent(10 * 1024)
	idx := NewIndex(content)

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		exactBenchMatches = idx.SearchString("HandleCheckoutRequest")
	}
}

func BenchmarkExactMatchCaseInsensitive(b *testing.B) {
	content := benchmarkExactContent(10 * 1024)
	lowerContent := bytes.ToLower(content)
	pattern := []byte("handlecheckoutrequest")

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		exactBenchCount = benchmarkCaseInsensitiveMatch(lowerContent, pattern)
	}
}

func BenchmarkExactSearch(b *testing.B) {
	indexes := make([]*Index, 1000)
	for i := range indexes {
		indexes[i] = NewIndex(benchmarkExactDocument(i))
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		exactBenchCount = benchmarkExactSearch(indexes, "HandleCheckoutRequest")
	}
}

func benchmarkExactContent(targetBytes int) []byte {
	var builder strings.Builder
	builder.Grow(targetBytes + 512)
	for section := 0; builder.Len() < targetBytes; section++ {
		fmt.Fprintf(&builder, "package bench\n\nfunc HandleCheckoutRequest%03d() string {\n", section)
		fmt.Fprintf(&builder, "\tmessage := \"HandleCheckoutRequest processed invoice and emitted audit trail\"\n")
		fmt.Fprintf(&builder, "\treturn message\n")
		fmt.Fprintf(&builder, "}\n\n")
	}
	return []byte(builder.String()[:targetBytes])
}

func benchmarkExactDocument(docID int) []byte {
	var builder strings.Builder
	for line := 0; line < 32; line++ {
		fmt.Fprintf(&builder, "file_%03d_line_%02d HandleCheckoutRequest records billing events and workspace ownership metadata\n", docID, line)
	}
	return []byte(builder.String())
}

func benchmarkCaseInsensitiveMatch(content []byte, pattern []byte) int {
	count := 0
	offset := 0
	for {
		idx := bytes.Index(content[offset:], pattern)
		if idx < 0 {
			return count
		}
		count++
		offset += idx + len(pattern)
	}
}

func benchmarkExactSearch(indexes []*Index, pattern string) int {
	total := 0
	for _, idx := range indexes {
		total += len(idx.SearchString(pattern))
	}
	return total
}
