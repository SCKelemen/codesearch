package vuln

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/SCKelemen/codesearch/advisory"
)

// LocalScanner implements DistributedScanner using in-memory stores. This is
// the reference implementation that proves the interface works and enables
// full end-to-end testing without infrastructure.
type LocalScanner struct {
	deps     DependencyStore
	advs     AdvisoryStore
	findings FindingStore
	opts     ScannerOptions
}

// NewLocalScanner returns a LocalScanner backed by the provided stores.
func NewLocalScanner(deps DependencyStore, advs AdvisoryStore, findings FindingStore, opts ScannerOptions) *LocalScanner {
	if opts.MinSeverity == advisory.SeverityUnknown {
		opts.MinSeverity = advisory.SeverityLow
	}
	return &LocalScanner{
		deps:     deps,
		advs:     advs,
		findings: findings,
		opts:     opts,
	}
}

// ScanAdvisory implements DistributedScanner.
func (s *LocalScanner) ScanAdvisory(ctx context.Context, req ScanRequest) (<-chan ScanResult, error) {
	if s == nil {
		return nil, fmt.Errorf("local scanner is nil")
	}
	if req.AdvisoryID == "" {
		return nil, fmt.Errorf("scan advisory: advisory ID is required")
	}

	adv, err := s.advs.Get(ctx, req.AdvisoryID)
	if err != nil {
		return nil, err
	}
	if adv == nil {
		return nil, fmt.Errorf("scan advisory: advisory %q not found", req.AdvisoryID)
	}

	results := make(chan ScanResult)
	go func() {
		defer close(results)

		start := time.Now()
		if !s.shouldReportSeverity(req.MinSeverity, adv.Severity) {
			return
		}

		byProject := make(map[string][]Finding)
		projectMeta := make(map[string]DependencyRecord)
		for _, affected := range adv.Affected {
			if req.Ecosystem != "" && string(affected.Ecosystem) != req.Ecosystem {
				continue
			}

			query := DependencyQuery{
				Ecosystem:   string(affected.Ecosystem),
				Name:        affected.Name,
				WorkspaceID: req.WorkspaceID,
				ProjectID:   req.ProjectID,
				VersionRange: &VersionRange{
					IntroducedIn: affected.IntroducedIn,
					FixedIn:      affected.FixedIn,
					LastAffected: affected.LastAffected,
				},
			}
			if !s.includeDevDeps(req) {
				dev := false
				query.Dev = &dev
			}

			page, err := s.deps.Query(ctx, query)
			if err != nil {
				s.sendStreamError(ctx, results, req, start, err)
				return
			}

			for _, rec := range page.Records {
				finding := newDependencyFinding(*adv, rec)
				byProject[rec.ProjectID] = append(byProject[rec.ProjectID], finding)
				if _, ok := projectMeta[rec.ProjectID]; !ok {
					projectMeta[rec.ProjectID] = rec
				}
			}
		}

		projectIDs := make([]string, 0, len(byProject))
		for projectID := range byProject {
			projectIDs = append(projectIDs, projectID)
		}
		sort.Strings(projectIDs)

		for _, projectID := range projectIDs {
			if err := ctx.Err(); err != nil {
				return
			}
			findings := deduplicateFindings(byProject[projectID])
			sortFindings(findings)
			if err := s.findings.PutBatch(ctx, findings); err != nil {
				s.sendStreamError(ctx, results, req, start, err)
				return
			}
			result := ScanResult{
				Request:         req,
				Findings:        findings,
				Duration:        time.Since(start),
				ProjectsScanned: 1,
				CompletedAt:     time.Now(),
			}
			select {
			case <-ctx.Done():
				return
			case results <- result:
			}
		}
	}()

	return results, nil
}

// ScanProject implements DistributedScanner.
func (s *LocalScanner) ScanProject(ctx context.Context, req ScanRequest) (*ScanResult, error) {
	if s == nil {
		return nil, fmt.Errorf("local scanner is nil")
	}
	if req.ProjectID == "" {
		return nil, fmt.Errorf("scan project: project ID is required")
	}

	start := time.Now()
	page, err := s.deps.Query(ctx, DependencyQuery{
		ProjectID:   req.ProjectID,
		WorkspaceID: req.WorkspaceID,
		Ecosystem:   req.Ecosystem,
	})
	if err != nil {
		return nil, err
	}

	if err := s.findings.Delete(ctx, req.ProjectID); err != nil {
		return nil, err
	}

	allFindings := make([]Finding, 0)
	for _, rec := range page.Records {
		if rec.Dev && !s.includeDevDeps(req) {
			continue
		}

		advs, err := s.advs.LookupByPackage(ctx, rec.Ecosystem, rec.Name)
		if err != nil {
			return nil, err
		}
		for _, adv := range advs {
			if !s.shouldReportSeverity(req.MinSeverity, adv.Severity) {
				continue
			}
			for _, affected := range adv.Affected {
				if string(affected.Ecosystem) != rec.Ecosystem || affected.Name != rec.Name {
					continue
				}
				if !affected.Contains(rec.Version) {
					continue
				}
				allFindings = append(allFindings, newDependencyFinding(adv, rec))
				break
			}
		}
	}

	allFindings = deduplicateFindings(allFindings)
	sortFindings(allFindings)
	if len(allFindings) > 0 {
		if err := s.findings.PutBatch(ctx, allFindings); err != nil {
			return nil, err
		}
	}

	return &ScanResult{
		Request:         req,
		Findings:        allFindings,
		Duration:        time.Since(start),
		ProjectsScanned: 1,
		CompletedAt:     time.Now(),
	}, nil
}

