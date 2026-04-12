package celfilter

import (
	"sync"

	"github.com/google/cel-go/cel"
)

// FilterEnv contains the CEL environment used to compile code search filters.
type FilterEnv struct {
	env *cel.Env
}

var (
	defaultEnvOnce sync.Once
	defaultEnv     *FilterEnv
	defaultEnvErr  error
)

// DefaultEnv returns the shared CEL environment for filter compilation.
func DefaultEnv() (*FilterEnv, error) {
	defaultEnvOnce.Do(func() {
		var env *cel.Env
		env, defaultEnvErr = cel.NewEnv(
			cel.Variable("language", cel.StringType),
			cel.Variable("file_path", cel.StringType),
			cel.Variable("file_extension", cel.StringType),
			cel.Variable("file_size", cel.IntType),
			cel.Variable("branch", cel.StringType),
			cel.Variable("repository", cel.StringType),
			cel.Variable("project_id", cel.StringType),
			cel.Variable("commit_author", cel.StringType),
			cel.Variable("commit_author_name", cel.StringType),
			cel.Variable("commit_committer", cel.StringType),
			cel.Variable("commit_date", cel.TimestampType),
			cel.Variable("commit_message", cel.StringType),
			cel.Variable("commit_coauthors", cel.ListType(cel.StringType)),
			cel.Variable("commit_source", cel.StringType),
			cel.Variable("commit_is_agent", cel.BoolType),
		)
		if defaultEnvErr != nil {
			return
		}
		defaultEnv = &FilterEnv{env: env}
	})

	return defaultEnv, defaultEnvErr
}
