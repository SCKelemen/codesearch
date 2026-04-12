package trigram

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp/syntax"
	"testing"
)

func TestExtractSimpleContent(t *testing.T) {
	t.Parallel()
	got := ExtractString("hello")
	want := []Trigram{{'e', 'l', 'l'}, {'h', 'e', 'l'}, {'l', 'l', 'o'}}
	assertTrigramsEqual(t, got, want)
}

func TestExtractDeduplicates(t *testing.T) {
	t.Parallel()
	got := ExtractString("aaaaa")
	want := []Trigram{{'a', 'a', 'a'}}
	assertTrigramsEqual(t, got, want)
}

func TestExtractSkipsNewlinesAndNulls(t *testing.T) {
	t.Parallel()
	got := Extract([]byte("ab\ncd\x00efg"))
	want := []Trigram{{'e', 'f', 'g'}}
	assertTrigramsEqual(t, got, want)
}

func TestExtractShortInput(t *testing.T) {
	t.Parallel()
	if got := Extract(nil); got != nil {
		t.Errorf("nil input: got %v", got)
	}
	if got := Extract([]byte("")); got != nil {
		t.Errorf("empty input: got %v", got)
	}
	if got := Extract([]byte("ab")); got != nil {
		t.Errorf("2 bytes: got %v", got)
	}
}

func TestExtractExactlyThreeBytes(t *testing.T) {
	t.Parallel()
	got := ExtractString("abc")
	if len(got) != 1 || got[0] != (Trigram{'a', 'b', 'c'}) {
		t.Errorf("got %v, want [{abc}]", got)
	}
}

func TestTrigramString(t *testing.T) {
	t.Parallel()
	tri := Trigram{'a', 'b', 'c'}
	if tri.String() != "abc" {
		t.Errorf("String() = %q, want %q", tri.String(), "abc")
	}
}

func TestBuildQueryPlanLiteralString(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
	plan, err := BuildQueryPlan("foo.*bar")
	if err != nil {
		t.Fatalf("BuildQueryPlan() error = %v", err)
	}
	want := []Trigram{{'b', 'a', 'r'}, {'f', 'o', 'o'}}
	assertTrigramsEqual(t, plan.Trigrams, want)
}

func TestBuildQueryPlanNoExtractableTrigrams(t *testing.T) {
	t.Parallel()
	_, err := BuildQueryPlan("a.*b")
	if !errors.Is(err, ErrNoExtractableTrigrams) {
		t.Fatalf("BuildQueryPlan() error = %v, want %v", err, ErrNoExtractableTrigrams)
	}
}

func TestBuildQueryPlanInvalidRegex(t *testing.T) {
	t.Parallel()
	_, err := BuildQueryPlan("[invalid")
	if err == nil {
		t.Fatal("expected error for invalid regex")
	}
}

func TestBuildQueryPlanCapture(t *testing.T) {
	t.Parallel()
	plan, err := BuildQueryPlan("(hello)")
	if err != nil {
		t.Fatalf("BuildQueryPlan() error = %v", err)
	}
	if len(plan.Trigrams) == 0 {
		t.Fatal("expected trigrams from captured literal")
	}
}

func TestBuildQueryPlanPlus(t *testing.T) {
	t.Parallel()
	plan, err := BuildQueryPlan("hello+world")
	if err != nil {
		t.Fatalf("BuildQueryPlan() error = %v", err)
	}
	if len(plan.Trigrams) == 0 {
		t.Fatal("expected trigrams from plus pattern")
	}
}

func TestBuildQueryPlanRepeat(t *testing.T) {
	t.Parallel()
	plan, err := BuildQueryPlan("abc{3}")
	if err != nil {
		t.Fatalf("BuildQueryPlan() error = %v", err)
	}
	if len(plan.Trigrams) == 0 {
		t.Fatal("expected trigrams from repeat pattern")
	}
}

func TestBuildQueryPlanAlternate(t *testing.T) {
	t.Parallel()
	plan, err := BuildQueryPlan("hello|helpme")
	if err != nil {
		t.Fatalf("BuildQueryPlan() error = %v", err)
	}
	// Both branches share "hel" prefix so intersection should have trigrams
	if len(plan.Trigrams) == 0 {
		t.Fatal("expected trigrams from alternation")
	}
}

func TestAnalyzeRegexpEmptyMatch(t *testing.T) {
	t.Parallel()
	re, _ := syntax.Parse("^$", syntax.Perl)
	info := analyzeRegexp(re.Simplify())
	if !info.exact {
		t.Error("empty match should be exact")
	}
}

func TestAnalyzeRegexpDot(t *testing.T) {
	t.Parallel()
	re, _ := syntax.Parse(".", syntax.Perl)
	info := analyzeRegexp(re.Simplify())
	if len(info.required) != 0 {
		t.Error("dot should produce no required trigrams")
	}
}

func TestSearcherNilIndex(t *testing.T) {
	t.Parallel()
	s := NewSearcher(nil)
	_, err := s.Search(context.Background(), "hello", SearchOptions{})
	if err == nil {
		t.Fatal("expected error for nil index")
	}
}

func TestSearcherNilSearcher(t *testing.T) {
	t.Parallel()
	var s *Searcher
	_, err := s.Search(context.Background(), "hello", SearchOptions{})
	if err == nil {
		t.Fatal("expected error for nil searcher")
	}
}

// memIndex is a simple in-memory trigram index for testing.
type memIndex struct {
	postings map[Trigram][]Posting
}

func newMemIndex() *memIndex {
	return &memIndex{postings: make(map[Trigram][]Posting)}
}

func (m *memIndex) Add(_ context.Context, tri Trigram, posting Posting) error {
	m.postings[tri] = append(m.postings[tri], posting)
	return nil
}

