package codesearch

import "log/slog"

// WithLogger sets the structured logger for the engine.
// If not set, slog.Default() is used.
func WithLogger(logger *slog.Logger) Option {
	return func(cfg *engineConfig) {
		cfg.logger = logger
	}
}
