package fuzzy

import "testing"

func TestMatchExactSubstring(t *testing.T) {
	t.Parallel()
	r := Match("hello world", "hello", Options{})
	if r.Start < 0 {
		t.Fatal("expected match")
	}
	if r.Score <= 0 {
		t.Error("expected positive score")
	}
}

func TestMatchCaseInsensitive(t *testing.T) {
	t.Parallel()
	r := Match("FooBar", "foobar", Options{CaseSensitive: false})
	if r.Start < 0 {
		t.Fatal("expected case-insensitive match")
	}
}

func TestMatchCaseSensitiveNoMatch(t *testing.T) {
	t.Parallel()
	r := Match("FooBar", "foobar", Options{CaseSensitive: true})
	if r.Start >= 0 {
		t.Fatal("expected no match in case-sensitive mode")
	}
}

func TestMatchNoMatch(t *testing.T) {
	t.Parallel()
	r := Match("hello", "xyz", Options{})
	if r.Start >= 0 {
		t.Fatal("expected no match")
	}
}

func TestMatchEmptyPattern(t *testing.T) {
	t.Parallel()
	r := Match("hello", "", Options{})
	if r.Score != 0 {
		t.Errorf("empty pattern should have score 0, got %d", r.Score)
	}
}

func TestMatchWithPositions(t *testing.T) {
	t.Parallel()
	r := Match("foobar", "fbr", Options{WithPositions: true})
	if r.Start < 0 {
		t.Fatal("expected match")
	}
	if len(r.Positions) != 3 {
		t.Errorf("expected 3 positions, got %d", len(r.Positions))
	}
}

func TestMatchV1Basic(t *testing.T) {
	t.Parallel()
	r := MatchV1([]rune("hello world"), []rune("hld"), Options{})
	if r.Start < 0 {
		t.Fatal("expected V1 match")
	}
}

func TestMatchV2BetterThanV1(t *testing.T) {
	t.Parallel()
	text := "foo_bar_baz_foobar"
	pattern := "fbar"
	v1 := MatchV1([]rune(text), []rune(pattern), Options{})
	v2 := MatchV2([]rune(text), []rune(pattern), Options{})
	if v1.Start < 0 || v2.Start < 0 {
		t.Fatal("expected both to match")
	}
	// V2 should find equal or better score
	if v2.Score < v1.Score {
		t.Errorf("V2 score %d < V1 score %d", v2.Score, v1.Score)
	}
}

func TestMatchWordBoundaryBonus(t *testing.T) {
	t.Parallel()
	// "fb" in "foo-bar" should score higher than in "foobar" due to boundary
	r1 := Match("foo-bar", "fb", Options{})
	r2 := Match("fxxbar", "fb", Options{})
	if r1.Start < 0 || r2.Start < 0 {
		t.Fatal("expected both to match")
	}
	if r1.Score <= r2.Score {
		t.Errorf("boundary match (%d) should score higher than non-boundary (%d)", r1.Score, r2.Score)
	}
}

func TestMatchCamelCaseBonus(t *testing.T) {
	t.Parallel()
	r := Match("FuzzyMatchV2", "FMV", Options{CaseSensitive: true, WithPositions: true})
	if r.Start < 0 {
		t.Fatal("expected camelCase match")
	}
	if r.Score <= 0 {
		t.Error("expected positive score for camelCase")
	}
}

func TestMatchConsecutiveBonus(t *testing.T) {
	t.Parallel()
	// Consecutive matches should score higher
	r1 := Match("foobar", "foo", Options{})
	r2 := Match("f_o_o_bar", "foo", Options{})
	if r1.Start < 0 || r2.Start < 0 {
		t.Fatal("expected both to match")
	}
	if r1.Score <= r2.Score {
		t.Errorf("consecutive (%d) should score higher than gapped (%d)", r1.Score, r2.Score)
	}
}

func TestClassifyChar(t *testing.T) {
	t.Parallel()
	tests := []struct {
		r    rune
		want charClass
	}{
		{'a', charLower},
		{'Z', charUpper},
		{'5', charNumber},
		{' ', charWhite},
		{'/', charDelimiter},
		{'_', charNonWord},
	}
	for _, tt := range tests {
		got := classifyChar(tt.r)
		if got != tt.want {
			t.Errorf("classifyChar(%q) = %d, want %d", tt.r, got, tt.want)
		}
	}
}
