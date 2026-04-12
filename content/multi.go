package content

import (
	"context"
	"fmt"
	"io"
	"strings"
)

// MultiResolver dispatches to scheme-specific resolvers.
type MultiResolver struct {
	resolvers map[string]Resolver
}

// NewMultiResolver creates a resolver that dispatches based on URI scheme.
func NewMultiResolver(resolvers ...Resolver) *MultiResolver {
	m := &MultiResolver{resolvers: make(map[string]Resolver)}
	for _, r := range resolvers {
		for _, scheme := range r.Schemes() {
			m.resolvers[scheme] = r
		}
	}
	return m
}

// Resolve dispatches to the appropriate scheme-specific resolver.
func (m *MultiResolver) Resolve(ctx context.Context, uri string) (io.ReadCloser, error) {
	scheme := extractScheme(uri)
	r, ok := m.resolvers[scheme]
	if !ok {
		return nil, fmt.Errorf("no resolver for scheme %q in URI %q", scheme, uri)
	}
	return r.Resolve(ctx, uri)
}

// Schemes returns all registered schemes.
func (m *MultiResolver) Schemes() []string {
	schemes := make([]string, 0, len(m.resolvers))
	for s := range m.resolvers {
		schemes = append(schemes, s)
	}
	return schemes
}

func extractScheme(uri string) string {
	if idx := strings.Index(uri, "://"); idx > 0 {
		return uri[:idx]
	}
	return "file"
}
