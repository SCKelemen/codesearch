package vuln

import (
	"strings"
	"testing"

	"github.com/SCKelemen/codesearch/advisory"
	"github.com/SCKelemen/codesearch/lockfile"
)

func TestScanDependencies(t *testing.T) {
	t.Parallel()

	advisories := []advisory.Advisory{
		{
			ID:          "CVE-2024-1234",
			Severity:    advisory.SeverityCritical,
			CVSS:        9.8,
			Title:       "Lodash vulnerability",
			Description: "Prototype pollution in lodash.",
			Affected: []advisory.AffectedPackage{{
				Ecosystem: advisory.EcosystemNPM,
				Name:      "lodash",
				FixedIn:   "4.17.21",
			}},
			FixTemplates: []advisory.FixTemplate{{
				Type:        advisory.FixDependencyBump,
				Description: "Upgrade lodash",
				Package:     "lodash",
				FromVersion: "4.17.20",
				ToVersion:   "4.17.21",
			}},
		},
		{
			ID:          "CVE-2024-5678",
			Severity:    advisory.SeverityHigh,
			CVSS:        8.1,
			Title:       "Express vulnerability",
			Description: "Issue in express.",
			Affected: []advisory.AffectedPackage{{
				Ecosystem: advisory.EcosystemNPM,
				Name:      "express",
				FixedIn:   "4.18.2",
			}},
		},
	}
	lf := &lockfile.Lockfile{
		Path:   "package-lock.json",
		Format: lockfile.FormatNPM,
		Dependencies: []lockfile.Dependency{
			{Name: "lodash", Version: "4.17.20", Ecosystem: "npm"},
			{Name: "express", Version: "4.18.2", Ecosystem: "npm"},
			{Name: "react", Version: "18.0.0", Ecosystem: "npm"},
		},
	}

	findings := NewScanner(advisories, ScannerOptions{}).ScanDependencies(lf)
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	finding := findings[0]
	if finding.Package != "lodash" {
		t.Fatalf("Package = %q, want lodash", finding.Package)
	}
	if finding.Severity != advisory.SeverityCritical {
		t.Fatalf("Severity = %v, want critical", finding.Severity)
	}
	if !finding.Fixable {
		t.Fatal("Fixable = false, want true")
	}
	if finding.FixedIn != "4.17.21" {
		t.Fatalf("FixedIn = %q, want 4.17.21", finding.FixedIn)
	}
	if finding.FixTemplate == nil || finding.FixTemplate.ToVersion != "4.17.21" {
		t.Fatalf("FixTemplate = %+v, want dependency bump to 4.17.21", finding.FixTemplate)
	}
}

func TestScanDependenciesMinSeverity(t *testing.T) {
	t.Parallel()

	advisories := []advisory.Advisory{
		{
			ID:       "ADV-low",
			Severity: advisory.SeverityLow,
			Affected: []advisory.AffectedPackage{{Ecosystem: advisory.EcosystemNPM, Name: "left-pad", FixedIn: "1.1.0"}},
		},
		{
			ID:       "ADV-high",
			Severity: advisory.SeverityHigh,
			Affected: []advisory.AffectedPackage{{Ecosystem: advisory.EcosystemNPM, Name: "lodash", FixedIn: "4.17.21"}},
		},
	}
	lf := &lockfile.Lockfile{
		Path:   "package-lock.json",
		Format: lockfile.FormatNPM,
		Dependencies: []lockfile.Dependency{
			{Name: "left-pad", Version: "1.0.0", Ecosystem: "npm"},
			{Name: "lodash", Version: "4.17.20", Ecosystem: "npm"},
		},
	}

	findings := NewScanner(advisories, ScannerOptions{MinSeverity: advisory.SeverityHigh}).ScanDependencies(lf)
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	if findings[0].AdvisoryID != "ADV-high" {
		t.Fatalf("AdvisoryID = %q, want ADV-high", findings[0].AdvisoryID)
	}
}

func TestScanDependenciesDevDeps(t *testing.T) {
	t.Parallel()

	advisories := []advisory.Advisory{{
		ID:       "ADV-dev",
		Severity: advisory.SeverityCritical,
		Affected: []advisory.AffectedPackage{{Ecosystem: advisory.EcosystemNPM, Name: "lodash", FixedIn: "4.17.21"}},
	}}
	lf := &lockfile.Lockfile{
		Path:         "package-lock.json",
		Format:       lockfile.FormatNPM,
		Dependencies: []lockfile.Dependency{{Name: "lodash", Version: "4.17.20", Ecosystem: "npm", Dev: true}},
	}

	findings := NewScanner(advisories, ScannerOptions{}).ScanDependencies(lf)
	if len(findings) != 0 {
		t.Fatalf("len(findings) = %d, want 0", len(findings))
	}
}

func TestScanContentPatterns(t *testing.T) {
	t.Parallel()

	scanner := NewScanner([]advisory.Advisory{{
		ID:       "ADV-eval",
		Severity: advisory.SeverityHigh,
		Title:    "Unsafe eval",
		Patterns: []advisory.SearchPattern{
			{Type: advisory.PatternTrigram, Query: "eval("},
			{Type: advisory.PatternRegex, Query: `eval\s*\(`},
		},
	}}, ScannerOptions{})
	content := []byte("function run() {\n  const result = eval(userInput)\n  return result\n}\n")

	findings := scanner.ScanContent("app.js", content)
	if len(findings) < 1 {
		t.Fatalf("len(findings) = %d, want at least 1", len(findings))
	}
	for _, finding := range findings {
		if finding.Type != FindingPattern {
			t.Fatalf("Type = %v, want FindingPattern", finding.Type)
		}
		if finding.Line != 2 {
			t.Fatalf("Line = %d, want 2", finding.Line)
		}
		if !strings.Contains(finding.Snippet, "eval(userInput)") {
			t.Fatalf("Snippet = %q, want eval(userInput)", finding.Snippet)
		}
	}
}

