package vuln

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/SCKelemen/codesearch/advisory"
)

// MemDependencyStore is an in-memory DependencyStore for testing.
type MemDependencyStore struct {
	mu      sync.RWMutex
	records []DependencyRecord
}

// NewMemDependencyStore returns a new in-memory dependency store.
func NewMemDependencyStore() *MemDependencyStore {
	return &MemDependencyStore{}
}

// Put indexes a dependency record and deduplicates on
// (project, ecosystem, name, version).
func (s *MemDependencyStore) Put(_ context.Context, rec DependencyRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for i := range s.records {
		existing := s.records[i]
		if existing.ProjectID == rec.ProjectID && existing.Ecosystem == rec.Ecosystem && existing.Name == rec.Name && existing.Version == rec.Version {
			s.records[i] = rec
			return nil
		}
	}

	s.records = append(s.records, rec)
	return nil
}

// PutBatch indexes multiple dependency records.
func (s *MemDependencyStore) PutBatch(ctx context.Context, recs []DependencyRecord) error {
	for _, rec := range recs {
		if err := s.Put(ctx, rec); err != nil {
			return err
		}
	}
	return nil
}

// Query finds dependency records matching the query.
func (s *MemDependencyStore) Query(_ context.Context, q DependencyQuery) (*DependencyPage, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	matches := make([]DependencyRecord, 0)
	for _, rec := range s.records {
		if q.Ecosystem != "" && rec.Ecosystem != q.Ecosystem {
			continue
		}
		if q.Name != "" && rec.Name != q.Name {
			continue
		}
		if q.WorkspaceID != "" && rec.WorkspaceID != q.WorkspaceID {
			continue
		}
		if q.ProjectID != "" && rec.ProjectID != q.ProjectID {
			continue
		}
		if q.Direct != nil && rec.Direct != *q.Direct {
			continue
		}
		if q.Dev != nil && rec.Dev != *q.Dev {
			continue
		}
		ok, err := matchesVersionRange(rec.Version, q.VersionRange)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		matches = append(matches, rec)
	}

	sort.Slice(matches, func(i, j int) bool {
		left := matches[i]
		right := matches[j]
		if left.ProjectID != right.ProjectID {
			return left.ProjectID < right.ProjectID
		}
		if left.WorkspaceID != right.WorkspaceID {
			return left.WorkspaceID < right.WorkspaceID
		}
		if left.Repository != right.Repository {
			return left.Repository < right.Repository
		}
		if left.Ecosystem != right.Ecosystem {
			return left.Ecosystem < right.Ecosystem
		}
		if left.Name != right.Name {
			return left.Name < right.Name
		}
		if left.Version != right.Version {
			return left.Version < right.Version
		}
		if left.LockfilePath != right.LockfilePath {
			return left.LockfilePath < right.LockfilePath
		}
		if left.Direct != right.Direct {
			return left.Direct && !right.Direct
		}
		if left.Dev != right.Dev {
			return !left.Dev && right.Dev
		}
		return left.IndexedAt.Before(right.IndexedAt)
	})

	start, end, nextCursor, err := pageBounds(q.Cursor, len(matches), q.Limit)
	if err != nil {
		return nil, err
	}

	page := &DependencyPage{
		Records:    append([]DependencyRecord(nil), matches[start:end]...),
		NextCursor: nextCursor,
		Total:      int64(len(matches)),
	}
	return page, nil
}

// Delete removes all dependency records for a project.
func (s *MemDependencyStore) Delete(_ context.Context, projectID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	filtered := s.records[:0]
	for _, rec := range s.records {
		if rec.ProjectID != projectID {
			filtered = append(filtered, rec)
		}
	}
	s.records = filtered
	return nil
}

// Count returns the total number of indexed dependency records.
func (s *MemDependencyStore) Count(_ context.Context) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return int64(len(s.records)), nil
}

// MemAdvisoryStore is an in-memory AdvisoryStore for testing.
type MemAdvisoryStore struct {
	mu         sync.RWMutex
	advisories map[string]advisory.Advisory
	byPackage  map[string][]string // "ecosystem:name" -> []advisoryID
}

// NewMemAdvisoryStore returns a new in-memory advisory store.
func NewMemAdvisoryStore() *MemAdvisoryStore {
	return &MemAdvisoryStore{
		advisories: make(map[string]advisory.Advisory),
		byPackage:  make(map[string][]string),
	}
}

// Put stores or updates an advisory.
func (s *MemAdvisoryStore) Put(_ context.Context, adv advisory.Advisory) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if old, ok := s.advisories[adv.ID]; ok {
		s.removeFromPackageIndex(old)
	}
	s.advisories[adv.ID] = cloneAdvisory(adv)
	for _, affected := range adv.Affected {
		key := packageKey(string(affected.Ecosystem), affected.Name)
		s.byPackage[key] = appendUniqueString(s.byPackage[key], adv.ID)
		sort.Strings(s.byPackage[key])
	}
	return nil
}

