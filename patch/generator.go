package patch

import "github.com/SCKelemen/codesearch/vuln"

// Generator creates patches for vulnerability findings.
type Generator struct {
	strategies []Strategy
}

// NewGenerator returns a Generator with the default strategies registered.
func NewGenerator() *Generator {
	return &Generator{
		strategies: []Strategy{
			&DependencyBumpStrategy{},
			&PatternReplaceStrategy{},
		},
	}
}

// AddStrategy registers an additional strategy.
func (g *Generator) AddStrategy(s Strategy) {
	g.strategies = append(g.strategies, s)
}

// Generate creates a patch for a single finding.
func (g *Generator) Generate(f vuln.Finding, content func(path string) ([]byte, error)) (*Patch, error) {
	for _, strategy := range g.strategies {
		if !strategy.CanFix(f) {
			continue
		}
		patch, err := strategy.Fix(f, content)
		if err != nil {
			return nil, err
		}
		if patch != nil {
			return patch, nil
		}
	}
	return nil, nil
}

// GenerateAll creates patches for all fixable findings.
func (g *Generator) GenerateAll(findings []vuln.Finding, content func(path string) ([]byte, error)) []*Patch {
	patches := make([]*Patch, 0, len(findings))
	for _, finding := range findings {
		patch, err := g.Generate(finding, content)
		if err != nil || patch == nil {
			continue
		}
		patches = append(patches, patch)
	}
	return patches
}
