package trigram

import (
	"errors"
	"testing"
)

func TestExtractSimpleContent(t *testing.T) {
	got := ExtractString("hello")
	want := []Trigram{{'e', 'l', 'l'}, {'h', 'e', 'l'}, {'l', 'l', 'o'}}
	assertTrigramsEqual(t, got, want)
}

func TestExtractDeduplicates(t *testing.T) {
	got := ExtractString("aaaaa")
	want := []Trigram{{'a', 'a', 'a'}}
	assertTrigramsEqual(t, got, want)
}

func TestExtractSkipsNewlinesAndNulls(t *testing.T) {
	got := Extract([]byte("ab\ncd\x00efg"))
	want := []Trigram{{'e', 'f', 'g'}}
	assertTrigramsEqual(t, got, want)
}

func TestBuildQueryPlanLiteralString(t *testing.T) {
	plan, err := BuildQueryPlan("hello")
	if err != nil {
		t.Fatalf("BuildQueryPlan() error = %v", err)
	}

	want := []Trigram{{'e', 'l', 'l'}, {'h', 'e', 'l'}, {'l', 'l', 'o'}}
	assertTrigramsEqual(t, plan.Trigrams, want)
	if !plan.Regex.MatchString("well hello there") {
		t.Fatalf("compiled regex did not match literal")
	}
}

func TestBuildQueryPlanSimpleRegex(t *testing.T) {
	plan, err := BuildQueryPlan("foo.*bar")
	if err != nil {
		t.Fatalf("BuildQueryPlan() error = %v", err)
	}

	want := []Trigram{{'b', 'a', 'r'}, {'f', 'o', 'o'}}
	assertTrigramsEqual(t, plan.Trigrams, want)
}

func TestBuildQueryPlanNoExtractableTrigrams(t *testing.T) {
	_, err := BuildQueryPlan("a.*b")
	if !errors.Is(err, ErrNoExtractableTrigrams) {
		t.Fatalf("BuildQueryPlan() error = %v, want %v", err, ErrNoExtractableTrigrams)
	}
}
func assertTrigramsEqual(t *testing.T, got, want []Trigram) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(got) = %d, want %d (got=%v, want=%v)", len(got), len(want), got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %v, want %v (got=%v, want=%v)", i, got[i], want[i], got, want)
		}
	}
}
