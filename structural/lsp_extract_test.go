package structural

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"github.com/SCKelemen/codesearch/lsp"
)

func TestLspKindToSymbolKind_AllKinds(t *testing.T) {
	t.Parallel()

	wants := []SymbolKind{
		0,
		SymbolKindUnknown,
		SymbolKindModule,
		SymbolKindModule,
		SymbolKindPackage,
		SymbolKindClass,
		SymbolKindMethod,
		SymbolKindField,
		SymbolKindField,
		SymbolKindMethod,
		SymbolKindEnum,
		SymbolKindInterface,
		SymbolKindFunction,
		SymbolKindVariable,
		SymbolKindConstant,
		SymbolKindVariable,
		SymbolKindVariable,
		SymbolKindVariable,
		SymbolKindVariable,
		SymbolKindVariable,
		SymbolKindField,
		SymbolKindVariable,
		SymbolKindEnumMember,
		SymbolKindStruct,
		SymbolKindFunction,
		SymbolKindFunction,
		SymbolKindType,
	}

	for kind := 1; kind <= 26; kind++ {
		if got := lspKindToSymbolKind(kind); got != wants[kind] {
			t.Fatalf("lspKindToSymbolKind(%d) = %v, want %v", kind, got, wants[kind])
		}
	}
}

func TestFlattenLSPSymbols_Empty(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name    string
		symbols []lsp.Symbol
	}{
		{name: "nil", symbols: nil},
		{name: "empty", symbols: []lsp.Symbol{}},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := flattenLSPSymbols(tc.symbols, "root")
			if len(got) != 0 {
				t.Fatalf("flattenLSPSymbols() len = %d, want 0", len(got))
			}
		})
	}
}

