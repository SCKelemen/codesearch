package structural

import "testing"

func TestSymbolIndexSearch(t *testing.T) {
	t.Parallel()

	idx := &SymbolIndex{}
	idx.Add(Symbol{Name: "User", Kind: SymbolKindStruct, Language: "go", Path: "pkg/models/user.go", Exported: true})
	idx.Add(Symbol{Name: "userID", Kind: SymbolKindVariable, Language: "go", Path: "pkg/models/user.go", Exported: false})
	idx.Add(Symbol{Name: "User", Kind: SymbolKindClass, Language: "typescript", Path: "web/models/user.ts", Exported: true})

	results := idx.Search(SymbolQuery{Name: "User", Path: "pkg/*/*.go"})
	if len(results) != 1 {
		t.Fatalf("Search() returned %d results, want 1", len(results))
	}
	if results[0].Language != "go" {
		t.Fatalf("Search() returned language %q, want go", results[0].Language)
	}

	exported := true
	results = idx.Search(SymbolQuery{Kind: SymbolKindClass, Exported: &exported})
	if len(results) != 1 || results[0].Path != "web/models/user.ts" {
		t.Fatalf("Search() exported class = %#v, want TypeScript class", results)
	}

	results = idx.Search(SymbolQuery{Path: "["})
	if len(results) != 0 {
		t.Fatalf("Search() with invalid glob returned %d results, want 0", len(results))
	}
}

func TestExtractGoSymbols(t *testing.T) {
	t.Parallel()

	src := []byte(`package sample

type Person struct {
	Name string
	age  int
}

type Greeter interface {
	Greet() error
}

type ID int

const Version = "1.0"
var counter int

func NewPerson() Person { return Person{} }

func (p *Person) Greet() string { return p.Name }
`)

	symbols, err := ExtractGoSymbols("sample.go", src)
	if err != nil {
		t.Fatalf("ExtractGoSymbols() error = %v", err)
	}

	assertHasSymbol(t, symbols, Symbol{Name: "sample", Kind: SymbolKindPackage, Language: "go", Path: "sample.go"})
	assertHasSymbol(t, symbols, Symbol{Name: "Person", Kind: SymbolKindStruct, Language: "go", Path: "sample.go", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "Name", Kind: SymbolKindField, Language: "go", Path: "sample.go", Container: "Person", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "age", Kind: SymbolKindField, Language: "go", Path: "sample.go", Container: "Person", Exported: false})
	assertHasSymbol(t, symbols, Symbol{Name: "Greeter", Kind: SymbolKindInterface, Language: "go", Path: "sample.go", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "Greet", Kind: SymbolKindMethod, Language: "go", Path: "sample.go", Container: "Greeter", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "ID", Kind: SymbolKindType, Language: "go", Path: "sample.go", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "Version", Kind: SymbolKindConstant, Language: "go", Path: "sample.go", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "counter", Kind: SymbolKindVariable, Language: "go", Path: "sample.go", Exported: false})
	assertHasSymbol(t, symbols, Symbol{Name: "NewPerson", Kind: SymbolKindFunction, Language: "go", Path: "sample.go", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "Greet", Kind: SymbolKindMethod, Language: "go", Path: "sample.go", Container: "Person", Exported: true})

	person := findSymbol(t, symbols, "Person", SymbolKindStruct, "")
	if person.Range.StartLine == 0 || person.Range.StartColumn == 0 {
		t.Fatalf("Person range = %#v, want non-zero coordinates", person.Range)
	}
}

