package vuln

import (
	"testing"
	"time"

	"github.com/SCKelemen/codesearch/advisory"
)

func TestFindingTypeString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ft   FindingType
		want string
	}{
		{name: "dependency", ft: FindingDependency, want: "dependency"},
		{name: "pattern", ft: FindingPattern, want: "pattern"},
		{name: "ioc", ft: FindingIoC, want: "ioc"},
		{name: "semantic", ft: FindingSemantic, want: "semantic"},
		{name: "unknown", ft: FindingType(99), want: "unknown"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.ft.String(); got != tc.want {
				t.Fatalf("FindingType(%d).String() = %q, want %q", tc.ft, got, tc.want)
			}
		})
	}
}

func TestSummarizeEmpty(t *testing.T) {
	t.Parallel()

	summary := Summarize(nil)
	if summary.Total != 0 {
		t.Fatalf("Total = %d, want 0", summary.Total)
	}
	if summary.Fixable != 0 || summary.Unfixable != 0 || summary.Projects != 0 {
		t.Fatalf("summary = %+v, want zero counts", summary)
	}
	if len(summary.BySeverity) != 0 {
		t.Fatalf("len(BySeverity) = %d, want 0", len(summary.BySeverity))
	}
	if len(summary.ByType) != 0 {
		t.Fatalf("len(ByType) = %d, want 0", len(summary.ByType))
	}
}

func TestSummarizeMixedFindings(t *testing.T) {
	t.Parallel()

	now := time.Unix(1700000000, 0)
	findings := []Finding{
		{AdvisoryID: "ADV-1", Severity: advisory.SeverityCritical, Type: FindingDependency, Fixable: true, ProjectID: "p-1", FoundAt: now},
		{AdvisoryID: "ADV-2", Severity: advisory.SeverityHigh, Type: FindingPattern, Fixable: false, ProjectID: "p-1", FoundAt: now},
		{AdvisoryID: "ADV-3", Severity: advisory.SeverityHigh, Type: FindingIoC, Fixable: false, ProjectID: "p-2", FoundAt: now},
	}

	summary := Summarize(findings)
	if summary.Total != 3 {
		t.Fatalf("Total = %d, want 3", summary.Total)
	}
	if summary.BySeverity[advisory.SeverityCritical] != 1 {
		t.Fatalf("critical count = %d, want 1", summary.BySeverity[advisory.SeverityCritical])
	}
	if summary.BySeverity[advisory.SeverityHigh] != 2 {
		t.Fatalf("high count = %d, want 2", summary.BySeverity[advisory.SeverityHigh])
	}
	if summary.ByType[FindingDependency] != 1 {
		t.Fatalf("dependency count = %d, want 1", summary.ByType[FindingDependency])
	}
	if summary.ByType[FindingPattern] != 1 {
		t.Fatalf("pattern count = %d, want 1", summary.ByType[FindingPattern])
	}
	if summary.ByType[FindingIoC] != 1 {
		t.Fatalf("ioc count = %d, want 1", summary.ByType[FindingIoC])
	}
	if summary.Fixable != 1 || summary.Unfixable != 2 {
		t.Fatalf("fixable/unfixable = %d/%d, want 1/2", summary.Fixable, summary.Unfixable)
	}
	if summary.Projects != 2 {
		t.Fatalf("Projects = %d, want 2", summary.Projects)
	}
}
