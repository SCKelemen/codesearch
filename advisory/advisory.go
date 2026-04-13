package advisory

import "time"

// Severity represents a normalized advisory severity level.
type Severity int

const (
	// SeverityUnknown indicates that no usable severity is available.
	SeverityUnknown Severity = iota
	// SeverityLow covers CVSS scores from 0.1 through 3.9.
	SeverityLow
	// SeverityMedium covers CVSS scores from 4.0 through 6.9.
	SeverityMedium
	// SeverityHigh covers CVSS scores from 7.0 through 8.9.
	SeverityHigh
	// SeverityCritical covers CVSS scores from 9.0 through 10.0.
	SeverityCritical
)

// String returns the lowercase string form of the severity.
func (s Severity) String() string {
	switch s {
	case SeverityLow:
		return "low"
	case SeverityMedium:
		return "medium"
	case SeverityHigh:
		return "high"
	case SeverityCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// SeverityFromCVSS maps a CVSS base score to a normalized severity.
func SeverityFromCVSS(score float64) Severity {
	switch {
	case score <= 0:
		return SeverityUnknown
	case score < 4.0:
		return SeverityLow
	case score < 7.0:
		return SeverityMedium
	case score < 9.0:
		return SeverityHigh
	default:
		return SeverityCritical
	}
}

// Ecosystem identifies a package registry.
type Ecosystem string

const (
	// EcosystemNPM identifies the npm package registry.
	EcosystemNPM Ecosystem = "npm"
	// EcosystemPyPI identifies the Python Package Index.
	EcosystemPyPI Ecosystem = "pypi"
	// EcosystemGo identifies Go modules.
	EcosystemGo Ecosystem = "go"
	// EcosystemCargo identifies Rust crates.
	EcosystemCargo Ecosystem = "cargo"
	// EcosystemRubyGems identifies RubyGems packages.
	EcosystemRubyGems Ecosystem = "rubygems"
	// EcosystemMaven identifies Maven artifacts.
	EcosystemMaven Ecosystem = "maven"
	// EcosystemNuGet identifies NuGet packages.
	EcosystemNuGet Ecosystem = "nuget"
)

// Advisory is a normalized vulnerability advisory.
type Advisory struct {
	ID            string
	Aliases       []string
	Source        string
	Severity      Severity
	CVSS          float64
	Title         string
	Description   string
	Published     time.Time
	Modified      time.Time
	Withdrawn     *time.Time
	Affected      []AffectedPackage
	Patterns      []SearchPattern
	IoCs          []IoC
	References    []string
	FixedVersions map[string]string
	FixTemplates  []FixTemplate
}

// AffectedPackage identifies a vulnerable package and version range.
type AffectedPackage struct {
	Ecosystem       Ecosystem
	Name            string
	IntroducedIn    string
	FixedIn         string
	LastAffected    string
	VulnerableRange string
}

// Contains reports whether the given version is within the affected range.
func (ap *AffectedPackage) Contains(version string) bool {
	current, err := ParseVersion(version)
	if err != nil {
		return false
	}

	if ap.IntroducedIn != "" {
		introduced, err := ParseVersion(ap.IntroducedIn)
		if err != nil {
			return false
		}
		if current.LessThan(introduced) {
			return false
		}
	}

	if ap.FixedIn != "" {
		fixed, err := ParseVersion(ap.FixedIn)
		if err != nil {
			return false
		}
		if current.GreaterThanOrEqual(fixed) {
			return false
		}
	}

	if ap.LastAffected != "" {
		lastAffected, err := ParseVersion(ap.LastAffected)
		if err != nil {
			return false
		}
		if current.Compare(lastAffected) > 0 {
			return false
		}
	}

	return true
}

// SearchPattern defines a code pattern to search for.
type SearchPattern struct {
	Type     PatternType
	Query    string
	Language string
	Context  string
}

// PatternType identifies the type of search pattern.
type PatternType string

const (
	// PatternTrigram is a plain text trigram search.
	PatternTrigram PatternType = "trigram"
	// PatternRegex is a regular expression search.
	PatternRegex PatternType = "regex"
	// PatternStructural is a structural or AST-aware search.
	PatternStructural PatternType = "structural"
)

// IoC is an indicator of compromise.
type IoC struct {
	Type    IoCType
	Value   string
	Context string
}

// IoCType identifies the kind of indicator of compromise.
type IoCType string

const (
	// IoCIP is an IP address indicator.
	IoCIP IoCType = "ip"
	// IoCDomain is a domain indicator.
	IoCDomain IoCType = "domain"
	// IoCHash is a file or artifact hash indicator.
	IoCHash IoCType = "hash"
	// IoCURL is a URL indicator.
	IoCURL IoCType = "url"
	// IoCEmail is an email indicator.
	IoCEmail IoCType = "email"
)

// FixTemplate describes an automated remediation template.
type FixTemplate struct {
	Type        FixType
	Description string
	Package     string
	FromVersion string
	ToVersion   string
	Pattern     string
	Replacement string
	Files       []string
	Commands    []string
	AgentAssist bool
}

// FixType identifies the remediation strategy.
type FixType string

const (
	// FixDependencyBump updates a dependency version.
	FixDependencyBump FixType = "dependency_bump"
	// FixAPIMigration migrates call sites to a safer API.
	FixAPIMigration FixType = "api_migration"
	// FixConfigChange updates configuration values.
	FixConfigChange FixType = "config_change"
	// FixCodePatch applies a source patch.
	FixCodePatch FixType = "code_patch"
)