// ScanAll implements DistributedScanner.
func (s *LocalScanner) ScanAll(ctx context.Context, req ScanRequest) (<-chan ScanResult, error) {
	if s == nil {
		return nil, fmt.Errorf("local scanner is nil")
	}

	page, err := s.deps.Query(ctx, DependencyQuery{
		WorkspaceID: req.WorkspaceID,
		Ecosystem:   req.Ecosystem,
	})
	if err != nil {
		return nil, err
	}

	projects := make(map[string]struct{})
	for _, rec := range page.Records {
		projects[rec.ProjectID] = struct{}{}
	}

	projectIDs := make([]string, 0, len(projects))
	for projectID := range projects {
		projectIDs = append(projectIDs, projectID)
	}
	sort.Strings(projectIDs)

	results := make(chan ScanResult)
	go func() {
		defer close(results)
		for _, projectID := range projectIDs {
			if err := ctx.Err(); err != nil {
				return
			}
			projectReq := req
			projectReq.ProjectID = projectID
			result, err := s.ScanProject(ctx, projectReq)
			if err != nil {
				select {
				case <-ctx.Done():
					return
				case results <- ScanResult{Request: projectReq, Error: err.Error(), CompletedAt: time.Now()}:
				}
				continue
			}
			select {
			case <-ctx.Done():
				return
			case results <- *result:
			}
		}
	}()
	return results, nil
}

func (s *LocalScanner) includeDevDeps(req ScanRequest) bool {
	return s.opts.IncludeDevDeps || req.IncludeDevDeps
}

func (s *LocalScanner) shouldReportSeverity(requestMin, severity advisory.Severity) bool {
	minSeverity := requestMin
	if minSeverity == advisory.SeverityUnknown {
		minSeverity = s.opts.MinSeverity
	}
	if minSeverity == advisory.SeverityUnknown {
		minSeverity = advisory.SeverityLow
	}
	return severity >= minSeverity
}

func (s *LocalScanner) sendStreamError(ctx context.Context, results chan<- ScanResult, req ScanRequest, started time.Time, err error) {
	select {
	case <-ctx.Done():
	case results <- ScanResult{
		Request:     req,
		Error:       err.Error(),
		Duration:    time.Since(started),
		CompletedAt: time.Now(),
	}:
	}
}

func newDependencyFinding(adv advisory.Advisory, rec DependencyRecord) Finding {
	fixedIn := ""
	for _, affected := range adv.Affected {
		if string(affected.Ecosystem) == rec.Ecosystem && affected.Name == rec.Name {
			fixedIn = affected.FixedIn
			break
		}
	}
	if fixedIn == "" && adv.FixedVersions != nil {
		fixedIn = adv.FixedVersions[rec.Name]
	}

	return Finding{
		ID:          distributedDependencyFindingID(adv.ID, rec),
		AdvisoryID:  adv.ID,
		Severity:    adv.Severity,
		CVSS:        adv.CVSS,
		Title:       adv.Title,
		Description: adv.Description,
		Type:        FindingDependency,
		Package:     rec.Name,
		Version:     rec.Version,
		FixedIn:     fixedIn,
		FilePath:    rec.LockfilePath,
		ProjectID:   rec.ProjectID,
		WorkspaceID: rec.WorkspaceID,
		Repository:  rec.Repository,
		Fixable:     fixedIn != "",
		FixTemplate: selectFixTemplate(adv, rec.Name),
		FoundAt:     time.Now(),
		Source:      "dependency",
	}
}

func distributedDependencyFindingID(advisoryID string, rec DependencyRecord) string {
	return "f-" + sanitizeIDPart(advisoryID) + "-" + sanitizeIDPart(rec.ProjectID) + "-" + sanitizeIDPart(rec.Name) + "-" + sanitizeIDPart(rec.Version) + "-" + sanitizeIDPart(rec.LockfilePath)
}
