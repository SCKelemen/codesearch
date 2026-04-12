package structural

import "testing"

func TestExtractSymbols_Go(t *testing.T) {
	t.Parallel()

	src := []byte(`package sample

type Pair[T any] struct {
	Value T
}

func NewPair[T any](value T) Pair[T] {
	return Pair[T]{Value: value}
}

func (p *Pair[T]) Get() T {
	return p.Value
}
`)

	symbols, err := ExtractSymbols("pair.go", src)
	if err != nil {
		t.Fatalf("ExtractSymbols() error = %v", err)
	}

	assertHasSymbol(t, symbols, Symbol{Name: "sample", Kind: SymbolKindPackage, Language: "go", Path: "pair.go"})
	assertHasSymbol(t, symbols, Symbol{Name: "Pair", Kind: SymbolKindStruct, Language: "go", Path: "pair.go", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "Value", Kind: SymbolKindField, Language: "go", Path: "pair.go", Container: "Pair", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "NewPair", Kind: SymbolKindFunction, Language: "go", Path: "pair.go", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "Get", Kind: SymbolKindMethod, Language: "go", Path: "pair.go", Container: "Pair", Exported: true})
}

func TestExtractSymbols_TypeScript(t *testing.T) {
	t.Parallel()

	src := []byte("/* top-level comment */\n" +
		"export class Widget<T> {\n" +
		"\tpublic title: string;\n" +
		"\tstatic version = 1;\n" +
		"\tasync render<U>(value: U): Promise<U> {\n" +
		"\t\treturn value\n" +
		"\t}\n" +
		"}\n\n" +
		"const template = `class Hidden {}`\n" +
		"export const build = async <T,>(value: T) => value\n" +
		"export default Widget\n\n" +
		"declare namespace App {}\n" +
		"declare module \"widget-runtime\" {}\n")

	symbols, err := ExtractSymbols("widget.ts", src)
	if err != nil {
		t.Fatalf("ExtractSymbols() error = %v", err)
	}

	assertHasSymbol(t, symbols, Symbol{Name: "Widget", Kind: SymbolKindClass, Language: "typescript", Path: "widget.ts", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "title", Kind: SymbolKindField, Language: "typescript", Path: "widget.ts", Container: "Widget", Exported: false})
	assertHasSymbol(t, symbols, Symbol{Name: "render", Kind: SymbolKindMethod, Language: "typescript", Path: "widget.ts", Container: "Widget", Exported: false})
	assertHasSymbol(t, symbols, Symbol{Name: "build", Kind: SymbolKindFunction, Language: "typescript", Path: "widget.ts", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "App", Kind: SymbolKindModule, Language: "typescript", Path: "widget.ts", Exported: false})
	assertHasSymbol(t, symbols, Symbol{Name: "widget-runtime", Kind: SymbolKindModule, Language: "typescript", Path: "widget.ts", Exported: false})
}

func TestExtractSymbols_JavaScript(t *testing.T) {
	t.Parallel()

	src := []byte(`class Store {
	state = {}
	async load() {
		return true
	}
}

const Widget = (props) => props.title
module.exports = Widget
exports.loadWidget = async () => true
`)

	symbols, err := ExtractSymbols("widget.js", src)
	if err != nil {
		t.Fatalf("ExtractSymbols() error = %v", err)
	}

	assertHasSymbol(t, symbols, Symbol{Name: "Store", Kind: SymbolKindClass, Language: "javascript", Path: "widget.js", Exported: false})
	assertHasSymbol(t, symbols, Symbol{Name: "state", Kind: SymbolKindField, Language: "javascript", Path: "widget.js", Container: "Store", Exported: false})
	assertHasSymbol(t, symbols, Symbol{Name: "load", Kind: SymbolKindMethod, Language: "javascript", Path: "widget.js", Container: "Store", Exported: false})
	assertHasSymbol(t, symbols, Symbol{Name: "Widget", Kind: SymbolKindFunction, Language: "javascript", Path: "widget.js", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "loadWidget", Kind: SymbolKindVariable, Language: "javascript", Path: "widget.js", Exported: true})
}

func TestExtractSymbols_Python(t *testing.T) {
	t.Parallel()

	src := []byte(`class Widget:
    pass

def build():
    return Widget()

MAX_COUNT = 10
_hidden = 1
`)

	symbols, err := ExtractSymbols("app.py", src)
	if err != nil {
		t.Fatalf("ExtractSymbols() error = %v", err)
	}

	assertHasSymbol(t, symbols, Symbol{Name: "Widget", Kind: SymbolKindClass, Language: "python", Path: "app.py", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "build", Kind: SymbolKindFunction, Language: "python", Path: "app.py", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "MAX_COUNT", Kind: SymbolKindConstant, Language: "python", Path: "app.py", Exported: true})
}