func (m *memIndex) Query(_ context.Context, trigrams []Trigram) ([]Posting, error) {
	seen := make(map[string]struct{})
	var results []Posting
	for _, tri := range trigrams {
		for _, p := range m.postings[tri] {
			key := p.WorkspaceID + "\x00" + p.RepositoryID + "\x00" + p.FilePath
			if _, ok := seen[key]; !ok {
				seen[key] = struct{}{}
				results = append(results, p)
			}
		}
	}
	return results, nil
}

func (m *memIndex) Remove(_ context.Context, _ string) error {
	return nil
}

func TestSearcherEndToEnd(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "test.go")
	content := "package main\n\nfunc hello() {\n\tprintln(\"hello world\")\n}\n"
	if err := os.WriteFile(file, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	idx := newMemIndex()
	for _, tri := range Extract([]byte(content)) {
		idx.Add(context.Background(), tri, Posting{
			WorkspaceID:  "ws1",
			RepositoryID: "repo1",
			IndexID:      "idx1",
			FilePath:     file,
			Language:     "Go",
		})
	}

	s := NewSearcher(idx)
	results, err := s.Search(context.Background(), "hello", SearchOptions{})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected at least one result")
	}

	found := false
	for _, r := range results {
		if r.LineNumber > 0 && len(r.MatchRanges) > 0 {
			found = true
		}
	}
	if !found {
		t.Error("expected result with line number and match ranges")
	}
}

func TestSearcherMaxResults(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "repeat.txt")
	var content string
	for i := 0; i < 200; i++ {
		content += "hello world\n"
	}
	if err := os.WriteFile(file, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	idx := newMemIndex()
	for _, tri := range Extract([]byte(content)) {
		idx.Add(context.Background(), tri, Posting{FilePath: file})
	}

	s := NewSearcher(idx)
	results, err := s.Search(context.Background(), "hello", SearchOptions{MaxResults: 5})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) > 5 {
		t.Errorf("got %d results, want <= 5", len(results))
	}
}

func TestSearcherWorkspaceFilter(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(file, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	idx := newMemIndex()
	for _, tri := range Extract([]byte("hello world")) {
		idx.Add(context.Background(), tri, Posting{
			WorkspaceID: "ws1",
			FilePath:    file,
		})
		idx.Add(context.Background(), tri, Posting{
			WorkspaceID: "ws2",
			FilePath:    file,
		})
	}

	s := NewSearcher(idx)
	results, err := s.Search(context.Background(), "hello", SearchOptions{WorkspaceID: "ws1"})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, r := range results {
		if r.WorkspaceID != "ws1" {
			t.Errorf("got workspace %q, want ws1", r.WorkspaceID)
		}
	}
}

func TestSearcherCancelledContext(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	file := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(file, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	idx := newMemIndex()
	for _, tri := range Extract([]byte("hello world")) {
		idx.Add(context.Background(), tri, Posting{FilePath: file})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	s := NewSearcher(idx)
	_, err := s.Search(ctx, "hello", SearchOptions{})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestMatchesOptions(t *testing.T) {
	t.Parallel()

	posting := Posting{WorkspaceID: "ws1", RepositoryID: "repo1", IndexID: "idx1"}

	tests := []struct {
		name string
		opts SearchOptions
		want bool
	}{
		{"empty opts matches all", SearchOptions{}, true},
		{"matching workspace", SearchOptions{WorkspaceID: "ws1"}, true},
		{"wrong workspace", SearchOptions{WorkspaceID: "ws2"}, false},
		{"matching repo", SearchOptions{RepositoryID: "repo1"}, true},
		{"wrong repo", SearchOptions{RepositoryID: "repo2"}, false},
		{"matching index", SearchOptions{IndexID: "idx1"}, true},
		{"wrong index", SearchOptions{IndexID: "idx2"}, false},
	}

	for _, tt := range tests {
		got := matchesOptions(posting, tt.opts)
		if got != tt.want {
			t.Errorf("%s: got %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestCommonPrefixSuffix(t *testing.T) {
	t.Parallel()

	if got := commonPrefix("hello", "help"); got != "hel" {
		t.Errorf("commonPrefix: got %q, want %q", got, "hel")
	}
	if got := commonPrefix("abc", "xyz"); got != "" {
		t.Errorf("commonPrefix different: got %q, want %q", got, "")
	}
	if got := commonSuffix("testing", "running"); got != "ing" {
		t.Errorf("commonSuffix: got %q, want %q", got, "ing")
	}
	if got := commonSuffix("abc", "xyz"); got != "" {
		t.Errorf("commonSuffix different: got %q, want %q", got, "")
	}
}

func TestTrigramSetOperations(t *testing.T) {
	t.Parallel()

	a := trigramSetFromString("hello")
	b := trigramSetFromString("help")

	merged := cloneTrigramSet(a)
	mergeTrigramSet(merged, b)
	if len(merged) < len(a) {
		t.Error("merge should not reduce set size")
	}

	intersected := intersectTrigramSets(a, b)
	// "hel" is common to both
	if len(intersected) == 0 {
		t.Error("expected non-empty intersection for hello/help")
	}

	empty := intersectTrigramSets(a, map[Trigram]struct{}{})
	if len(empty) != 0 {
		t.Error("intersection with empty should be empty")
	}
}

func TestBridgeTrigramSet(t *testing.T) {
	t.Parallel()

	got := bridgeTrigramSet("ab", "cd")
	if len(got) == 0 {
		t.Error("expected trigrams from bridge")
	}

	got = bridgeTrigramSet("", "cd")
	if len(got) != 0 {
		t.Error("expected empty for empty left")
	}

	got = bridgeTrigramSet("ab", "")
	if len(got) != 0 {
		t.Error("expected empty for empty right")
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
