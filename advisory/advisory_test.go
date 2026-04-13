package advisory

import "testing"

func TestSeverityFromCVSS(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		score float64
		want  Severity
	}{
		{name: "unknown", score: 0, want: SeverityUnknown},
		{name: "low", score: 2.0, want: SeverityLow},
		{name: "medium", score: 5.0, want: SeverityMedium},
		{name: "high", score: 7.5, want: SeverityHigh},
		{name: "critical", score: 9.5, want: SeverityCritical},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := SeverityFromCVSS(test.score); got != test.want {
				t.Fatalf("SeverityFromCVSS(%v) = %v, want %v", test.score, got, test.want)
			}
		})
	}
}

func TestSeverityString(t *testing.T) {
	t.Parallel()

	tests := []struct {
		severity Severity
		want     string
	}{
		{severity: SeverityUnknown, want: "unknown"},
		{severity: SeverityLow, want: "low"},
		{severity: SeverityMedium, want: "medium"},
		{severity: SeverityHigh, want: "high"},
		{severity: SeverityCritical, want: "critical"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.want, func(t *testing.T) {
			t.Parallel()
			if got := test.severity.String(); got != test.want {
				t.Fatalf("Severity(%d).String() = %q, want %q", test.severity, got, test.want)
			}
		})
	}
}

func TestAffectedPackageContains(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pkg     AffectedPackage
		version string
		want    bool
	}{
		{name: "introduced and fixed in range", pkg: AffectedPackage{IntroducedIn: "1.0.0", FixedIn: "1.2.3"}, version: "1.1.0", want: true},
		{name: "fixed version excluded", pkg: AffectedPackage{IntroducedIn: "1.0.0", FixedIn: "1.2.3"}, version: "1.2.3", want: false},
		{name: "below introduced excluded", pkg: AffectedPackage{IntroducedIn: "1.0.0", FixedIn: "1.2.3"}, version: "0.9.0", want: false},
		{name: "all versions before fix", pkg: AffectedPackage{FixedIn: "2.0.0"}, version: "1.0.0", want: true},
		{name: "last affected included", pkg: AffectedPackage{IntroducedIn: "1.0.0", LastAffected: "1.5.0"}, version: "1.5.0", want: true},
		{name: "after last affected excluded", pkg: AffectedPackage{IntroducedIn: "1.0.0", LastAffected: "1.5.0"}, version: "1.5.1", want: false},
		{name: "everything affected", pkg: AffectedPackage{}, version: "99.99.99", want: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := test.pkg.Contains(test.version); got != test.want {
				t.Fatalf("Contains(%q) = %v, want %v", test.version, got, test.want)
			}
		})
	}
}

func TestAdvisoryAliasesCrossReferencing(t *testing.T) {
	t.Parallel()

	advisory := Advisory{ID: "GHSA-xxxx-xxxx-xxxx", Aliases: []string{"CVE-2024-12345", "OSV-2024-1"}}
	if !containsString(advisory.Aliases, "CVE-2024-12345") {
		t.Fatal("Aliases does not include CVE cross-reference")
	}
	if !containsString(advisory.Aliases, "OSV-2024-1") {
		t.Fatal("Aliases does not include OSV cross-reference")
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
