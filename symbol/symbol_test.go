package symbol

import "testing"

func TestIndexAddAndLookup(t *testing.T) {
	t.Parallel()
	idx := NewIndex()
	idx.Add(Symbol{
		Name: "Foo", Kind: KindFunction, Language: "go",
		Location:   Location{URI: "file:///main.go", StartLine: 10},
		IsExported: true, IsDefinition: true,
	})
	idx.Add(Symbol{
		Name: "Foo", Kind: KindVariable, Language: "go",
		Location:   Location{URI: "file:///other.go", StartLine: 5},
		IsExported: false, IsDefinition: false,
	})

	if idx.Count() != 2 {
		t.Errorf("Count() = %d, want 2", idx.Count())
	}

	byName := idx.LookupName("Foo")
	if len(byName) != 2 {
		t.Errorf("LookupName(Foo) = %d, want 2", len(byName))
	}

	defs := idx.Definitions("Foo")
	if len(defs) != 1 {
		t.Errorf("Definitions(Foo) = %d, want 1", len(defs))
	}

	byKind := idx.LookupKind(KindFunction)
	if len(byKind) != 1 {
		t.Errorf("LookupKind(Function) = %d, want 1", len(byKind))
	}

	byURI := idx.LookupURI("file:///main.go")
	if len(byURI) != 1 {
		t.Errorf("LookupURI = %d, want 1", len(byURI))
	}
}

func TestIndexSearch(t *testing.T) {
	t.Parallel()
	idx := NewIndex()
	exported := true
	idx.Add(Symbol{Name: "Handle", Kind: KindFunction, Language: "go", IsExported: true, IsDefinition: true})
	idx.Add(Symbol{Name: "handle", Kind: KindFunction, Language: "go", IsExported: false, IsDefinition: true})
	idx.Add(Symbol{Name: "Handle", Kind: KindMethod, Language: "go", IsExported: true, IsDefinition: true})

	results := idx.Search(Filter{Name: "Handle", IsExported: &exported})
	if len(results) != 2 {
		t.Errorf("expected 2 exported Handle, got %d", len(results))
	}
}

func TestIndexEmpty(t *testing.T) {
	t.Parallel()
	idx := NewIndex()
	if idx.Count() != 0 {
		t.Error("empty index should have count 0")
	}
	if idx.LookupName("x") != nil {
		t.Error("empty lookup should return nil")
	}
}
