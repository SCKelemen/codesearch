package structural

import "testing"

func TestExtractTypeScriptSymbols(t *testing.T) {
	t.Parallel()

	src := []byte(`@Component()
export abstract class Widget<T> extends BaseWidget implements Renderable<T> {
	public title: string;
	private readonly cache: Map<string, T>;
	protected async renderItem<V>(item: V): Promise<V> {
		return item
	}
	static create<U>(value: U): Widget<U> {
		return value as unknown as Widget<U>
	}
}

@Injectable()
export class WidgetService {}

export interface Repository<T> {
	find(id: string): T
}

export type WidgetProps<T> = {
	value: T
}

export const Status = "ready"
export const WidgetView: React.FC<WidgetProps<string>> = ({ value }) => <div>{value}</div>
export const useWidget = <T,>(value: T) => value
export const ForwardedWidget = React.forwardRef(function ForwardedWidgetInner() { return null })

export default async function createWidget<T>(input: T): Promise<T> {
	return input
}

declare namespace App {}
declare module "widget-runtime" {}
`)

	symbols, err := ExtractTypeScriptSymbols("widget.tsx", src)
	if err != nil {
		t.Fatalf("ExtractTypeScriptSymbols() error = %v", err)
	}

	wants := []Symbol{
		{Name: "Widget", Kind: SymbolKindClass, Language: "typescript", Path: "widget.tsx", Exported: true},
		{Name: "title", Kind: SymbolKindField, Language: "typescript", Path: "widget.tsx", Container: "Widget", Exported: false},
		{Name: "cache", Kind: SymbolKindField, Language: "typescript", Path: "widget.tsx", Container: "Widget", Exported: false},
		{Name: "renderItem", Kind: SymbolKindMethod, Language: "typescript", Path: "widget.tsx", Container: "Widget", Exported: false},
		{Name: "create", Kind: SymbolKindMethod, Language: "typescript", Path: "widget.tsx", Container: "Widget", Exported: false},
		{Name: "WidgetService", Kind: SymbolKindClass, Language: "typescript", Path: "widget.tsx", Exported: true},
		{Name: "Repository", Kind: SymbolKindInterface, Language: "typescript", Path: "widget.tsx", Exported: true},
		{Name: "WidgetProps", Kind: SymbolKindType, Language: "typescript", Path: "widget.tsx", Exported: true},
		{Name: "WidgetView", Kind: SymbolKindFunction, Language: "typescript", Path: "widget.tsx", Exported: true},
		{Name: "useWidget", Kind: SymbolKindFunction, Language: "typescript", Path: "widget.tsx", Exported: true},
		{Name: "ForwardedWidget", Kind: SymbolKindFunction, Language: "typescript", Path: "widget.tsx", Exported: true},
		{Name: "createWidget", Kind: SymbolKindFunction, Language: "typescript", Path: "widget.tsx", Exported: true},
		{Name: "Status", Kind: SymbolKindConstant, Language: "typescript", Path: "widget.tsx", Exported: true},
		{Name: "App", Kind: SymbolKindModule, Language: "typescript", Path: "widget.tsx", Exported: false},
		{Name: "widget-runtime", Kind: SymbolKindModule, Language: "typescript", Path: "widget.tsx", Exported: false},
	}

	for _, want := range wants {
		assertHasSymbol(t, symbols, want)
	}
}

func TestExtractTypeScriptSymbolsViaGenericExtractor(t *testing.T) {
	t.Parallel()

	symbols, err := ExtractSymbolsGeneric("hooks.ts", []byte(`export function useFeature() { return true }`))
	if err != nil {
		t.Fatalf("ExtractSymbolsGeneric() error = %v", err)
	}

	assertHasSymbol(t, symbols, Symbol{Name: "useFeature", Kind: SymbolKindFunction, Language: "typescript", Path: "hooks.ts", Exported: true})
}
