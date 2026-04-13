package vuln

import (
	"context"
	"time"

	"github.com/SCKelemen/codesearch/advisory"
)

// DependencyRecord is a resolved dependency installed in a specific project.
// At 55TB scale, there are billions of these records.
type DependencyRecord struct {
	ProjectID    string    // project that has this dependency
	WorkspaceID  string    // workspace the project belongs to
	Repository   string    // repository identifier
	Ecosystem    string    // "npm", "go", "cargo", etc.
	Name         string    // package name
	Version      string    // resolved version
	Direct       bool      // direct vs transitive
	Dev          bool      // dev dependency
	LockfilePath string    // which lockfile declared this
	IndexedAt    time.Time // when this record was indexed
}

// DependencyQuery specifies which dependencies to find.
// All non-empty fields are ANDed together. Advisory-first scans typically set
// Ecosystem and Name, while broader project-scoped queries may leave them empty
// to act as wildcards.
type DependencyQuery struct {
	Ecosystem    string        // optional: "npm", "go", etc.
	Name         string        // optional: package name
	VersionRange *VersionRange // optional: version constraint
	WorkspaceID  string        // optional: scope to workspace
	ProjectID    string        // optional: scope to project
	Direct       *bool         // optional: only direct deps
	Dev          *bool         // optional: only dev deps
	Limit        int           // max results (0 = no limit)
	Cursor       string        // pagination cursor
}

// VersionRange specifies a version constraint for dependency queries.
type VersionRange struct {
	IntroducedIn string // >= this version (empty = all)
	FixedIn      string // < this version (empty = unfixed)
	LastAffected string // <= this version
}

// DependencyPage is a paginated result of dependency records.
type DependencyPage struct {
	Records    []DependencyRecord
	NextCursor string // empty if no more results
	Total      int64  // estimated total matches (-1 if unknown)
}

// DependencyStore provides indexed access to dependency records. At scale, this
// is backed by a distributed store with indexes on hot advisory-first lookup
// fields such as ecosystem, name, and version.
type DependencyStore interface {
	// Put indexes a dependency record. It is idempotent on
	// (project, ecosystem, name, version).
	Put(ctx context.Context, rec DependencyRecord) error

	// PutBatch indexes multiple dependency records in a single logical write.
	PutBatch(ctx context.Context, recs []DependencyRecord) error

	// Query finds dependency records matching the query.
	Query(ctx context.Context, q DependencyQuery) (*DependencyPage, error)

	// Delete removes all dependency records for a project.
	Delete(ctx context.Context, projectID string) error

	// Count returns the total number of indexed dependency records.
	Count(ctx context.Context) (int64, error)
}

// AdvisoryStore persists normalized advisories.
type AdvisoryStore interface {
	// Put stores or updates an advisory. It upserts on ID.
	Put(ctx context.Context, adv advisory.Advisory) error

	// PutBatch stores multiple advisories.
	PutBatch(ctx context.Context, advs []advisory.Advisory) error

	// Get retrieves an advisory by ID.
	Get(ctx context.Context, id string) (*advisory.Advisory, error)

	// List returns advisories modified since the given time, paginated.
	List(ctx context.Context, since time.Time, limit int, cursor string) ([]advisory.Advisory, string, error)

	// LookupByPackage returns advisories affecting a specific package.
	LookupByPackage(ctx context.Context, ecosystem, name string) ([]advisory.Advisory, error)
}

// FindingStore persists vulnerability findings.
type FindingStore interface {
	// Put stores a finding. It upserts on ID.
	Put(ctx context.Context, f Finding) error

	// PutBatch stores multiple findings.
	PutBatch(ctx context.Context, findings []Finding) error

	// Get retrieves a finding by ID.
	Get(ctx context.Context, id string) (*Finding, error)

	// ListByProject returns findings for a project, sorted by severity.
	ListByProject(ctx context.Context, projectID string, limit int, cursor string) ([]Finding, string, error)

	// ListByAdvisory returns all findings for an advisory across all projects.
	ListByAdvisory(ctx context.Context, advisoryID string, limit int, cursor string) ([]Finding, string, error)

	// ListByWorkspace returns findings for a workspace.
	ListByWorkspace(ctx context.Context, workspaceID string, limit int, cursor string) ([]Finding, string, error)

	// Delete removes findings for a project.
	Delete(ctx context.Context, projectID string) error

	// Count returns total findings, optionally filtered by minimum severity.
	Count(ctx context.Context, minSeverity advisory.Severity) (int64, error)
}

// ScanRequest describes a scan job for the distributed scanner.
type ScanRequest struct {
	// Advisory-first: scan all projects for a specific advisory.
	AdvisoryID string

	// Project-first: scan a specific project against all advisories.
	ProjectID string

	// Scope narrowing.
	WorkspaceID string
	Ecosystem   string

	// Options.
	MinSeverity    advisory.Severity
	IncludeDevDeps bool

	// Metadata.
	RequestedAt time.Time
	RequestedBy string // "feed-ingestion", "webhook", "manual"
}

// ScanResult is the output of a distributed scan job.
type ScanResult struct {
	Request         ScanRequest
	Findings        []Finding
	Duration        time.Duration
	ProjectsScanned int64
	Error           string    // non-empty if the scan failed
	CompletedAt     time.Time // when the scan result was produced
}

// DistributedScanner runs scans at scale using storage backends.
type DistributedScanner interface {
	// ScanAdvisory fans out a scan for a single advisory across all projects and
	// streams results as they are produced.
	ScanAdvisory(ctx context.Context, req ScanRequest) (<-chan ScanResult, error)

	// ScanProject scans a single project against all relevant advisories.
	ScanProject(ctx context.Context, req ScanRequest) (*ScanResult, error)

	// ScanAll runs a full scan of all projects against all advisories.
	ScanAll(ctx context.Context, req ScanRequest) (<-chan ScanResult, error)
}