func TestFlattenLSPSymbols_Nested(t *testing.T) {
	t.Parallel()

	symbols := []lsp.Symbol{
		{
			Name: "Service",
			Kind: 5,
			SelectionRange: lsp.Range{
				Start: lsp.Position{Line: 9, Character: 4},
				End:   lsp.Position{Line: 9, Character: 11},
			},
			Children: []lsp.Symbol{
				{
					Name: "Run",
					Kind: 6,
					Range: lsp.Range{
						Start: lsp.Position{Line: 11, Character: 1},
						End:   lsp.Position{Line: 11, Character: 4},
					},
					Children: []lsp.Symbol{
						{
							Name: "message",
							Kind: 13,
							SelectionRange: lsp.Range{
								Start: lsp.Position{Line: 12, Character: 7},
								End:   lsp.Position{Line: 12, Character: 14},
							},
						},
					},
				},
				{
					Name: "Config",
					Kind: 8,
					Range: lsp.Range{
						Start: lsp.Position{Line: 14, Character: 2},
						End:   lsp.Position{Line: 14, Character: 8},
					},
				},
			},
		},
	}

	got := flattenLSPSymbols(symbols, "workspace")
	want := []Symbol{
		{
			Name:      "Service",
			Kind:      SymbolKindClass,
			Container: "workspace",
			Range:     Range{StartLine: 10, StartColumn: 5, EndLine: 10, EndColumn: 12},
		},
		{
			Name:      "Run",
			Kind:      SymbolKindMethod,
			Container: "Service",
			Range:     Range{StartLine: 12, StartColumn: 2, EndLine: 12, EndColumn: 5},
		},
		{
			Name:      "message",
			Kind:      SymbolKindVariable,
			Container: "Run",
			Range:     Range{StartLine: 13, StartColumn: 8, EndLine: 13, EndColumn: 15},
		},
		{
			Name:      "Config",
			Kind:      SymbolKindField,
			Container: "Service",
			Range:     Range{StartLine: 15, StartColumn: 3, EndLine: 15, EndColumn: 9},
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("flattenLSPSymbols() = %#v, want %#v", got, want)
	}
}

func TestFlattenLSPSymbols_MultiLevel(t *testing.T) {
	t.Parallel()

	symbols := []lsp.Symbol{
		{
			Name: "Workspace",
			Kind: 2,
			Range: lsp.Range{
				Start: lsp.Position{Line: 0, Character: 0},
				End:   lsp.Position{Line: 0, Character: 9},
			},
			Children: []lsp.Symbol{
				{
					Name: "Widget",
					Kind: 23,
					Range: lsp.Range{
						Start: lsp.Position{Line: 1, Character: 0},
						End:   lsp.Position{Line: 1, Character: 6},
					},
					Children: []lsp.Symbol{
						{
							Name: "Render",
							Kind: 6,
							Range: lsp.Range{
								Start: lsp.Position{Line: 2, Character: 1},
								End:   lsp.Position{Line: 2, Character: 7},
							},
							Children: []lsp.Symbol{
								{
									Name: "theme",
									Kind: 13,
									Range: lsp.Range{
										Start: lsp.Position{Line: 3, Character: 2},
										End:   lsp.Position{Line: 3, Character: 7},
									},
									Children: []lsp.Symbol{
										{
											Name: "mode",
											Kind: 20,
											Range: lsp.Range{
												Start: lsp.Position{Line: 4, Character: 3},
												End:   lsp.Position{Line: 4, Character: 7},
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	got := flattenLSPSymbols(symbols, "")
	if len(got) != 5 {
		t.Fatalf("flattenLSPSymbols() len = %d, want 5", len(got))
	}

	containers := []struct {
		name      string
		container string
		kind      SymbolKind
	}{
		{name: "Workspace", container: "", kind: SymbolKindModule},
		{name: "Widget", container: "Workspace", kind: SymbolKindStruct},
		{name: "Render", container: "Widget", kind: SymbolKindMethod},
		{name: "theme", container: "Render", kind: SymbolKindVariable},
		{name: "mode", container: "theme", kind: SymbolKindField},
	}
	for i, want := range containers {
		if got[i].Name != want.name || got[i].Container != want.container || got[i].Kind != want.kind {
			t.Fatalf("symbol %d = %#v, want name=%q container=%q kind=%v", i, got[i], want.name, want.container, want.kind)
		}
	}
}

func TestExtractWithLSP_Fallback(t *testing.T) {
	t.Parallel()

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

func TestExtractWithLSP_NilMux(t *testing.T) {
	t.Parallel()

	src := []byte(`package sample

type Widget struct{}

func Build() Widget { return Widget{} }
`)

	want, err := ExtractSymbols("sample.go", src)
	if err != nil {
		t.Fatalf("ExtractSymbols() error = %v", err)
	}

	got, err := ExtractWithLSP(context.Background(), nil, "sample.go", src)
	if err != nil {
		t.Fatalf("ExtractWithLSP() error = %v", err)
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractWithLSP() = %#v, want %#v", got, want)
	}
}

func TestExtractWithLSP_UsesClient(t *testing.T) {
	ctx := context.Background()
	client, err := lsp.NewClient(ctx, t.TempDir(), []string{"env", "GO_WANT_HELPER_PROCESS=1", os.Args[0], "-test.run=TestLSPHelperProcess", "--"})
	if err != nil {
		t.Fatalf("NewClient() error = %v", err)
	}
	defer func() {
		_ = client.Close()
	}()

	mux := lsp.NewMultiplexer(t.TempDir())
	setMultiplexerClients(mux, map[lsp.ServerID]*lsp.Client{lsp.ServerPython: client})

	src := []byte("print(\"sample\")\n")
	got, err := ExtractWithLSP(ctx, mux, "sample.py", src)
	if err != nil {
		t.Fatalf("ExtractWithLSP() error = %v", err)
	}

	want := []Symbol{
		{
			Name:      "Widget",
			Kind:      SymbolKindClass,
			Language:  "python",
			Path:      "sample.py",
			Container: "",
			Range:     Range{StartLine: 2, StartColumn: 6, EndLine: 2, EndColumn: 12},
			Exported:  true,
		},
		{
			Name:      "Render",
			Kind:      SymbolKindMethod,
			Language:  "python",
			Path:      "sample.py",
			Container: "Widget",
			Range:     Range{StartLine: 4, StartColumn: 2, EndLine: 4, EndColumn: 8},
			Exported:  true,
		},
		{
			Name:      "_hiddenValue",
			Kind:      SymbolKindField,
			Language:  "python",
			Path:      "sample.py",
			Container: "Widget",
			Range:     Range{StartLine: 5, StartColumn: 4, EndLine: 5, EndColumn: 15},
			Exported:  false,
		},
	}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExtractWithLSP() = %#v, want %#v", got, want)
	}
}

func setMultiplexerClients(mux *lsp.Multiplexer, clients map[lsp.ServerID]*lsp.Client) {
	field := reflect.ValueOf(mux).Elem().FieldByName("clients")
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(clients))
}

func TestLSPHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	if err := runFakeLSPServer(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "fake lsp server error: %v\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}

func runFakeLSPServer(stdin io.Reader, stdout io.Writer) error {
	reader := bufio.NewReader(stdin)
	for {
		payload, err := readLSPMessage(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}

		var req struct {
			ID     any             `json:"id,omitempty"`
			Method string          `json:"method"`
			Params json.RawMessage `json:"params,omitempty"`
		}
		if err := json.Unmarshal(payload, &req); err != nil {
			return err
		}

		switch req.Method {
		case "initialize":
			if err := writeLSPMessage(stdout, map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  map[string]any{"capabilities": map[string]any{}},
			}); err != nil {
				return err
			}
		case "shutdown":
			return writeLSPMessage(stdout, map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  nil,
			})
		case "textDocument/documentSymbol":
			if err := writeLSPMessage(stdout, map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result":  fakeLSPSymbols(),
			}); err != nil {
				return err
			}
		}
	}
}

func fakeLSPSymbols() []map[string]any {
	return []map[string]any{
		{
			"name": "Widget",
			"kind": 5,
			"range": map[string]any{
				"start": map[string]any{"line": 1, "character": 0},
				"end":   map[string]any{"line": 1, "character": 12},
			},
			"selectionRange": map[string]any{
				"start": map[string]any{"line": 1, "character": 5},
				"end":   map[string]any{"line": 1, "character": 11},
			},
			"children": []map[string]any{
				{
					"name": "Render",
					"kind": 6,
					"range": map[string]any{
						"start": map[string]any{"line": 3, "character": 1},
						"end":   map[string]any{"line": 3, "character": 14},
					},
					"selectionRange": map[string]any{
						"start": map[string]any{"line": 3, "character": 1},
						"end":   map[string]any{"line": 3, "character": 7},
					},
				},
				{
					"name": "_hiddenValue",
					"kind": 20,
					"range": map[string]any{
						"start": map[string]any{"line": 4, "character": 3},
						"end":   map[string]any{"line": 4, "character": 14},
					},
					"selectionRange": map[string]any{
						"start": map[string]any{"line": 0, "character": 0},
						"end":   map[string]any{"line": 0, "character": 0},
					},
				},
			},
		},
	}
}

func readLSPMessage(reader *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		if !strings.HasPrefix(strings.ToLower(line), "content-length:") {
			continue
		}
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			return nil, io.ErrUnexpectedEOF
		}
		length, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, err
		}
		contentLength = length
	}
	if contentLength < 0 {
		return nil, io.ErrUnexpectedEOF
	}

	payload := make([]byte, contentLength)
	if _, err := io.ReadFull(reader, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func writeLSPMessage(writer io.Writer, message any) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		return err
	}
	_, err = writer.Write(payload)
	return err
}
