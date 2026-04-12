package celfilter

import (
	"strings"
	"testing"
	"time"
)

func TestCompileAndEval(t *testing.T) {
	t.Parallel()

	ctx := FilterContext{
		Language:         "go",
		FilePath:         "pkg/celfilter/program.go",
		FileExtension:    ".go",
		FileSize:         2048,
		Branch:           "main",
		Repository:       "lovable",
		ProjectID:        "proj_123",
		CommitAuthor:     "sam@example.com",
		CommitAuthorName: "Sam",
		CommitCommitter:  "ci@example.com",
		CommitDate:       time.Date(2025, time.January, 15, 10, 30, 0, 0, time.UTC),
		CommitMessage:    "add CEL filter engine",
		CommitCoauthors:  []string{"alex@example.com", "sam@example.com"},
		CommitSource:     "github",
		CommitIsAgent:    false,
	}

	tests := []struct {
		name       string
		expression string
		want       bool
	}{
		{
			name:       "basic equality",
			expression: `language == "go"`,
			want:       true,
		},
		{
			name:       "compound and",
			expression: `language == "go" && file_extension == ".go"`,
			want:       true,
		},
		{
			name:       "list membership",
			expression: `"sam@example.com" in commit_coauthors`,
			want:       true,
		},
		{
			name:       "timestamp comparison",
			expression: `commit_date > timestamp("2025-01-01T00:00:00Z")`,
			want:       true,
		},
		{
			name:       "string functions",
			expression: `file_path.startsWith("pkg/")`,
			want:       true,
		},
		{
			name:       "empty filter",
			expression: ``,
			want:       true,
		},
	}

	for _, tt := range tests {
		tc := tt
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			program, err := Compile(tc.expression)
			if err != nil {
				t.Fatalf("Compile() error = %v", err)
			}

			got, err := program.Eval(ctx)
			if err != nil {
				t.Fatalf("Eval() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("Eval() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestCompileInvalidExpression(t *testing.T) {
	t.Parallel()

	_, err := Compile(`language = "go"`)
	if err == nil {
		t.Fatal("Compile() expected error for invalid expression")
	}
}

func TestEvalCostLimit(t *testing.T) {
	t.Parallel()

	program, err := Compile(`commit_coauthors.all(a, commit_coauthors.all(b, a != "needle" && b != "needle"))`)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	coauthors := make([]string, 100)
	for i := range coauthors {
		coauthors[i] = "person-" + strings.Repeat("x", i%5+1)
	}

	_, err = program.Eval(FilterContext{CommitCoauthors: coauthors})
	if err == nil {
		t.Fatal("Eval() expected cost limit error")
	}
	if !strings.Contains(err.Error(), "actual cost limit exceeded") {
		t.Fatalf("Eval() error = %v, want cost limit error", err)
	}
}