func TestExtractSymbolsGeneric(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		path  string
		src   string
		wants []Symbol
	}{
		{
			name: "python",
			path: "app.py",
			src:  "class Widget:\n    pass\n\ndef build():\n    return Widget()\n\nMAX_COUNT = 10\n",
			wants: []Symbol{
				{Name: "Widget", Kind: SymbolKindClass, Language: "python", Path: "app.py", Exported: true},
				{Name: "build", Kind: SymbolKindFunction, Language: "python", Path: "app.py", Exported: true},
				{Name: "MAX_COUNT", Kind: SymbolKindConstant, Language: "python", Path: "app.py", Exported: true},
			},
		},
		{
			name: "typescript",
			path: "widget.ts",
			src:  "export class Widget {}\nexport interface Named {}\nexport enum State {}\nexport async function build() {}\nexport const VERSION = '1';\n",
			wants: []Symbol{
				{Name: "Widget", Kind: SymbolKindClass, Language: "typescript", Path: "widget.ts", Exported: true},
				{Name: "Named", Kind: SymbolKindInterface, Language: "typescript", Path: "widget.ts", Exported: true},
				{Name: "State", Kind: SymbolKindEnum, Language: "typescript", Path: "widget.ts", Exported: true},
				{Name: "build", Kind: SymbolKindFunction, Language: "typescript", Path: "widget.ts", Exported: true},
				{Name: "VERSION", Kind: SymbolKindConstant, Language: "typescript", Path: "widget.ts", Exported: true},
			},
		},
		{
			name: "javascript",
			path: "widget.js",
			src:  "export class Widget {}\nexport function build() {}\nexport const VERSION = '1';\n",
			wants: []Symbol{
				{Name: "Widget", Kind: SymbolKindClass, Language: "javascript", Path: "widget.js", Exported: true},
				{Name: "build", Kind: SymbolKindFunction, Language: "javascript", Path: "widget.js", Exported: true},
				{Name: "VERSION", Kind: SymbolKindConstant, Language: "javascript", Path: "widget.js", Exported: true},
			},
		},
		{
			name: "java",
			path: "Widget.java",
			src:  "public class Widget {}\npublic interface Named {}\npublic enum State {}\npublic class Tools {\n    public static final int MAX_SIZE = 10;\n    public String build() { return \"x\"; }\n}\n",
			wants: []Symbol{
				{Name: "Widget", Kind: SymbolKindClass, Language: "java", Path: "Widget.java", Exported: true},
				{Name: "Named", Kind: SymbolKindInterface, Language: "java", Path: "Widget.java", Exported: true},
				{Name: "State", Kind: SymbolKindEnum, Language: "java", Path: "Widget.java", Exported: true},
				{Name: "MAX_SIZE", Kind: SymbolKindConstant, Language: "java", Path: "Widget.java", Exported: true},
				{Name: "build", Kind: SymbolKindMethod, Language: "java", Path: "Widget.java", Exported: false},
			},
		},
		{
			name: "rust",
			path: "lib.rs",
			src:  "pub mod widgets;\npub struct Widget;\npub enum State { Ready }\npub trait Renderable {}\npub fn build() {}\npub const MAX_SIZE: usize = 10;\n",
			wants: []Symbol{
				{Name: "widgets", Kind: SymbolKindModule, Language: "rust", Path: "lib.rs", Exported: false},
				{Name: "Widget", Kind: SymbolKindStruct, Language: "rust", Path: "lib.rs", Exported: true},
				{Name: "State", Kind: SymbolKindEnum, Language: "rust", Path: "lib.rs", Exported: true},
				{Name: "Renderable", Kind: SymbolKindTrait, Language: "rust", Path: "lib.rs", Exported: true},
				{Name: "build", Kind: SymbolKindFunction, Language: "rust", Path: "lib.rs", Exported: false},
				{Name: "MAX_SIZE", Kind: SymbolKindConstant, Language: "rust", Path: "lib.rs", Exported: true},
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			symbols, err := ExtractSymbolsGeneric(tt.path, []byte(tt.src))
			if err != nil {
				t.Fatalf("ExtractSymbolsGeneric() error = %v", err)
			}
			for _, want := range tt.wants {
				assertHasSymbol(t, symbols, want)
			}
			for _, symbol := range symbols {
				if symbol.Range.StartLine == 0 || symbol.Range.StartColumn == 0 {
					t.Fatalf("symbol %q has invalid range %#v", symbol.Name, symbol.Range)
				}
			}
		})
	}
}

func assertHasSymbol(t *testing.T, symbols []Symbol, want Symbol) {
	t.Helper()
	for _, symbol := range symbols {
		if symbol.Name == want.Name &&
			symbol.Kind == want.Kind &&
			symbol.Language == want.Language &&
			symbol.Path == want.Path &&
			symbol.Container == want.Container &&
			symbol.Exported == want.Exported {
			return
		}
	}
	t.Fatalf("missing symbol: %#v\nall symbols: %#v", want, symbols)
}

func findSymbol(t *testing.T, symbols []Symbol, name string, kind SymbolKind, container string) Symbol {
	t.Helper()
	for _, symbol := range symbols {
		if symbol.Name == name && symbol.Kind == kind && symbol.Container == container {
			return symbol
		}
	}
	t.Fatalf("symbol %q of kind %d in container %q not found", name, kind, container)
	return Symbol{}
}
