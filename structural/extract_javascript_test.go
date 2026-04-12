package structural

import "testing"

func TestExtractJavaScriptSymbols(t *testing.T) {
	t.Parallel()

	src := []byte(`export default function App() {
	return <div />
}

class Store extends BaseStore {
	state = {}
	async load() {
		return true
	}
	render() {
		return null
	}
}

const Widget = (props) => <section>{props.title}</section>
const useWidget = () => true
module.exports = Widget
exports.loadWidget = async () => true
`)

	symbols, err := ExtractJavaScriptSymbols("widget.jsx", src)
	if err != nil {
		t.Fatalf("ExtractJavaScriptSymbols() error = %v", err)
	}

	wants := []Symbol{
		{Name: "App", Kind: SymbolKindFunction, Language: "javascript", Path: "widget.jsx", Exported: true},
		{Name: "Store", Kind: SymbolKindClass, Language: "javascript", Path: "widget.jsx", Exported: false},
		{Name: "state", Kind: SymbolKindField, Language: "javascript", Path: "widget.jsx", Container: "Store", Exported: false},
		{Name: "load", Kind: SymbolKindMethod, Language: "javascript", Path: "widget.jsx", Container: "Store", Exported: false},
		{Name: "render", Kind: SymbolKindMethod, Language: "javascript", Path: "widget.jsx", Container: "Store", Exported: false},
		{Name: "Widget", Kind: SymbolKindFunction, Language: "javascript", Path: "widget.jsx", Exported: true},
		{Name: "useWidget", Kind: SymbolKindFunction, Language: "javascript", Path: "widget.jsx", Exported: false},
		{Name: "loadWidget", Kind: SymbolKindVariable, Language: "javascript", Path: "widget.jsx", Exported: true},
	}

	for _, want := range wants {
		assertHasSymbol(t, symbols, want)
	}
}

func TestExtractJavaScriptSymbolsViaGenericExtractor(t *testing.T) {
	t.Parallel()

	symbols, err := ExtractSymbolsGeneric("widget.js", []byte(`const Widget = () => null
module.exports = Widget
`))
	if err != nil {
		t.Fatalf("ExtractSymbolsGeneric() error = %v", err)
	}

	assertHasSymbol(t, symbols, Symbol{Name: "Widget", Kind: SymbolKindFunction, Language: "javascript", Path: "widget.js", Exported: true})
}
