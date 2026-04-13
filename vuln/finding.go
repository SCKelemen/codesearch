package vuln

import (
	"time"

	"github.com/SCKelemen/codesearch/advisory"
)

// Finding represents a discovered vulnerability in a project.
type Finding struct {
	ID          string
	AdvisoryID  string
	Severity    advisory.Severity
	CVSS        float64
	Title       string
	Description string

	// What was found.
	Type     FindingType
	Package  string
	Version  string
	FixedIn  string
	FilePath string
	Line     int
	Snippet  string

	// Where it was found.
	ProjectID   string
	WorkspaceID string
	Repository  string

	// Remediation.
	Fixable     bool
	FixTemplate *advisory.FixTemplate

	// Metadata.
	FoundAt time.Time
	Source  string
}

// FindingType identifies how a vulnerability was detected.
type FindingType int

const (
	// FindingDependency indicates a vulnerable dependency version.
	FindingDependency FindingType = iota
	// FindingPattern indicates a code pattern match such as a vulnerable API.
	FindingPattern
	// FindingIoC indicates an indicator of compromise match.
	FindingIoC
	// FindingSemantic indicates a semantic similarity match.
	FindingSemantic
)

// String returns the lowercase string form of the finding type.
func (ft FindingType) String() string {
	switch ft {
	case FindingDependency:
		return "dependency"
	case FindingPattern:
		return "pattern"
	case FindingIoC:
		return "ioc"
	case FindingSemantic:
		return "semantic"
	default:
		return "unknown"
	}
}

// FindingSummary aggregates findings for reporting.
type FindingSummary struct {
	Total      int
	BySeverity map[advisory.Severity]int
	ByType     map[FindingType]int
	Fixable    int
	Unfixable  int
	Projects   int
}

// Summarize aggregates findings into report-friendly counts.
func Summarize(findings []Finding) FindingSummary {
	summary := FindingSummary{
		BySeverity: make(map[advisory.Severity]int),
		ByType:     make(map[FindingType]int),
	}

	projects := make(map[string]struct{})
	for _, finding := range findings {
		summary.Total++
		summary.BySeverity[finding.Severity]++
		summary.ByType[finding.Type]++
		if finding.Fixable {
			summary.Fixable++
		} else {
			summary.Unfixable++
		}
		if finding.ProjectID != "" {
			projects[finding.ProjectID] = struct{}{}
		}
	}

	summary.Projects = len(projects)
	return summary
}
