package patch

import "fmt"

// Patch is a generated fix for a vulnerability finding.
type Patch struct {
	FindingID   string      // the finding this patch addresses
	AdvisoryID  string      // source advisory
	Description string      // human-readable description of the change
	Files       []FilePatch // file-level changes
	Commands    []string    // post-patch commands to run (for example, "npm install")
	Confidence  Confidence  // how confident we are this fix is correct
	AgentAssist bool        // true if the patch needs agent review or completion
}

// Confidence reports how likely a generated patch is to be correct.
type Confidence int

const (
	// ConfidenceHigh marks a fully automated, well-understood fix.
	ConfidenceHigh Confidence = iota
	// ConfidenceMedium marks a likely correct fix that should be verified.
	ConfidenceMedium
	// ConfidenceLow marks a best-guess fix that needs human review.
	ConfidenceLow
)

// String returns the lowercase string form of the confidence value.
func (c Confidence) String() string {
	switch c {
	case ConfidenceHigh:
		return "high"
	case ConfidenceMedium:
		return "medium"
	case ConfidenceLow:
		return "low"
	default:
		return fmt.Sprintf("confidence(%d)", c)
	}
}

// FilePatch describes changes to a single file.
type FilePatch struct {
	Path    string // file path
	Hunks   []Hunk // diff hunks
	Created bool   // true if this is a new file
	Deleted bool   // true if this file should be deleted
}

// Hunk is a contiguous block of changes in a file.
type Hunk struct {
	OldStart int    // starting line in the original file
	OldCount int    // number of lines in the original file
	NewStart int    // starting line in the new file
	NewCount int    // number of lines in the new file
	Lines    []Line // the actual diff lines
}

// Line is a single line in a diff hunk.
type Line struct {
	Op      Op     // add, delete, or context
	Content string // line content without a trailing newline
}

// Op identifies a diff line operation.
type Op int

const (
	// OpContext marks an unchanged line.
	OpContext Op = iota
	// OpAdd marks an added line.
	OpAdd
	// OpDelete marks a removed line.
	OpDelete
)
