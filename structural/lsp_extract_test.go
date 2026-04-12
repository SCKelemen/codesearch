package structural

import (
	"context"
	"reflect"
	"testing"

	"github.com/SCKelemen/codesearch/lsp"
)

func TestLspKindToSymbolKind(t *testing.T) {
	tests := []struct {
		name    string
		lspKind int
		want    SymbolKind
	}{
		{name: "unknown", lspKind: 1, want: SymbolKindUnknown},
		{name: "module", lspKind: 2, want: SymbolKindModule},
		{name: "namespace", lspKind: 3, want: SymbolKindModule},
		{name: "package", lspKind: 4, want: SymbolKindPackage},
		{name: "class", lspKind: 5, want: SymbolKindClass},
		{name: "method", lspKind: 6, want: SymbolKindMethod},
		{name: "property", lspKind: 7, want: SymbolKindField},
		{name: "field", lspKind: 8, want: SymbolKindField},
		{name: "constructor", lspKind: 9, want: SymbolKindMethod},
		{name: "enum", lspKind: 10, want: SymbolKindEnum},
		{name: "interface", lspKind: 11, want: SymbolKindInterface},
		{name: "function", lspKind: 12, want: SymbolKindFunction},
		{name: "variable", lspKind: 13, want: SymbolKindVariable},
		{name: "constant", lspKind: 14, want: SymbolKindConstant},
		{name: "enum member", lspKind: 22, want: SymbolKindEnumMember},
		{name: "struct", lspKind: 23, want: SymbolKindStruct},
		{name: "type parameter", lspKind: 26, want: SymbolKindType},
		{name: "unexpected", lspKind: 999, want: SymbolKindUnknown},
	}

	for _, tt := range tests {
		if got := lspKindToSymbolKind(tt.lspKind); got != tt.want {
			t.Fatalf("%s: lspKindToSymbolKind(%d) = %v, want %v", tt.name, tt.lspKind, got, tt.want)
		}
	}
}

func TestExtractWithLSP_Fallback(t *testing.T) {
	ctx := context.Background()
	mux := &lsp.Multiplexer{}
	src := []byte(`package sample

type Widget struct {
	Name string
}

func (w *Widget) Render() {}
`)

	want, err := ExtractSymbols("sample.go", src)
	if err != nil {
		t.Fatalf("ExtractSymbols() error = %v", err)
	}

	got, err := ExtractWithLSP(ctx, mux, "sample.go", src)
	if err != nil {
		t.Fatalf("ExtractWithLSP() error = %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractWithLSP() fallback = %#v, want %#v", got, want)
	}
}
