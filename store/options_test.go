package store

import (
	"testing"
)

func TestResolveSearchOptions(t *testing.T) {
	t.Parallel()

	opts := ResolveSearchOptions(
		WithFilter(Filter{RepositoryID: "repo-1", Branch: "main"}),
		WithLimit(50),
	)
	if opts.Filter.RepositoryID != "repo-1" {
		t.Errorf("got RepositoryID=%q, want %q", opts.Filter.RepositoryID, "repo-1")
	}
	if opts.Filter.Branch != "main" {
		t.Errorf("got Branch=%q, want %q", opts.Filter.Branch, "main")
	}
	if opts.Limit != 50 {
		t.Errorf("got Limit=%d, want %d", opts.Limit, 50)
	}
}

func TestResolveListOptions(t *testing.T) {
	t.Parallel()

	opts := ResolveListOptions(
		WithCursor("abc"),
		WithLimit(10),
		WithLanguage("go"),
	)
	if opts.Cursor != "abc" {
		t.Errorf("got Cursor=%q, want %q", opts.Cursor, "abc")
	}
	if opts.Limit != 10 {
		t.Errorf("got Limit=%d, want %d", opts.Limit, 10)
	}
	if opts.Filter.Language != "go" {
		t.Errorf("got Language=%q, want %q", opts.Filter.Language, "go")
	}
}

func TestResolveLookupOptions(t *testing.T) {
	t.Parallel()

	opts := ResolveLookupOptions(
		WithRepositoryID("repo-2"),
		WithBranch("dev"),
	)
	if opts.Filter.RepositoryID != "repo-2" {
		t.Errorf("got RepositoryID=%q, want %q", opts.Filter.RepositoryID, "repo-2")
	}
	if opts.Filter.Branch != "dev" {
		t.Errorf("got Branch=%q, want %q", opts.Filter.Branch, "dev")
	}
}

func TestWithFilterOptions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		opt   option
		check func(Filter) bool
	}{
		{"PathPrefix", WithPathPrefix("src/"), func(f Filter) bool { return f.PathPrefix == "src/" }},
		{"DocumentID", WithDocumentID("doc-1"), func(f Filter) bool { return f.DocumentID == "doc-1" }},
		{"SymbolID", WithSymbolID("sym-1"), func(f Filter) bool { return f.SymbolID == "sym-1" }},
		{"Tier", WithTier(T1), func(f Filter) bool { return f.Tier == T1 }},
		{"Kinds", WithKinds(SymbolKindFunction, SymbolKindClass), func(f Filter) bool { return len(f.Kinds) == 2 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			opts := ResolveSearchOptions(tt.opt)
			if !tt.check(opts.Filter) {
				t.Errorf("filter check failed for %s", tt.name)
			}
		})
	}
}

func TestWithMetadata(t *testing.T) {
	t.Parallel()

	opts := ResolveSearchOptions(
		WithMetadata("team", "security"),
		WithMetadata("env", "prod"),
	)
	if opts.Filter.Metadata["team"] != "security" {
		t.Errorf("got team=%q, want %q", opts.Filter.Metadata["team"], "security")
	}
	if opts.Filter.Metadata["env"] != "prod" {
		t.Errorf("got env=%q, want %q", opts.Filter.Metadata["env"], "prod")
	}
}

func TestCloneFilter(t *testing.T) {
	t.Parallel()

	original := Filter{
		RepositoryID: "repo-1",
		Metadata:     map[string]string{"key": "value"},
		Kinds:        []SymbolKind{SymbolKindFunction},
	}
	clone := cloneFilter(original)

	// Modify clone, original should be unchanged
	clone.RepositoryID = "repo-2"
	clone.Metadata["key"] = "changed"
	clone.Kinds[0] = SymbolKindClass

	if original.RepositoryID != "repo-1" {
		t.Error("clone modified original RepositoryID")
	}
	if original.Metadata["key"] != "value" {
		t.Error("clone modified original Metadata")
	}
	if original.Kinds[0] != SymbolKindFunction {
		t.Error("clone modified original Kinds")
	}
}