// PutBatch stores multiple advisories.
func (s *MemAdvisoryStore) PutBatch(ctx context.Context, advs []advisory.Advisory) error {
	for _, adv := range advs {
		if err := s.Put(ctx, adv); err != nil {
			return err
		}
	}
	return nil
}

// Get retrieves an advisory by ID.
func (s *MemAdvisoryStore) Get(_ context.Context, id string) (*advisory.Advisory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	adv, ok := s.advisories[id]
	if !ok {
		return nil, nil
	}
	copy := cloneAdvisory(adv)
	return &copy, nil
}

// List returns advisories modified since the given time, paginated.
func (s *MemAdvisoryStore) List(_ context.Context, since time.Time, limit int, cursor string) ([]advisory.Advisory, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	advs := make([]advisory.Advisory, 0, len(s.advisories))
	for _, adv := range s.advisories {
		if !since.IsZero() && adv.Modified.Before(since) {
			continue
		}
		advs = append(advs, cloneAdvisory(adv))
	}

	sort.Slice(advs, func(i, j int) bool {
		if !advs[i].Modified.Equal(advs[j].Modified) {
			return advs[i].Modified.Before(advs[j].Modified)
		}
		return advs[i].ID < advs[j].ID
	})

	start, end, nextCursor, err := pageBounds(cursor, len(advs), limit)
	if err != nil {
		return nil, "", err
	}
	return advs[start:end], nextCursor, nil
}

// LookupByPackage returns advisories affecting a specific package.
func (s *MemAdvisoryStore) LookupByPackage(_ context.Context, ecosystem, name string) ([]advisory.Advisory, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := s.byPackage[packageKey(ecosystem, name)]
	advs := make([]advisory.Advisory, 0, len(ids))
	for _, id := range ids {
		adv, ok := s.advisories[id]
		if ok {
			advs = append(advs, cloneAdvisory(adv))
		}
	}
	return advs, nil
}

// MemFindingStore is an in-memory FindingStore for testing.
type MemFindingStore struct {
	mu       sync.RWMutex
	findings map[string]Finding
}

// NewMemFindingStore returns a new in-memory finding store.
func NewMemFindingStore() *MemFindingStore {
	return &MemFindingStore{
		findings: make(map[string]Finding),
	}
}

// Put stores or updates a finding.
func (s *MemFindingStore) Put(_ context.Context, f Finding) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.findings[f.ID] = cloneFinding(f)
	return nil
}

// PutBatch stores multiple findings.
func (s *MemFindingStore) PutBatch(ctx context.Context, findings []Finding) error {
	for _, finding := range findings {
		if err := s.Put(ctx, finding); err != nil {
			return err
		}
	}
	return nil
}

// Get retrieves a finding by ID.
func (s *MemFindingStore) Get(_ context.Context, id string) (*Finding, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	finding, ok := s.findings[id]
	if !ok {
		return nil, nil
	}
	copy := cloneFinding(finding)
	return &copy, nil
}

// ListByProject returns findings for a project, sorted by severity.
func (s *MemFindingStore) ListByProject(_ context.Context, projectID string, limit int, cursor string) ([]Finding, string, error) {
	return s.listFindings(cursor, limit, func(f Finding) bool {
		return f.ProjectID == projectID
	})
}

// ListByAdvisory returns all findings for an advisory across all projects.
func (s *MemFindingStore) ListByAdvisory(_ context.Context, advisoryID string, limit int, cursor string) ([]Finding, string, error) {
	return s.listFindings(cursor, limit, func(f Finding) bool {
		return f.AdvisoryID == advisoryID
	})
}

// ListByWorkspace returns findings for a workspace.
func (s *MemFindingStore) ListByWorkspace(_ context.Context, workspaceID string, limit int, cursor string) ([]Finding, string, error) {
	return s.listFindings(cursor, limit, func(f Finding) bool {
		return f.WorkspaceID == workspaceID
	})
}

// Delete removes findings for a project.
func (s *MemFindingStore) Delete(_ context.Context, projectID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, finding := range s.findings {
		if finding.ProjectID == projectID {
			delete(s.findings, id)
		}
	}
	return nil
}

// Count returns total findings, optionally filtered by severity.
func (s *MemFindingStore) Count(_ context.Context, minSeverity advisory.Severity) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var total int64
	for _, finding := range s.findings {
		if minSeverity != advisory.SeverityUnknown && finding.Severity < minSeverity {
			continue
		}
		total++
	}
	return total, nil
}

