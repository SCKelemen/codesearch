package celfilter

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/cel-go/cel"
)

const maxProgramCost = 1000

// FilterContext contains the attributes available to CEL filter expressions.
type FilterContext struct {
	Language         string
	FilePath         string
	FileExtension    string
	FileSize         int64
	Branch           string
	Repository       string
	ProjectID        string
	CommitAuthor     string
	CommitAuthorName string
	CommitCommitter  string
	CommitDate       time.Time
	CommitMessage    string
	CommitCoauthors  []string
	CommitSource     string
	CommitIsAgent    bool
}

// FilterProgram wraps a compiled CEL program.
type FilterProgram struct {
	expression string
	program    cel.Program
}

// Compile builds a filter program using the default filter environment.
func Compile(expression string) (*FilterProgram, error) {
	env, err := DefaultEnv()
	if err != nil {
		return nil, err
	}

	return env.Compile(expression)
}

// Compile builds a filter program using the provided filter environment.
func (e *FilterEnv) Compile(expression string) (*FilterProgram, error) {
	trimmed := strings.TrimSpace(expression)
	if trimmed == "" {
		trimmed = "true"
	}

	ast, issues := e.env.Compile(trimmed)
	if issues != nil {
		if err := issues.Err(); err != nil {
			return nil, err
		}
	}

	program, err := e.env.Program(ast, cel.CostLimit(maxProgramCost))
	if err != nil {
		return nil, err
	}

	return &FilterProgram{
		expression: trimmed,
		program:    program,
	}, nil
}

// Eval evaluates the filter program against the provided context.
func (p *FilterProgram) Eval(ctx FilterContext) (bool, error) {
	value, _, err := p.program.Eval(activationForContext(ctx))
	if err != nil {
		return false, err
	}

	result, ok := value.Value().(bool)
	if !ok {
		return false, fmt.Errorf("expression must evaluate to bool, got %T", value.Value())
	}

	return result, nil
}

func activationForContext(ctx FilterContext) map[string]any {
	coauthors := make([]string, len(ctx.CommitCoauthors))
	copy(coauthors, ctx.CommitCoauthors)

	return map[string]any{
		"language":           ctx.Language,
		"file_path":          ctx.FilePath,
		"file_extension":     ctx.FileExtension,
		"file_size":          ctx.FileSize,
		"branch":             ctx.Branch,
		"repository":         ctx.Repository,
		"project_id":         ctx.ProjectID,
		"commit_author":      ctx.CommitAuthor,
		"commit_author_name": ctx.CommitAuthorName,
		"commit_committer":   ctx.CommitCommitter,
		"commit_date":        ctx.CommitDate,
		"commit_message":     ctx.CommitMessage,
		"commit_coauthors":   coauthors,
		"commit_source":      ctx.CommitSource,
		"commit_is_agent":    ctx.CommitIsAgent,
	}
}
