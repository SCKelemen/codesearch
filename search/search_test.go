package search

import (
	"context"
	"testing"

	"github.com/SCKelemen/codesearch/exact"
	"github.com/SCKelemen/codesearch/symbol"
)

func TestEngineExactSearch(t *testing.T) {
	t.Parallel()
	idx := exact.NewIndex([]byte("func hello() {\n\tprintln(\"hello world\")\n}"))
	engine := NewEngine(WithExactIndex("main.go", idx))

	results, err := engine.Search(context.Background(), Request{
		Query: "hello",
		Mode:  ModeExact,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	if results[0].Mode != ModeExact {
		t.Errorf("expected ModeExact, got %d", results[0].Mode)
	}
}

func TestEngineSymbolSearch(t *testing.T) {
	t.Parallel()
	symIdx := symbol.NewIndex()
	symIdx.Add(symbol.Symbol{
		Name: "HandleRequest", Kind: symbol.KindFunction,
		Location:     symbol.Location{URI: "handler.go", StartLine: 10},
		IsDefinition: true, IsExported: true,
	})

	engine := NewEngine(WithSymbolIndex(symIdx))
	results, err := engine.Search(context.Background(), Request{
		Query: "HandleRequest",
		Mode:  ModeSymbol,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].SymbolName != "HandleRequest" {
		t.Errorf("expected HandleRequest, got %s", results[0].SymbolName)
	}
}

func TestEngineFuzzySearch(t *testing.T) {
	t.Parallel()
	symIdx := symbol.NewIndex()
	symIdx.Add(symbol.Symbol{Name: "HandleRequest", Kind: symbol.KindFunction, Location: symbol.Location{URI: "a.go"}})
	symIdx.Add(symbol.Symbol{Name: "ProcessOrder", Kind: symbol.KindFunction, Location: symbol.Location{URI: "b.go"}})
	symIdx.Add(symbol.Symbol{Name: "HandleError", Kind: symbol.KindFunction, Location: symbol.Location{URI: "c.go"}})

	engine := NewEngine(WithSymbolIndex(symIdx))
	results, err := engine.Search(context.Background(), Request{
		Query: "HndReq",
		Mode:  ModeFuzzy,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected fuzzy results")
	}
	// HandleRequest should be the top result
	if results[0].SymbolName != "HandleRequest" {
		t.Errorf("expected HandleRequest first, got %s", results[0].SymbolName)
	}
}

func TestEngineAutoDetect(t *testing.T) {
	t.Parallel()
	tests := []struct {
		query string
		want  Mode
	}{
		{"hello.*world", ModeRegex},
		{"HandleRequest", ModeSymbol},
		{"hello", ModeExact},
	}
	for _, tt := range tests {
		got := detectMode(tt.query)
		if got != tt.want {
			t.Errorf("detectMode(%q) = %d, want %d", tt.query, got, tt.want)
		}
	}
}

func TestEngineMaxResults(t *testing.T) {
	t.Parallel()
	idx := exact.NewIndex([]byte("aaa\naaa\naaa\naaa\naaa"))
	engine := NewEngine(WithExactIndex("test.txt", idx))
	results, err := engine.Search(context.Background(), Request{
		Query:      "aaa",
		Mode:       ModeExact,
		MaxResults: 2,
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) > 2 {
		t.Errorf("expected <= 2 results, got %d", len(results))
	}
}