func (s *MemAdvisoryStore) removeFromPackageIndex(adv advisory.Advisory) {
	for _, affected := range adv.Affected {
		key := packageKey(string(affected.Ecosystem), affected.Name)
		ids := s.byPackage[key]
		filtered := ids[:0]
		for _, id := range ids {
			if id != adv.ID {
				filtered = append(filtered, id)
			}
		}
		if len(filtered) == 0 {
			delete(s.byPackage, key)
			continue
		}
		s.byPackage[key] = filtered
	}
}

func (s *MemFindingStore) listFindings(cursor string, limit int, match func(Finding) bool) ([]Finding, string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	findings := make([]Finding, 0, len(s.findings))
	for _, finding := range s.findings {
		if match(finding) {
			findings = append(findings, cloneFinding(finding))
		}
	}
	sortMemFindings(findings)

	start, end, nextCursor, err := pageBounds(cursor, len(findings), limit)
	if err != nil {
		return nil, "", err
	}
	return findings[start:end], nextCursor, nil
}

func matchesVersionRange(version string, vr *VersionRange) (bool, error) {
	if vr == nil {
		return true, nil
	}

	current, err := advisory.ParseVersion(version)
	if err != nil {
		return false, nil
	}
	if vr.IntroducedIn != "" {
		introduced, err := advisory.ParseVersion(vr.IntroducedIn)
		if err != nil {
			return false, fmt.Errorf("parse introduced version %q: %w", vr.IntroducedIn, err)
		}
		if current.Compare(introduced) < 0 {
			return false, nil
		}
	}
	if vr.FixedIn != "" {
		fixed, err := advisory.ParseVersion(vr.FixedIn)
		if err != nil {
			return false, fmt.Errorf("parse fixed version %q: %w", vr.FixedIn, err)
		}
		if current.Compare(fixed) >= 0 {
			return false, nil
		}
	}
	if vr.LastAffected != "" {
		lastAffected, err := advisory.ParseVersion(vr.LastAffected)
		if err != nil {
			return false, fmt.Errorf("parse last affected version %q: %w", vr.LastAffected, err)
		}
		if current.Compare(lastAffected) > 0 {
			return false, nil
		}
	}
	return true, nil
}

func pageBounds(cursor string, total, limit int) (int, int, string, error) {
	start := 0
	if cursor != "" {
		value, err := strconv.Atoi(cursor)
		if err != nil {
			return 0, 0, "", fmt.Errorf("parse cursor %q: %w", cursor, err)
		}
		if value < 0 {
			return 0, 0, "", fmt.Errorf("parse cursor %q: negative cursor", cursor)
		}
		if value > total {
			value = total
		}
		start = value
	}

	end := total
	if limit > 0 && start+limit < end {
		end = start + limit
	}

	nextCursor := ""
	if end < total {
		nextCursor = strconv.Itoa(end)
	}
	return start, end, nextCursor, nil
}

func appendUniqueString(values []string, value string) []string {
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func cloneAdvisory(adv advisory.Advisory) advisory.Advisory {
	copy := adv
	copy.Aliases = append([]string(nil), adv.Aliases...)
	copy.Affected = append([]advisory.AffectedPackage(nil), adv.Affected...)
	copy.Patterns = append([]advisory.SearchPattern(nil), adv.Patterns...)
	copy.IoCs = append([]advisory.IoC(nil), adv.IoCs...)
	copy.References = append([]string(nil), adv.References...)
	copy.FixTemplates = append([]advisory.FixTemplate(nil), adv.FixTemplates...)
	if adv.FixedVersions != nil {
		copy.FixedVersions = make(map[string]string, len(adv.FixedVersions))
		for key, value := range adv.FixedVersions {
			copy.FixedVersions[key] = value
		}
	}
	return copy
}

func cloneFinding(f Finding) Finding {
	copy := f
	if f.FixTemplate != nil {
		template := *f.FixTemplate
		copy.FixTemplate = &template
	}
	return copy
}

func sortMemFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		left := findings[i]
		right := findings[j]
		if left.Severity != right.Severity {
			return left.Severity > right.Severity
		}
		if left.ProjectID != right.ProjectID {
			return left.ProjectID < right.ProjectID
		}
		if left.WorkspaceID != right.WorkspaceID {
			return left.WorkspaceID < right.WorkspaceID
		}
		if left.AdvisoryID != right.AdvisoryID {
			return left.AdvisoryID < right.AdvisoryID
		}
		if left.Package != right.Package {
			return left.Package < right.Package
		}
		if left.FilePath != right.FilePath {
			return left.FilePath < right.FilePath
		}
		if left.Line != right.Line {
			return left.Line < right.Line
		}
		return left.ID < right.ID
	})
}
