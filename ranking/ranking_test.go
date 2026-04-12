package ranking

import "testing"

func TestBM25Scoring(t *testing.T) {
	t.Parallel()
	r := NewRanker(DefaultConfig(), 1000, 100, map[string]int{"hello": 10, "world": 500})
	doc := &Document{URI: "test.go", LineCount: 50}
	query := Query{Terms: []string{"hello", "world"}}
	score := r.Score(doc, query, map[string]int{"hello": 3, "world": 1})
	if score <= 0 {
		t.Errorf("expected positive score, got %f", score)
	}
}

func TestDefinitionBoost(t *testing.T) {
	t.Parallel()
	r := NewRanker(DefaultConfig(), 100, 50, map[string]int{"foo": 5})
	query := Query{Terms: []string{"foo"}}
	freqs := map[string]int{"foo": 1}

	def := &Document{URI: "def.go", LineCount: 50, IsDefinition: true, IsExported: true}
	ref := &Document{URI: "ref.go", LineCount: 50, IsDefinition: false, IsExported: false}

	defScore := r.Score(def, query, freqs)
	refScore := r.Score(ref, query, freqs)

	if defScore <= refScore {
		t.Errorf("definition (%f) should rank higher than reference (%f)", defScore, refScore)
	}
}

func TestRank(t *testing.T) {
	t.Parallel()
	r := NewRanker(DefaultConfig(), 100, 50, map[string]int{"foo": 5})
	docs := []*Document{
		{URI: "low.go", LineCount: 200},
		{URI: "high.go", LineCount: 50, IsDefinition: true, IsExported: true, ReferenceCount: 100},
	}
	query := Query{Terms: []string{"foo"}}
	freqMap := map[string]map[string]int{
		"low.go":  {"foo": 1},
		"high.go": {"foo": 3},
	}
	r.Rank(docs, query, freqMap)
	if docs[0].URI != "high.go" {
		t.Errorf("expected high.go first, got %s", docs[0].URI)
	}
}

func TestDefaultConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultConfig()
	if cfg.K1 <= 0 || cfg.B <= 0 {
		t.Error("BM25 params should be positive")
	}
}