func TestExtractSymbols_Rust(t *testing.T) {
	t.Parallel()

	src := []byte(`pub mod widgets;

pub struct Widget<T>(pub T);

pub enum State {
    Ready,
    Busy(String),
}

pub trait Renderable {
    const LIMIT: usize;
    type Output;
    fn render(&self) -> Self::Output;
}

impl<T> Widget<T> {
    pub const fn new(value: T) -> Self {
        Self(value)
    }

    async fn load(&self) {}

    type Alias = T;
}

impl Renderable for Widget<String> {
    const LIMIT: usize = 1;
    type Output = ();
    fn render(&self) -> Self::Output {}
}

pub const RAW: &str = r#"/* not a comment */"#;
static BYTES: &[u8] = br#"bytes"#;

macro_rules! widget_map {
    ($value:expr) => {{ $value }};
}
`)

	symbols, err := ExtractSymbols("lib.rs", src)
	if err != nil {
		t.Fatalf("ExtractSymbols() error = %v", err)
	}

	assertHasSymbol(t, symbols, Symbol{Name: "widgets", Kind: SymbolKindModule, Language: "rust", Path: "lib.rs", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "Widget", Kind: SymbolKindStruct, Language: "rust", Path: "lib.rs", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "State", Kind: SymbolKindEnum, Language: "rust", Path: "lib.rs", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "Ready", Kind: SymbolKindEnumMember, Language: "rust", Path: "lib.rs", Container: "State", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "Renderable", Kind: SymbolKindTrait, Language: "rust", Path: "lib.rs", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "LIMIT", Kind: SymbolKindConstant, Language: "rust", Path: "lib.rs", Container: "Renderable", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "Output", Kind: SymbolKindType, Language: "rust", Path: "lib.rs", Container: "Renderable", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "render", Kind: SymbolKindMethod, Language: "rust", Path: "lib.rs", Container: "Widget", Exported: false})
	assertHasSymbol(t, symbols, Symbol{Name: "new", Kind: SymbolKindMethod, Language: "rust", Path: "lib.rs", Container: "Widget", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "load", Kind: SymbolKindMethod, Language: "rust", Path: "lib.rs", Container: "Widget", Exported: false})
	assertHasSymbol(t, symbols, Symbol{Name: "Alias", Kind: SymbolKindType, Language: "rust", Path: "lib.rs", Container: "Widget", Exported: false})
	assertHasSymbol(t, symbols, Symbol{Name: "RAW", Kind: SymbolKindConstant, Language: "rust", Path: "lib.rs", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "BYTES", Kind: SymbolKindVariable, Language: "rust", Path: "lib.rs", Exported: false})
	assertHasSymbol(t, symbols, Symbol{Name: "widget_map", Kind: SymbolKindFunction, Language: "rust", Path: "lib.rs", Exported: false})
}

func TestExtractSymbols_SQL(t *testing.T) {
	t.Parallel()

	src := []byte("-- line comment\n" +
		"CREATE TABLE \"sales\".\"orders\" (\n" +
		"    \"order_id\" BIGINT PRIMARY KEY,\n" +
		"    [customer_id] BIGINT NOT NULL,\n" +
		"    `status` TEXT,\n" +
		"    CONSTRAINT orders_pk PRIMARY KEY (\"order_id\"),\n" +
		"    CHECK (\"order_id\" > 0)\n" +
		");\n\n" +
		"/* block comment */\n" +
		"ALTER TABLE IF EXISTS \"sales\".\"orders\" ADD COLUMN created_at TIMESTAMP;\n" +
		"# hash comment\n" +
		"CREATE OR REPLACE FUNCTION [sales].[refresh_orders]() RETURNS INT AS $$ SELECT 1; $$;\n" +
		"CREATE UNIQUE INDEX IF NOT EXISTS `idx_orders_status` ON \"sales\".\"orders\" (\"status\");\n" +
		"CREATE OR REPLACE VIEW \"sales\".\"active_orders\" AS SELECT * FROM \"sales\".\"orders\";\n" +
		"CREATE TYPE [sales].[order_state] AS ENUM ('open', 'closed');\n" +
		"CREATE TRIGGER sync_orders BEFORE INSERT ON \"sales\".\"orders\" FOR EACH ROW EXECUTE PROCEDURE sync();\n")

	symbols, err := ExtractSymbols("schema.sql", src)
	if err != nil {
		t.Fatalf("ExtractSymbols() error = %v", err)
	}

	assertHasSymbol(t, symbols, Symbol{Name: "orders", Kind: SymbolKindStruct, Language: "sql", Path: "schema.sql", Container: "sales"})
	assertHasSymbol(t, symbols, Symbol{Name: "order_id", Kind: SymbolKindField, Language: "sql", Path: "schema.sql", Container: "orders"})
	assertHasSymbol(t, symbols, Symbol{Name: "customer_id", Kind: SymbolKindField, Language: "sql", Path: "schema.sql", Container: "orders"})
	assertHasSymbol(t, symbols, Symbol{Name: "status", Kind: SymbolKindField, Language: "sql", Path: "schema.sql", Container: "orders"})
	assertHasSymbol(t, symbols, Symbol{Name: "refresh_orders", Kind: SymbolKindFunction, Language: "sql", Path: "schema.sql", Container: "sales"})
	assertHasSymbol(t, symbols, Symbol{Name: "idx_orders_status", Kind: SymbolKindType, Language: "sql", Path: "schema.sql"})
	assertHasSymbol(t, symbols, Symbol{Name: "active_orders", Kind: SymbolKindType, Language: "sql", Path: "schema.sql", Container: "sales"})
	assertHasSymbol(t, symbols, Symbol{Name: "order_state", Kind: SymbolKindType, Language: "sql", Path: "schema.sql", Container: "sales"})
	assertHasSymbol(t, symbols, Symbol{Name: "sync_orders", Kind: SymbolKindFunction, Language: "sql", Path: "schema.sql"})
}

func TestExtractSymbols_Unknown(t *testing.T) {
	t.Parallel()

	symbols, err := ExtractSymbols("README.unknown", []byte("hello"))
	if err != nil {
		t.Fatalf("ExtractSymbols() error = %v", err)
	}
	if len(symbols) != 0 {
		t.Fatalf("ExtractSymbols() len = %d, want 0", len(symbols))
	}
}