func TestScanContentIoCs(t *testing.T) {
	t.Parallel()

	scanner := NewScanner([]advisory.Advisory{{
		ID:       "ADV-ioc",
		Severity: advisory.SeverityMedium,
		Title:    "Known bad IP",
		IoCs: []advisory.IoC{{
			Type:  advisory.IoCIP,
			Value: "192.168.1.100",
		}},
	}}, ScannerOptions{})
	content := []byte("const ip = \"192.168.1.100\"\n")

	findings := scanner.ScanContent("config.js", content)
	if len(findings) != 1 {
		t.Fatalf("len(findings) = %d, want 1", len(findings))
	}
	if findings[0].Type != FindingIoC {
		t.Fatalf("Type = %v, want FindingIoC", findings[0].Type)
	}
}

func TestScanProject(t *testing.T) {
	t.Parallel()

	scanner := NewScanner([]advisory.Advisory{
		{
			ID:       "ADV-dep",
			Severity: advisory.SeverityCritical,
			Affected: []advisory.AffectedPackage{{Ecosystem: advisory.EcosystemNPM, Name: "lodash", FixedIn: "4.17.21"}},
		},
		{
			ID:       "ADV-pattern",
			Severity: advisory.SeverityHigh,
			Patterns: []advisory.SearchPattern{{Type: advisory.PatternTrigram, Query: "eval("}},
		},
	}, ScannerOptions{})
	files := map[string][]byte{
		"package-lock.json": []byte(`{
  "name": "example",
  "lockfileVersion": 3,
  "packages": {
    "": {
      "dependencies": {
        "lodash": "^4.17.20"
      }
    },
    "node_modules/lodash": {
      "version": "4.17.20"
    }
  }
}`),
		"src/app.js": []byte("const result = eval(userInput)\n"),
	}

	findings := scanner.ScanProject(func(yield func(path string, content []byte) bool) {
		for path, content := range files {
			if !yield(path, content) {
				return
			}
		}
	})
	if len(findings) != 2 {
		t.Fatalf("len(findings) = %d, want 2", len(findings))
	}

	var hasDependency bool
	var hasPattern bool
	for _, finding := range findings {
		switch finding.Type {
		case FindingDependency:
			hasDependency = true
		case FindingPattern:
			hasPattern = true
		}
	}
	if !hasDependency || !hasPattern {
		t.Fatalf("findings = %+v, want dependency and pattern results", findings)
	}
}

func TestIndex(t *testing.T) {
	t.Parallel()

	advisories := []advisory.Advisory{
		{ID: "ADV-1", Affected: []advisory.AffectedPackage{{Ecosystem: advisory.EcosystemNPM, Name: "lodash"}}},
		{ID: "ADV-2", Affected: []advisory.AffectedPackage{{Ecosystem: advisory.EcosystemNPM, Name: "lodash"}}},
		{ID: "ADV-3", Affected: []advisory.AffectedPackage{{Ecosystem: advisory.EcosystemPyPI, Name: "requests"}}},
	}

	idx := NewIndex(advisories)
	matches := idx.Lookup("npm", "lodash")
	if len(matches) != 2 {
		t.Fatalf("len(matches) = %d, want 2", len(matches))
	}
	if idx.Get("ADV-3") == nil || idx.Get("ADV-3").ID != "ADV-3" {
		t.Fatalf("Get(ADV-3) = %+v, want advisory ADV-3", idx.Get("ADV-3"))
	}
	if idx.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", idx.Len())
	}
}

func TestSummarize(t *testing.T) {
	t.Parallel()

	findings := []Finding{
		{Severity: advisory.SeverityCritical, Type: FindingDependency, Fixable: true, ProjectID: "project-a"},
		{Severity: advisory.SeverityHigh, Type: FindingPattern, Fixable: false, ProjectID: "project-a"},
		{Severity: advisory.SeverityHigh, Type: FindingIoC, Fixable: false, ProjectID: "project-b"},
	}

	summary := Summarize(findings)
	if summary.Total != 3 {
		t.Fatalf("Total = %d, want 3", summary.Total)
	}
	if summary.BySeverity[advisory.SeverityHigh] != 2 {
		t.Fatalf("high count = %d, want 2", summary.BySeverity[advisory.SeverityHigh])
	}
	if summary.ByType[FindingDependency] != 1 || summary.ByType[FindingPattern] != 1 || summary.ByType[FindingIoC] != 1 {
		t.Fatalf("ByType = %+v, want one of each type", summary.ByType)
	}
	if summary.Fixable != 1 || summary.Unfixable != 2 {
		t.Fatalf("fixable/unfixable = %d/%d, want 1/2", summary.Fixable, summary.Unfixable)
	}
	if summary.Projects != 2 {
		t.Fatalf("Projects = %d, want 2", summary.Projects)
	}
}
