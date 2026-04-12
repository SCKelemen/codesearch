package fuzzy

import (
	"fmt"
	"strings"
	"testing"
)

var fuzzyBenchResult Result
var fuzzyBenchScore int

func BenchmarkFuzzyMatch(b *testing.B) {
	text := benchmarkFuzzyText(1000)
	pattern := "HandleCheckoutRequest"
	opts := Options{CaseSensitive: false, WithPositions: true}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		fuzzyBenchResult = Match(text, pattern, opts)
	}
}

func BenchmarkFuzzySearch(b *testing.B) {
	items := benchmarkFuzzyItems(1000)
	pattern := "hndlchktrq"
	opts := Options{CaseSensitive: false}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		fuzzyBenchResult = benchmarkFuzzySearch(items, pattern, opts)
	}
}

func BenchmarkFuzzyScore(b *testing.B) {
	text := []rune(benchmarkFuzzyText(1000))
	pattern := []rune("HandleCheckoutRequest")
	matched := MatchV1(text, pattern, Options{CaseSensitive: false, WithPositions: true})
	if matched.Start < 0 {
		b.Fatal("expected prepared fuzzy match")
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		score, positions := calculateScore(text, pattern, matched.Start, matched.End, Options{CaseSensitive: false, WithPositions: true})
		fuzzyBenchScore = score + len(positions)
	}
}

func benchmarkFuzzyText(targetLen int) string {
	var builder strings.Builder
	builder.Grow(targetLen + 256)
	for segment := 0; builder.Len() < targetLen; segment++ {
		fmt.Fprintf(&builder, "CheckoutWorker%03d handles background reconciliation for HandleCheckoutRequest and sends audit events through WorkspaceBillingService. ", segment)
	}
	text := builder.String()
	return text[:targetLen]
}

func benchmarkFuzzyItems(count int) []string {
	items := make([]string, count)
	for i := range items {
		items[i] = fmt.Sprintf("services/workspace_%03d/HandleCheckoutRequestForWorkspaceBillingPipeline%03d.go", i, i)
	}
	return items
}

func benchmarkFuzzySearch(items []string, pattern string, opts Options) Result {
	best := Result{Start: -1, End: -1, Score: -1}
	for _, item := range items {
		match := Match(item, pattern, opts)
		if match.Score > best.Score {
			best = match
		}
	}
	return best
}
