package ranking

import (
	"fmt"
	"math"
	"testing"
)

var rankingBenchScore float64
var rankingBenchDocs []*Document
var rankingBenchScores []float64

func BenchmarkScore(b *testing.B) {
	ranker, query, docs, freqs := benchmarkRankingData(1)
	doc := docs[0]
	termFreqs := freqs[doc.URI]

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rankingBenchScore = ranker.Score(doc, query, termFreqs)
	}
}

func BenchmarkRankResults(b *testing.B) {
	ranker, query, baseDocs, freqs := benchmarkRankingData(1000)
	docs := make([]*Document, len(baseDocs))

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		b.StopTimer()
		for idx, doc := range baseDocs {
			copyDoc := *doc
			docs[idx] = &copyDoc
		}
		b.StartTimer()

		ranker.Rank(docs, query, freqs)
	}

	rankingBenchDocs = docs
}

func BenchmarkNormalize(b *testing.B) {
	scores := make([]float64, 1000)
	for i := range scores {
		scores[i] = math.Sin(float64(i)/17) + float64(i%29)/7 + float64(i)/1000
	}

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		rankingBenchScores = benchmarkNormalize(scores)
	}
}

func benchmarkRankingData(count int) (*Ranker, Query, []*Document, map[string]map[string]int) {
	docFreqs := map[string]int{
		"checkout": 120,
		"request":  340,
		"billing":  180,
	}
	query := Query{Terms: []string{"checkout", "request", "billing"}, IsExact: true}
	docs := make([]*Document, count)
	freqs := make(map[string]map[string]int, count)
	for i := 0; i < count; i++ {
		uri := fmt.Sprintf("services/billing/checkout_%04d.go", i)
		docs[i] = &Document{
			URI:            uri,
			Language:       "Go",
			LineCount:      40 + i%300,
			IsDefinition:   i%5 == 0,
			IsExported:     i%3 == 0,
			ReferenceCount: 2 + i%200,
			LastModified:   int64(1_700_000_000 + i),
			ImportCount:    1 + i%40,
			Depth:          1 + i%6,
		}
		freqs[uri] = map[string]int{
			"checkout": 1 + i%6,
			"request":  2 + i%5,
			"billing":  1 + i%4,
		}
	}
	return NewRanker(DefaultConfig(), count, 120, docFreqs), query, docs, freqs
}

func benchmarkNormalize(scores []float64) []float64 {
	if len(scores) == 0 {
		return nil
	}
	minScore := scores[0]
	maxScore := scores[0]
	for _, score := range scores[1:] {
		if score < minScore {
			minScore = score
		}
		if score > maxScore {
			maxScore = score
		}
	}
	out := make([]float64, len(scores))
	if maxScore == minScore {
		for i := range out {
			out[i] = 1
		}
		return out
	}
	scale := maxScore - minScore
	for i, score := range scores {
		out[i] = (score - minScore) / scale
	}
	return out
}
