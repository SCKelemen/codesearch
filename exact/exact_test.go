package exact

import "testing"

func TestIndexSearch(t *testing.T) {
	t.Parallel()
	content := []byte("package main\n\nfunc hello() {\n\tprintln(\"hello world\")\n}\n")
	idx := NewIndex(content)

	matches := idx.SearchString("hello")
	if len(matches) < 2 {
		t.Fatalf("expected at least 2 matches, got %d", len(matches))
	}
	for _, m := range matches {
		if m.Line <= 0 {
			t.Errorf("expected positive line number, got %d", m.Line)
		}
	}
}

func TestIndexSearchNoMatch(t *testing.T) {
	t.Parallel()
	idx := NewIndex([]byte("hello world"))
	matches := idx.SearchString("xyz")
	if len(matches) != 0 {
		t.Errorf("expected no matches, got %d", len(matches))
	}
}

func TestIndexSearchEmpty(t *testing.T) {
	t.Parallel()
	idx := NewIndex([]byte("hello"))
	matches := idx.SearchString("")
	if matches != nil {
		t.Error("expected nil for empty pattern")
	}
}

func TestIndexCount(t *testing.T) {
	t.Parallel()
	idx := NewIndex([]byte("abcabcabc"))
	count := idx.Count([]byte("abc"))
	if count != 3 {
		t.Errorf("expected 3, got %d", count)
	}
}

func TestIndexSize(t *testing.T) {
	t.Parallel()
	data := []byte("hello world")
	idx := NewIndex(data)
	if idx.Size() != len(data) {
		t.Errorf("Size() = %d, want %d", idx.Size(), len(data))
	}
}

func TestIndexLineCount(t *testing.T) {
	t.Parallel()
	idx := NewIndex([]byte("line1\nline2\nline3"))
	if idx.LineCount() != 3 {
		t.Errorf("LineCount() = %d, want 3", idx.LineCount())
	}
}

func TestIndexLineMapping(t *testing.T) {
	t.Parallel()
	idx := NewIndex([]byte("aaa\nbbb\nccc"))
	matches := idx.SearchString("bbb")
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
	if matches[0].Line != 2 {
		t.Errorf("expected line 2, got %d", matches[0].Line)
	}
	if matches[0].Column != 0 {
		t.Errorf("expected column 0, got %d", matches[0].Column)
	}
}

func TestPhraseSearch(t *testing.T) {
	t.Parallel()
	idx := NewIndex([]byte("func TestFoo(t *testing.T) {\n\tt.Run(\"case\", func(t *testing.T) {})\n}"))
	ps := NewPhraseSearch(idx)
	matches := ps.Search([]byte("TestFoo"))
	if len(matches) == 0 {
		t.Fatal("expected phrase match")
	}
	if matches[0].Before == "" && matches[0].After == "" {
		t.Error("expected some context")
	}
}
