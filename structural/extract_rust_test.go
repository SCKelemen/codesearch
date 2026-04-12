package structural

import "testing"

func TestExtractRustSymbolsBasicItems(t *testing.T) {
	t.Parallel()

	src := []byte(`
#[derive(Debug)]
pub(crate) struct Widget<'a, T> {
    pub id: &'a str,
    value: T,
}

#[cfg(test)]
pub enum State<T> {
    Ready,
    Busy(T),
}

pub unsafe trait Renderable<'a, T> {
    type Output;
    fn render(&self, value: &'a T) -> Self::Output;
}

#[test]
pub fn build<'a, T>(input: &'a T) -> Widget<'a, T> {
    todo!()
}
`)

	symbols, err := ExtractRustSymbols("lib.rs", src)
	if err != nil {
		t.Fatalf("ExtractRustSymbols() error = %v", err)
	}

	wants := []Symbol{
		{Name: "Widget", Kind: SymbolKindStruct, Language: "rust", Path: "lib.rs", Exported: true},
		{Name: "id", Kind: SymbolKindField, Language: "rust", Path: "lib.rs", Container: "Widget", Exported: true},
		{Name: "value", Kind: SymbolKindField, Language: "rust", Path: "lib.rs", Container: "Widget", Exported: false},
		{Name: "State", Kind: SymbolKindEnum, Language: "rust", Path: "lib.rs", Exported: true},
		{Name: "Ready", Kind: SymbolKindEnumMember, Language: "rust", Path: "lib.rs", Container: "State", Exported: true},
		{Name: "Busy", Kind: SymbolKindEnumMember, Language: "rust", Path: "lib.rs", Container: "State", Exported: true},
		{Name: "Renderable", Kind: SymbolKindTrait, Language: "rust", Path: "lib.rs", Exported: true},
		{Name: "Output", Kind: SymbolKindType, Language: "rust", Path: "lib.rs", Container: "Renderable", Exported: true},
		{Name: "render", Kind: SymbolKindMethod, Language: "rust", Path: "lib.rs", Container: "Renderable", Exported: true},
		{Name: "build", Kind: SymbolKindFunction, Language: "rust", Path: "lib.rs", Exported: true},
	}

	for _, want := range wants {
		assertHasSymbol(t, symbols, want)
	}
}

func TestExtractRustSymbolsImplsAndAssociatedItems(t *testing.T) {
	t.Parallel()

	src := []byte(`
pub struct Widget<T> {
    value: T,
}

impl<T> Widget<T> {
    pub const fn new(value: T) -> Self {
        Self { value }
    }

    async fn load(&self) {}

    type Alias = T;
}

impl<T> Renderable for Widget<T> {
    unsafe fn render(&self) {}
}
`)

	symbols, err := ExtractRustSymbols("impls.rs", src)
	if err != nil {
		t.Fatalf("ExtractRustSymbols() error = %v", err)
	}

	wants := []Symbol{
		{Name: "Widget", Kind: SymbolKindStruct, Language: "rust", Path: "impls.rs", Exported: true},
		{Name: "new", Kind: SymbolKindMethod, Language: "rust", Path: "impls.rs", Container: "Widget", Exported: true},
		{Name: "load", Kind: SymbolKindMethod, Language: "rust", Path: "impls.rs", Container: "Widget", Exported: false},
		{Name: "Alias", Kind: SymbolKindType, Language: "rust", Path: "impls.rs", Container: "Widget", Exported: false},
		{Name: "render", Kind: SymbolKindMethod, Language: "rust", Path: "impls.rs", Container: "Widget", Exported: false},
	}

	for _, want := range wants {
		assertHasSymbol(t, symbols, want)
	}
}

func TestExtractRustSymbolsModulesMacrosStaticsAndAliases(t *testing.T) {
	t.Parallel()

	src := []byte(`
pub mod widgets;
mod internal {
    pub struct Hidden;
}

pub type WidgetId<'a, T> = (&'a str, T);
pub const MAX_SIZE: usize = 10;
static CACHE: usize = 1;
static mut GLOBAL: usize = 2;
macro_rules! widget_map {
    () => {};
}
`)

	symbols, err := ExtractRustSymbols("items.rs", src)
	if err != nil {
		t.Fatalf("ExtractRustSymbols() error = %v", err)
	}

	wants := []Symbol{
		{Name: "widgets", Kind: SymbolKindModule, Language: "rust", Path: "items.rs", Exported: true},
		{Name: "internal", Kind: SymbolKindModule, Language: "rust", Path: "items.rs", Exported: false},
		{Name: "WidgetId", Kind: SymbolKindType, Language: "rust", Path: "items.rs", Exported: true},
		{Name: "MAX_SIZE", Kind: SymbolKindConstant, Language: "rust", Path: "items.rs", Exported: true},
		{Name: "CACHE", Kind: SymbolKindVariable, Language: "rust", Path: "items.rs", Exported: false},
		{Name: "GLOBAL", Kind: SymbolKindVariable, Language: "rust", Path: "items.rs", Exported: false},
		{Name: "widget_map", Kind: SymbolKindFunction, Language: "rust", Path: "items.rs", Exported: false},
	}

	for _, want := range wants {
		assertHasSymbol(t, symbols, want)
	}
}

func TestExtractRustSymbolsViaGenericExtractor(t *testing.T) {
	t.Parallel()

	symbols, err := ExtractSymbolsGeneric("lib.rs", []byte(`pub mod widgets; pub fn build() {}`))
	if err != nil {
		t.Fatalf("ExtractSymbolsGeneric() error = %v", err)
	}

	assertHasSymbol(t, symbols, Symbol{Name: "widgets", Kind: SymbolKindModule, Language: "rust", Path: "lib.rs", Exported: true})
	assertHasSymbol(t, symbols, Symbol{Name: "build", Kind: SymbolKindFunction, Language: "rust", Path: "lib.rs", Exported: true})
}
