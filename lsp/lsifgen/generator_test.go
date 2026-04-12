package lsifgen

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestIsSupportedExt(t *testing.T) {
	if !IsSupportedExt(".go") {
		t.Fatal("expected .go to be supported")
	}
	if !IsSupportedExt("tsx") {
		t.Fatal("expected tsx without a leading dot to be supported")
	}
	if IsSupportedExt(".unknown") {
		t.Fatal("expected .unknown to be unsupported")
	}
}

func TestFormatStats(t *testing.T) {
	stats := &Stats{
		Documents:   2,
		Symbols:     3,
		References:  4,
		HoverInfos:  5,
		Definitions: 6,
		Errors:      1,
		Languages: map[string]int{
			"go":         1,
			"typescript": 1,
		},
	}
	got := FormatStats(stats)
	for _, want := range []string{
		"documents=2",
		"symbols=3",
		"references=4",
		"hover infos=5",
		"definitions=6",
		"errors=1",
		"go=1",
		"typescript=1",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("FormatStats() = %q, want substring %q", got, want)
		}
	}
}

func TestEmitVertex(t *testing.T) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	g := NewGenerator(nil)

	id := g.emitVertex(enc, "metaData", map[string]any{"version": "0.4.3"})
	if id != 1 {
		t.Fatalf("emitVertex() id = %d, want 1", id)
	}

	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got["type"] != "vertex" {
		t.Fatalf("type = %v, want vertex", got["type"])
	}
	if got["label"] != "metaData" {
		t.Fatalf("label = %v, want metaData", got["label"])
	}
	if got["version"] != "0.4.3" {
		t.Fatalf("version = %v, want 0.4.3", got["version"])
	}
}

func TestEmitEdge(t *testing.T) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	g := NewGenerator(nil)

	g.emitEdge(enc, "contains", 1, 0, []int{2, 3})

	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &got); err != nil {
		t.Fatalf("Unmarshal() error = %v", err)
	}
	if got["type"] != "edge" {
		t.Fatalf("type = %v, want edge", got["type"])
	}
	if got["label"] != "contains" {
		t.Fatalf("label = %v, want contains", got["label"])
	}
	if got["outV"].(float64) != 1 {
		t.Fatalf("outV = %v, want 1", got["outV"])
	}
	inVs, ok := got["inVs"].([]any)
	if !ok || len(inVs) != 2 {
		t.Fatalf("inVs = %#v, want 2 entries", got["inVs"])
	}
	if inVs[0].(float64) != 2 || inVs[1].(float64) != 3 {
		t.Fatalf("inVs = %#v, want [2 3]", inVs)
	}
}
