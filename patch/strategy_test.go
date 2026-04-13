package patch

import (
	"errors"
	"reflect"
	"testing"

	"github.com/SCKelemen/codesearch/advisory"
	"github.com/SCKelemen/codesearch/vuln"
)

func TestDependencyBumpStrategyFixNPM(t *testing.T) {
	t.Parallel()

	strategy := &DependencyBumpStrategy{}
	finding := vuln.Finding{
		ID:         "finding-1",
		AdvisoryID: "GHSA-lodash",
		Type:       vuln.FindingDependency,
		Package:    "lodash",
		Version:    "4.17.20",
		FixedIn:    "4.17.21",
		FilePath:   "package.json",
		Fixable:    true,
	}

	content := map[string][]byte{
		"package.json": []byte("{\n  \"dependencies\": {\n    \"lodash\": \"4.17.20\"\n  }\n}\n"),
	}

	patch, err := strategy.Fix(finding, mapContent(content))
	if err != nil {
		t.Fatalf("Fix() error = %v", err)
	}
	if patch.Confidence != ConfidenceHigh {
		t.Fatalf("Fix() confidence = %v, want %v", patch.Confidence, ConfidenceHigh)
	}
	if !reflect.DeepEqual(patch.Commands, []string{"npm install"}) {
		t.Fatalf("Fix() commands = %#v, want %#v", patch.Commands, []string{"npm install"})
	}
	if got := patch.Files[0].Hunks[0].Lines[1].Content; got != `    "lodash": "4.17.21"` {
		t.Fatalf("Fix() replacement = %q", got)
	}
}

func TestDependencyBumpStrategyFixPip(t *testing.T) {
	t.Parallel()

	strategy := &DependencyBumpStrategy{}
	finding := vuln.Finding{
		ID:         "finding-2",
		AdvisoryID: "PYSEC-1",
		Type:       vuln.FindingDependency,
		Package:    "requests",
		Version:    "2.25.0",
		FixedIn:    "2.31.0",
		FilePath:   "requirements.txt",
		Fixable:    true,
	}

	patch, err := strategy.Fix(finding, mapContent(map[string][]byte{
		"requirements.txt": []byte("requests==2.25.0\nflask==2.0.0\n"),
	}))
	if err != nil {
		t.Fatalf("Fix() error = %v", err)
	}
	if got := patch.Files[0].Hunks[0].Lines[1].Content; got != "requests==2.31.0" {
		t.Fatalf("Fix() replacement = %q", got)
	}
}

func TestPatternReplaceStrategyFix(t *testing.T) {
	t.Parallel()

	strategy := &PatternReplaceStrategy{}
	finding := vuln.Finding{
		ID:         "finding-3",
		AdvisoryID: "GHSA-api",
		Package:    "legacy-lib",
		FilePath:   "index.js",
		Type:       vuln.FindingPattern,
		FixTemplate: &advisory.FixTemplate{
			Type:        advisory.FixAPIMigration,
			Description: "Migrate to the safe API",
			Pattern:     "legacyCall(userInput)",
			Replacement: "safeCall(userInput)",
			AgentAssist: true,
		},
	}

	patch, err := strategy.Fix(finding, mapContent(map[string][]byte{
		"index.js": []byte("function run() {\n  legacyCall(userInput)\n}\n"),
	}))
	if err != nil {
		t.Fatalf("Fix() error = %v", err)
	}
	if patch.Confidence != ConfidenceMedium {
		t.Fatalf("Fix() confidence = %v, want %v", patch.Confidence, ConfidenceMedium)
	}
	if !patch.AgentAssist {
		t.Fatal("Fix() AgentAssist = false, want true")
	}
	if got := patch.Files[0].Hunks[0].Lines[1].Content; got != "safeCall(userInput)" {
		t.Fatalf("Fix() replacement = %q", got)
	}
}

func TestStrategyCanFix(t *testing.T) {
	t.Parallel()

	t.Run("dependency bump rejects non-dependency", func(t *testing.T) {
		t.Parallel()

		strategy := &DependencyBumpStrategy{}
		finding := vuln.Finding{Type: vuln.FindingPattern, Fixable: true, FixedIn: "1.0.1"}
		if strategy.CanFix(finding) {
			t.Fatal("CanFix() = true, want false")
		}
	})

	t.Run("pattern replace rejects missing template", func(t *testing.T) {
		t.Parallel()

		strategy := &PatternReplaceStrategy{}
		if strategy.CanFix(vuln.Finding{}) {
			t.Fatal("CanFix() = true, want false")
		}
	})
}

func mapContent(files map[string][]byte) func(path string) ([]byte, error) {
	return func(path string) ([]byte, error) {
		data, ok := files[path]
		if !ok {
			return nil, errors.New("missing content")
		}
		return data, nil
	}
}
