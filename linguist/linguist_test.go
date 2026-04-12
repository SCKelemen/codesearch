package linguist

import (
	"testing"
)

func TestLookupByExtension_Go(t *testing.T) {
	t.Parallel()
	lang := LookupByExtension(".go")
	if lang == nil {
		t.Fatal("expected Go language for .go extension")
	}
	if lang.Name != "Go" {
		t.Errorf("Name: got %q, want %q", lang.Name, "Go")
	}
}

func TestLookupByExtension_WithoutDot(t *testing.T) {
	t.Parallel()
	lang := LookupByExtension("go")
	if lang == nil {
		t.Fatal("expected Go language for 'go' extension (no dot)")
	}
	if lang.Name != "Go" {
		t.Errorf("Name: got %q, want %q", lang.Name, "Go")
	}
}

func TestLookupByExtension_CaseInsensitive(t *testing.T) {
	t.Parallel()
	lang := LookupByExtension(".GO")
	if lang == nil {
		t.Fatal("expected Go language for .GO extension")
	}
}

func TestLookupByExtension_TypeScript(t *testing.T) {
	t.Parallel()
	lang := LookupByExtension(".ts")
	if lang == nil {
		t.Fatal("expected TypeScript for .ts")
	}
}

func TestLookupByExtension_Unknown(t *testing.T) {
	t.Parallel()
	lang := LookupByExtension(".xyznonexistent")
	if lang != nil {
		t.Errorf("expected nil for unknown extension, got %v", lang)
	}
}

func TestLookupByExtension_Empty(t *testing.T) {
	t.Parallel()
	lang := LookupByExtension("")
	if lang != nil {
		t.Errorf("expected nil for empty extension, got %v", lang)
	}
}

func TestLookupByName_Go(t *testing.T) {
	t.Parallel()
	lang := LookupByName("Go")
	if lang == nil {
		t.Fatal("expected Go language by name")
	}
	if lang.Name != "Go" {
		t.Errorf("Name: got %q, want %q", lang.Name, "Go")
	}
}

func TestLookupByName_CaseInsensitive(t *testing.T) {
	t.Parallel()
	lang := LookupByName("go")
	if lang == nil {
		t.Fatal("expected Go language by lowercase name")
	}
}

func TestLookupByName_JavaScript(t *testing.T) {
	t.Parallel()
	lang := LookupByName("JavaScript")
	if lang == nil {
		t.Fatal("expected JavaScript by name")
	}
}

func TestLookupByName_Unknown(t *testing.T) {
	t.Parallel()
	lang := LookupByName("NotARealLanguage")
	if lang != nil {
		t.Errorf("expected nil for unknown name, got %v", lang)
	}
}

func TestLookupByName_Empty(t *testing.T) {
	t.Parallel()
	lang := LookupByName("")
	if lang != nil {
		t.Errorf("expected nil for empty name, got %v", lang)
	}
}

func TestLookupByName_Whitespace(t *testing.T) {
	t.Parallel()
	lang := LookupByName("  Go  ")
	if lang == nil {
		t.Fatal("expected Go language with whitespace padding")
	}
}

func TestLanguagesMapPopulated(t *testing.T) {
	t.Parallel()
	if len(Languages) == 0 {
		t.Fatal("Languages map is empty — generated data not loaded")
	}
}

func TestLanguageHasColor(t *testing.T) {
	t.Parallel()
	lang := LookupByName("Go")
	if lang == nil {
		t.Skip("Go not in Languages map")
	}
	if lang.Color == "" {
		t.Error("Go should have a color")
	}
}

func TestNormalizeExtension(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input string
		want  string
	}{
		{".go", ".go"},
		{"go", ".go"},
		{".GO", ".go"},
		{"  .ts  ", ".ts"},
		{"", ""},
		{"  ", ""},
	}

	for _, tt := range tests {
		got := normalizeExtension(tt.input)
		if got != tt.want {
			t.Errorf("normalizeExtension(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}
