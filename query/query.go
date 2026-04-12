// Package query provides a CEL-based query language for code search.
//
// It uses Google's Common Expression Language (CEL) via cel-go to
// let users write expressive, type-safe search filters.
//
// Example filters:
//
//	language == "go" && file.endsWith("_test.go")
//	type == "function" && name.startsWith("Test")
//	refs > 10 && exported == true
//	language in ["go", "rust"] && file.contains("auth")
package query

import (
	"fmt"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
)

// Document represents the variables available in a CEL filter expression.
type Document struct {
	Name       string
	Language   string
	File       string
	Type       string // symbol kind: "function", "class", "interface", etc.
	Exported   bool
	Definition bool
	Refs       int64
	Line       int64
	Branch     string
	Repository string
	Workspace  string
	Project    string
}

// Env creates a CEL environment with all available filter variables.
func Env() (*cel.Env, error) {
	return cel.NewEnv(
		cel.Variable("name", cel.StringType),
		cel.Variable("language", cel.StringType),
		cel.Variable("file", cel.StringType),
		cel.Variable("type", cel.StringType),
		cel.Variable("exported", cel.BoolType),
		cel.Variable("definition", cel.BoolType),
		cel.Variable("refs", cel.IntType),
		cel.Variable("line", cel.IntType),
		cel.Variable("branch", cel.StringType),
		cel.Variable("repository", cel.StringType),
		cel.Variable("workspace", cel.StringType),
		cel.Variable("project", cel.StringType),
	)
}

// Filter is a compiled CEL expression that can evaluate documents.
type Filter struct {
	program cel.Program
	expr    string
}

// Compile parses and type-checks a CEL filter expression.
func Compile(expr string) (*Filter, error) {
	env, err := Env()
	if err != nil {
		return nil, fmt.Errorf("create CEL env: %w", err)
	}

	ast, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return nil, fmt.Errorf("compile CEL: %w", issues.Err())
	}

	// Ensure the expression evaluates to bool
	if ast.OutputType() != cel.BoolType {
		return nil, fmt.Errorf("filter must evaluate to bool, got %s", ast.OutputType())
	}

	prg, err := env.Program(ast)
	if err != nil {
		return nil, fmt.Errorf("program CEL: %w", err)
	}

	return &Filter{program: prg, expr: expr}, nil
}

// Eval evaluates the filter against a document. Returns true if the
// document matches the filter.
func (f *Filter) Eval(doc Document) (bool, error) {
	vars := map[string]any{
		"name":       doc.Name,
		"language":   doc.Language,
		"file":       doc.File,
		"type":       doc.Type,
		"exported":   doc.Exported,
		"definition": doc.Definition,
		"refs":       doc.Refs,
		"line":       doc.Line,
		"branch":     doc.Branch,
		"repository": doc.Repository,
		"workspace":  doc.Workspace,
		"project":    doc.Project,
	}

	out, _, err := f.program.Eval(vars)
	if err != nil {
		return false, fmt.Errorf("eval CEL: %w", err)
	}

	if out.Type() != types.BoolType {
		return false, fmt.Errorf("expected bool, got %s", out.Type())
	}

	return out.Value().(bool), nil
}

// MustCompile is like Compile but panics on error.
func MustCompile(expr string) *Filter {
	f, err := Compile(expr)
	if err != nil {
		panic(err)
	}
	return f
}

// EvalMany filters a slice of documents, returning only those that match.
func (f *Filter) EvalMany(docs []Document) ([]Document, error) {
	var results []Document
	for _, doc := range docs {
		ok, err := f.Eval(doc)
		if err != nil {
			return nil, err
		}
		if ok {
			results = append(results, doc)
		}
	}
	return results, nil
}

// Expression returns the original CEL expression string.
func (f *Filter) Expression() string {
	return f.expr
}

// Validate checks if an expression is syntactically and type-valid
// without compiling a full program.
func Validate(expr string) error {
	env, err := Env()
	if err != nil {
		return err
	}
	_, issues := env.Compile(expr)
	if issues != nil && issues.Err() != nil {
		return issues.Err()
	}
	return nil
}

// Variables returns the list of available CEL variable names.
func Variables() []string {
	return []string{
		"name", "language", "file", "type", "exported",
		"definition", "refs", "line", "branch",
		"repository", "workspace", "project",
	}
}

// Ensure Filter satisfies the interface at compile time.
