package patch

import (
	"fmt"
	"testing"

	"github.com/SCKelemen/codesearch/advisory"
	"github.com/SCKelemen/codesearch/vuln"
)

func TestGeneratorGenerateReturnsPatch(t *testing.T) {
	t.Parallel()

	g := NewGenerator()
	finding := vuln.Finding{
		ID:         "finding-1",
		AdvisoryID: "GHSA-1",
		Type:       vuln.FindingDependency,
		Package:    "lodash",
		Version:    "4.17.20",
		FixedIn:    "4.17.21",
		FilePath:   "package.json",
		Fixable:    true,
	}

	patch, err := g.Generate(finding, func(path string) ([]byte, error) {
		return []byte("{\n  \"dependencies\": {\n    \"lodash\": \"4.17.20\"\n  }\n}\n"), nil
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if patch == nil {
		t.Fatal("Generate() = nil, want patch")
	}
}

func TestGeneratorGenerateReturnsNilForUnfixableFinding(t *testing.T) {
	t.Parallel()

	g := NewGenerator()
	patch, err := g.Generate(vuln.Finding{Type: vuln.FindingPattern}, func(path string) ([]byte, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if patch != nil {
		t.Fatalf("Generate() = %#v, want nil", patch)
	}
}

func TestGeneratorGenerateAllMixedFindings(t *testing.T) {
	t.Parallel()

	g := NewGenerator()
	findings := []vuln.Finding{
		{
			ID:         "finding-1",
			AdvisoryID: "GHSA-1",
			Type:       vuln.FindingDependency,
			Package:    "lodash",
			Version:    "4.17.20",
			FixedIn:    "4.17.21",
			FilePath:   "package.json",
			Fixable:    true,
		},
		{ID: "finding-2", Type: vuln.FindingPattern},
		{
			ID:         "finding-3",
			AdvisoryID: "GHSA-2",
			Type:       vuln.FindingPattern,
			FilePath:   "index.js",
			FixTemplate: &advisory.FixTemplate{
				Type:        advisory.FixAPIMigration,
				Pattern:     "legacyCall(userInput)",
				Replacement: "safeCall(userInput)",
			},
		},
	}

	patches := g.GenerateAll(findings, func(path string) ([]byte, error) {
		switch path {
		case "package.json":
			return []byte("{\n  \"dependencies\": {\n    \"lodash\": \"4.17.20\"\n  }\n}\n"), nil
		case "index.js":
			return []byte("legacyCall(userInput)\n"), nil
		default:
			return nil, fmt.Errorf("unexpected path %s", path)
		}
	})
	if len(patches) != 2 {
		t.Fatalf("GenerateAll() len = %d, want 2", len(patches))
	}
}

func TestGeneratorCustomStrategyRegistration(t *testing.T) {
	t.Parallel()

	g := &Generator{}
	g.AddStrategy(customStrategy{patch: &Patch{Description: "custom"}})

	patch, err := g.Generate(vuln.Finding{}, func(path string) ([]byte, error) {
		return nil, nil
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if patch == nil || patch.Description != "custom" {
		t.Fatalf("Generate() = %#v, want custom patch", patch)
	}
}

type customStrategy struct {
	patch *Patch
}

func (s customStrategy) CanFix(vuln.Finding) bool {
	return true
}

func (s customStrategy) Fix(vuln.Finding, func(string) ([]byte, error)) (*Patch, error) {
	return s.patch, nil
}
