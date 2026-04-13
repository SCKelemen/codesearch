package vuln

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/SCKelemen/codesearch/advisory"
	"github.com/SCKelemen/codesearch/lockfile"
)

func TestMemDependencyStorePutAndQuery(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemDependencyStore()

	if err := store.Put(ctx, testDependencyRecord("project-1", "workspace-1", "repo-1", "package-lock.json", lockfile.Dependency{Name: "lodash", Version: "4.17.20", Ecosystem: "npm", Direct: true})); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := store.Put(ctx, testDependencyRecord("project-1", "workspace-1", "repo-1", "package-lock.json", lockfile.Dependency{Name: "react", Version: "18.2.0", Ecosystem: "npm", Direct: true})); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	page, err := store.Query(ctx, DependencyQuery{Ecosystem: "npm", Name: "lodash"})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(page.Records) != 1 {
		t.Fatalf("len(page.Records) = %d, want 1", len(page.Records))
	}
	if page.Records[0].Name != "lodash" {
		t.Fatalf("page.Records[0].Name = %q, want lodash", page.Records[0].Name)
	}
	if page.Total != 1 {
		t.Fatalf("page.Total = %d, want 1", page.Total)
	}
}

func TestMemDependencyStorePutBatchDeduplication(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemDependencyStore()
	records := []DependencyRecord{
		testDependencyRecord("project-1", "workspace-1", "repo-1", "package-lock.json", lockfile.Dependency{Name: "lodash", Version: "4.17.20", Ecosystem: "npm"}),
		testDependencyRecord("project-1", "workspace-1", "repo-1", "pnpm-lock.yaml", lockfile.Dependency{Name: "lodash", Version: "4.17.20", Ecosystem: "npm"}),
		testDependencyRecord("project-2", "workspace-1", "repo-2", "package-lock.json", lockfile.Dependency{Name: "lodash", Version: "4.17.20", Ecosystem: "npm"}),
	}

	if err := store.PutBatch(ctx, records); err != nil {
		t.Fatalf("PutBatch() error = %v", err)
	}

	count, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 2 {
		t.Fatalf("Count() = %d, want 2", count)
	}

	page, err := store.Query(ctx, DependencyQuery{ProjectID: "project-1", Ecosystem: "npm", Name: "lodash"})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(page.Records) != 1 {
		t.Fatalf("len(page.Records) = %d, want 1", len(page.Records))
	}
	if page.Records[0].LockfilePath != "pnpm-lock.yaml" {
		t.Fatalf("LockfilePath = %q, want pnpm-lock.yaml", page.Records[0].LockfilePath)
	}
}

func TestMemDependencyStoreQueryVersionRange(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemDependencyStore()
	for i, version := range []string{"1.0.0", "1.5.0", "2.0.0"} {
		if err := store.Put(ctx, testDependencyRecord(fmt.Sprintf("project-%d", i+1), "workspace-1", fmt.Sprintf("repo-%d", i+1), "package-lock.json", lockfile.Dependency{Name: "pkg", Version: version, Ecosystem: "npm"})); err != nil {
			t.Fatalf("Put() error = %v", err)
		}
	}

	tests := []struct {
		name   string
		rangeQ *VersionRange
		want   []string
	}{
		{name: "introduced", rangeQ: &VersionRange{IntroducedIn: "1.5.0"}, want: []string{"1.5.0", "2.0.0"}},
		{name: "fixed", rangeQ: &VersionRange{FixedIn: "2.0.0"}, want: []string{"1.0.0", "1.5.0"}},
		{name: "last affected", rangeQ: &VersionRange{LastAffected: "1.5.0"}, want: []string{"1.0.0", "1.5.0"}},
		{name: "combined", rangeQ: &VersionRange{IntroducedIn: "1.5.0", FixedIn: "2.0.0"}, want: []string{"1.5.0"}},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			page, err := store.Query(ctx, DependencyQuery{Ecosystem: "npm", Name: "pkg", VersionRange: tc.rangeQ})
			if err != nil {
				t.Fatalf("Query() error = %v", err)
			}
			got := dependencyVersions(page.Records)
			if len(got) != len(tc.want) {
				t.Fatalf("versions = %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Fatalf("versions = %v, want %v", got, tc.want)
				}
			}
		})
	}
}

func TestMemDependencyStoreQueryScopesAndPagination(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemDependencyStore()
	records := []DependencyRecord{
		testDependencyRecord("project-a", "workspace-1", "repo-a", "package-lock.json", lockfile.Dependency{Name: "lodash", Version: "4.17.20", Ecosystem: "npm"}),
		testDependencyRecord("project-b", "workspace-1", "repo-b", "package-lock.json", lockfile.Dependency{Name: "lodash", Version: "4.17.20", Ecosystem: "npm"}),
		testDependencyRecord("project-c", "workspace-2", "repo-c", "package-lock.json", lockfile.Dependency{Name: "lodash", Version: "4.17.20", Ecosystem: "npm"}),
	}
	if err := store.PutBatch(ctx, records); err != nil {
		t.Fatalf("PutBatch() error = %v", err)
	}

	workspacePage, err := store.Query(ctx, DependencyQuery{Ecosystem: "npm", Name: "lodash", WorkspaceID: "workspace-1"})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(workspacePage.Records) != 2 {
		t.Fatalf("len(workspacePage.Records) = %d, want 2", len(workspacePage.Records))
	}

	projectPage, err := store.Query(ctx, DependencyQuery{ProjectID: "project-b"})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(projectPage.Records) != 1 || projectPage.Records[0].ProjectID != "project-b" {
		t.Fatalf("projectPage.Records = %+v, want only project-b", projectPage.Records)
	}

	firstPage, err := store.Query(ctx, DependencyQuery{Ecosystem: "npm", Name: "lodash", Limit: 2})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(firstPage.Records) != 2 {
		t.Fatalf("len(firstPage.Records) = %d, want 2", len(firstPage.Records))
	}
	if firstPage.NextCursor != "2" {
		t.Fatalf("firstPage.NextCursor = %q, want 2", firstPage.NextCursor)
	}
	if firstPage.Records[0].ProjectID != "project-a" || firstPage.Records[1].ProjectID != "project-b" {
		t.Fatalf("first page order = %+v, want project-a then project-b", firstPage.Records)
	}

	secondPage, err := store.Query(ctx, DependencyQuery{Ecosystem: "npm", Name: "lodash", Limit: 2, Cursor: firstPage.NextCursor})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(secondPage.Records) != 1 {
		t.Fatalf("len(secondPage.Records) = %d, want 1", len(secondPage.Records))
	}
	if secondPage.Records[0].ProjectID != "project-c" {
		t.Fatalf("secondPage.Records[0].ProjectID = %q, want project-c", secondPage.Records[0].ProjectID)
	}
}

func TestMemDependencyStoreDeleteAndCount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemDependencyStore()
	if err := store.PutBatch(ctx, []DependencyRecord{
		testDependencyRecord("project-1", "workspace-1", "repo-1", "package-lock.json", lockfile.Dependency{Name: "lodash", Version: "4.17.20", Ecosystem: "npm"}),
		testDependencyRecord("project-1", "workspace-1", "repo-1", "package-lock.json", lockfile.Dependency{Name: "react", Version: "18.2.0", Ecosystem: "npm"}),
		testDependencyRecord("project-2", "workspace-1", "repo-2", "package-lock.json", lockfile.Dependency{Name: "lodash", Version: "4.17.20", Ecosystem: "npm"}),
	}); err != nil {
		t.Fatalf("PutBatch() error = %v", err)
	}

	count, err := store.Count(ctx)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 3 {
		t.Fatalf("Count() = %d, want 3", count)
	}

	if err := store.Delete(ctx, "project-1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	count, err = store.Count(ctx)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("Count() after delete = %d, want 1", count)
	}

	page, err := store.Query(ctx, DependencyQuery{ProjectID: "project-1"})
	if err != nil {
		t.Fatalf("Query() error = %v", err)
	}
	if len(page.Records) != 0 {
		t.Fatalf("len(page.Records) = %d, want 0", len(page.Records))
	}
}

func TestMemAdvisoryStorePutGetAndUpsert(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemAdvisoryStore()
	modified := time.Unix(1700000000, 0)
	adv := testAdvisory("ADV-1", advisory.SeverityHigh, advisory.EcosystemNPM, "lodash", "", "4.17.21", "", modified)
	adv.Title = "Original title"

	if err := store.Put(ctx, adv); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	got, err := store.Get(ctx, "ADV-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || got.Title != "Original title" {
		t.Fatalf("Get() = %+v, want title %q", got, "Original title")
	}

	adv.Title = "Updated title"
	adv.Affected = []advisory.AffectedPackage{{Ecosystem: advisory.EcosystemNPM, Name: "express", FixedIn: "4.18.2"}}
	if err := store.Put(ctx, adv); err != nil {
		t.Fatalf("Put() update error = %v", err)
	}

	got, err = store.Get(ctx, "ADV-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || got.Title != "Updated title" {
		t.Fatalf("Get() after update = %+v, want title %q", got, "Updated title")
	}

	oldPackage, err := store.LookupByPackage(ctx, "npm", "lodash")
	if err != nil {
		t.Fatalf("LookupByPackage() error = %v", err)
	}
	if len(oldPackage) != 0 {
		t.Fatalf("len(oldPackage) = %d, want 0", len(oldPackage))
	}

	newPackage, err := store.LookupByPackage(ctx, "npm", "express")
	if err != nil {
		t.Fatalf("LookupByPackage() error = %v", err)
	}
	if len(newPackage) != 1 || newPackage[0].ID != "ADV-1" {
		t.Fatalf("newPackage = %+v, want advisory ADV-1", newPackage)
	}
}

func TestMemAdvisoryStorePutBatchListAndLookupByPackage(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemAdvisoryStore()
	base := time.Unix(1700000000, 0)
	advs := []advisory.Advisory{
		testAdvisory("ADV-1", advisory.SeverityMedium, advisory.EcosystemNPM, "lodash", "", "4.17.21", "", base),
		testAdvisory("ADV-2", advisory.SeverityHigh, advisory.EcosystemNPM, "lodash", "", "4.17.22", "", base.Add(time.Hour)),
		testAdvisory("ADV-3", advisory.SeverityCritical, advisory.EcosystemGo, "stdlib", "", "1.2.0", "", base.Add(2*time.Hour)),
	}

	if err := store.PutBatch(ctx, advs); err != nil {
		t.Fatalf("PutBatch() error = %v", err)
	}

	firstPage, nextCursor, err := store.List(ctx, base.Add(30*time.Minute), 1, "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(firstPage) != 1 || firstPage[0].ID != "ADV-2" {
		t.Fatalf("firstPage = %+v, want ADV-2", firstPage)
	}
	if nextCursor != "1" {
		t.Fatalf("nextCursor = %q, want 1", nextCursor)
	}

	secondPage, nextCursor, err := store.List(ctx, base.Add(30*time.Minute), 1, nextCursor)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(secondPage) != 1 || secondPage[0].ID != "ADV-3" {
		t.Fatalf("secondPage = %+v, want ADV-3", secondPage)
	}
	if nextCursor != "" {
		t.Fatalf("nextCursor = %q, want empty", nextCursor)
	}

	matches, err := store.LookupByPackage(ctx, "npm", "lodash")
	if err != nil {
		t.Fatalf("LookupByPackage() error = %v", err)
	}
	if len(matches) != 2 || matches[0].ID != "ADV-1" || matches[1].ID != "ADV-2" {
		t.Fatalf("matches = %+v, want ADV-1 and ADV-2", matches)
	}
}

func TestMemFindingStorePutGetAndLists(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemFindingStore()
	findings := []Finding{
		{ID: "f-1", AdvisoryID: "ADV-1", Severity: advisory.SeverityCritical, Package: "lodash", ProjectID: "project-1", WorkspaceID: "workspace-1"},
		{ID: "f-2", AdvisoryID: "ADV-2", Severity: advisory.SeverityHigh, Package: "react", ProjectID: "project-1", WorkspaceID: "workspace-1"},
		{ID: "f-3", AdvisoryID: "ADV-1", Severity: advisory.SeverityMedium, Package: "requests", ProjectID: "project-2", WorkspaceID: "workspace-2"},
	}
	if err := store.Put(ctx, findings[0]); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := store.PutBatch(ctx, findings[1:]); err != nil {
		t.Fatalf("PutBatch() error = %v", err)
	}

	got, err := store.Get(ctx, "f-1")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got == nil || got.ID != "f-1" {
		t.Fatalf("Get() = %+v, want f-1", got)
	}

	projectFindings, nextCursor, err := store.ListByProject(ctx, "project-1", 10, "")
	if err != nil {
		t.Fatalf("ListByProject() error = %v", err)
	}
	if nextCursor != "" {
		t.Fatalf("nextCursor = %q, want empty", nextCursor)
	}
	if len(projectFindings) != 2 {
		t.Fatalf("len(projectFindings) = %d, want 2", len(projectFindings))
	}
	if projectFindings[0].Severity != advisory.SeverityCritical || projectFindings[1].Severity != advisory.SeverityHigh {
		t.Fatalf("projectFindings severities = %v, want critical then high", []advisory.Severity{projectFindings[0].Severity, projectFindings[1].Severity})
	}

	advisoryFindings, _, err := store.ListByAdvisory(ctx, "ADV-1", 10, "")
	if err != nil {
		t.Fatalf("ListByAdvisory() error = %v", err)
	}
	if len(advisoryFindings) != 2 {
		t.Fatalf("len(advisoryFindings) = %d, want 2", len(advisoryFindings))
	}

	workspaceFindings, _, err := store.ListByWorkspace(ctx, "workspace-1", 10, "")
	if err != nil {
		t.Fatalf("ListByWorkspace() error = %v", err)
	}
	if len(workspaceFindings) != 2 {
		t.Fatalf("len(workspaceFindings) = %d, want 2", len(workspaceFindings))
	}
}

func TestMemFindingStoreDeleteAndCount(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	store := NewMemFindingStore()
	if err := store.PutBatch(ctx, []Finding{
		{ID: "f-1", Severity: advisory.SeverityCritical, ProjectID: "project-1"},
		{ID: "f-2", Severity: advisory.SeverityHigh, ProjectID: "project-1"},
		{ID: "f-3", Severity: advisory.SeverityMedium, ProjectID: "project-2"},
		{ID: "f-4", Severity: advisory.SeverityLow, ProjectID: "project-2"},
	}); err != nil {
		t.Fatalf("PutBatch() error = %v", err)
	}

	countAll, err := store.Count(ctx, advisory.SeverityUnknown)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if countAll != 4 {
		t.Fatalf("Count(all) = %d, want 4", countAll)
	}

	countHigh, err := store.Count(ctx, advisory.SeverityHigh)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if countHigh != 2 {
		t.Fatalf("Count(high) = %d, want 2", countHigh)
	}

	if err := store.Delete(ctx, "project-1"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	countAll, err = store.Count(ctx, advisory.SeverityUnknown)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if countAll != 2 {
		t.Fatalf("Count(all) after delete = %d, want 2", countAll)
	}
}

func TestLocalScannerScanAdvisoryEndToEnd(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := NewMemDependencyStore()
	advs := NewMemAdvisoryStore()
	findings := NewMemFindingStore()
	scanner := NewLocalScanner(deps, advs, findings, ScannerOptions{})

	adv := testAdvisory("ADV-1", advisory.SeverityCritical, advisory.EcosystemNPM, "lodash", "", "4.17.21", "", time.Now())
	adv.FixTemplates = []advisory.FixTemplate{{Type: advisory.FixDependencyBump, Package: "lodash", ToVersion: "4.17.21"}}
	if err := advs.Put(ctx, adv); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := deps.PutBatch(ctx, []DependencyRecord{
		testDependencyRecord("project-1", "workspace-1", "repo-1", "package-lock.json", lockfile.Dependency{Name: "lodash", Version: "4.17.20", Ecosystem: "npm"}),
		testDependencyRecord("project-2", "workspace-1", "repo-2", "package-lock.json", lockfile.Dependency{Name: "lodash", Version: "4.17.21", Ecosystem: "npm"}),
	}); err != nil {
		t.Fatalf("PutBatch() error = %v", err)
	}

	stream, err := scanner.ScanAdvisory(ctx, ScanRequest{AdvisoryID: "ADV-1", RequestedAt: time.Now(), RequestedBy: "manual"})
	if err != nil {
		t.Fatalf("ScanAdvisory() error = %v", err)
	}
	results := collectResults(stream)
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if len(results[0].Findings) != 1 {
		t.Fatalf("len(results[0].Findings) = %d, want 1", len(results[0].Findings))
	}
	finding := results[0].Findings[0]
	if finding.ProjectID != "project-1" {
		t.Fatalf("finding.ProjectID = %q, want project-1", finding.ProjectID)
	}
	if !finding.Fixable {
		t.Fatal("finding.Fixable = false, want true")
	}

	stored, _, err := findings.ListByProject(ctx, "project-1", 10, "")
	if err != nil {
		t.Fatalf("ListByProject() error = %v", err)
	}
	if len(stored) != 1 {
		t.Fatalf("len(stored) = %d, want 1", len(stored))
	}
}

func TestLocalScannerScanProject(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := NewMemDependencyStore()
	advs := NewMemAdvisoryStore()
	findings := NewMemFindingStore()
	scanner := NewLocalScanner(deps, advs, findings, ScannerOptions{})

	if err := advs.PutBatch(ctx, []advisory.Advisory{
		testAdvisory("ADV-1", advisory.SeverityHigh, advisory.EcosystemNPM, "lodash", "", "4.17.21", "", time.Now()),
		testAdvisory("ADV-2", advisory.SeverityLow, advisory.EcosystemNPM, "left-pad", "", "1.1.0", "", time.Now()),
	}); err != nil {
		t.Fatalf("PutBatch() error = %v", err)
	}
	if err := deps.PutBatch(ctx, []DependencyRecord{
		testDependencyRecord("project-1", "workspace-1", "repo-1", "package-lock.json", lockfile.Dependency{Name: "lodash", Version: "4.17.20", Ecosystem: "npm"}),
		testDependencyRecord("project-1", "workspace-1", "repo-1", "package-lock.json", lockfile.Dependency{Name: "react", Version: "18.2.0", Ecosystem: "npm"}),
	}); err != nil {
		t.Fatalf("PutBatch() error = %v", err)
	}

	result, err := scanner.ScanProject(ctx, ScanRequest{ProjectID: "project-1", RequestedAt: time.Now(), RequestedBy: "manual"})
	if err != nil {
		t.Fatalf("ScanProject() error = %v", err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("len(result.Findings) = %d, want 1", len(result.Findings))
	}
	if result.Findings[0].Package != "lodash" {
		t.Fatalf("result.Findings[0].Package = %q, want lodash", result.Findings[0].Package)
	}

	count, err := findings.Count(ctx, advisory.SeverityUnknown)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 1 {
		t.Fatalf("Count() = %d, want 1", count)
	}
}

func TestLocalScannerScanAdvisoryFanOut(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := NewMemDependencyStore()
	advs := NewMemAdvisoryStore()
	findings := NewMemFindingStore()
	scanner := NewLocalScanner(deps, advs, findings, ScannerOptions{})

	if err := advs.Put(ctx, testAdvisory("ADV-1", advisory.SeverityCritical, advisory.EcosystemNPM, "lodash", "", "4.17.21", "", time.Now())); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	batch := make([]DependencyRecord, 0, 100)
	for i := 0; i < 100; i++ {
		batch = append(batch, testDependencyRecord(fmt.Sprintf("project-%03d", i), "workspace-1", fmt.Sprintf("repo-%03d", i), "package-lock.json", lockfile.Dependency{Name: "lodash", Version: "4.17.20", Ecosystem: "npm"}))
	}
	if err := deps.PutBatch(ctx, batch); err != nil {
		t.Fatalf("PutBatch() error = %v", err)
	}

	stream, err := scanner.ScanAdvisory(ctx, ScanRequest{AdvisoryID: "ADV-1", RequestedAt: time.Now(), RequestedBy: "feed-ingestion"})
	if err != nil {
		t.Fatalf("ScanAdvisory() error = %v", err)
	}
	results := collectResults(stream)
	if len(results) != 100 {
		t.Fatalf("len(results) = %d, want 100", len(results))
	}
	if total := totalFindings(results); total != 100 {
		t.Fatalf("total findings = %d, want 100", total)
	}

	count, err := findings.Count(ctx, advisory.SeverityUnknown)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 100 {
		t.Fatalf("Count() = %d, want 100", count)
	}
}

func TestLocalScannerVersionRangePrecision(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := NewMemDependencyStore()
	advs := NewMemAdvisoryStore()
	findings := NewMemFindingStore()
	scanner := NewLocalScanner(deps, advs, findings, ScannerOptions{})

	if err := advs.Put(ctx, testAdvisory("ADV-1", advisory.SeverityCritical, advisory.EcosystemNPM, "lodash", "", "4.17.21", "", time.Now())); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := deps.PutBatch(ctx, []DependencyRecord{
		testDependencyRecord("project-1", "workspace-1", "repo-1", "package-lock.json", lockfile.Dependency{Name: "lodash", Version: "4.17.20", Ecosystem: "npm"}),
		testDependencyRecord("project-2", "workspace-1", "repo-2", "package-lock.json", lockfile.Dependency{Name: "lodash", Version: "4.17.21", Ecosystem: "npm"}),
	}); err != nil {
		t.Fatalf("PutBatch() error = %v", err)
	}

	stream, err := scanner.ScanAdvisory(ctx, ScanRequest{AdvisoryID: "ADV-1", RequestedAt: time.Now(), RequestedBy: "manual"})
	if err != nil {
		t.Fatalf("ScanAdvisory() error = %v", err)
	}
	results := collectResults(stream)
	if total := totalFindings(results); total != 1 {
		t.Fatalf("total findings = %d, want 1", total)
	}
	if results[0].Findings[0].ProjectID != "project-1" {
		t.Fatalf("results[0].Findings[0].ProjectID = %q, want project-1", results[0].Findings[0].ProjectID)
	}
}

func TestLocalScannerEmptyResultsForNonVulnerableDeps(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	deps := NewMemDependencyStore()
	advs := NewMemAdvisoryStore()
	findings := NewMemFindingStore()
	scanner := NewLocalScanner(deps, advs, findings, ScannerOptions{})

	if err := advs.Put(ctx, testAdvisory("ADV-1", advisory.SeverityHigh, advisory.EcosystemNPM, "lodash", "", "4.17.21", "", time.Now())); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	if err := deps.Put(ctx, testDependencyRecord("project-1", "workspace-1", "repo-1", "package-lock.json", lockfile.Dependency{Name: "lodash", Version: "4.17.21", Ecosystem: "npm"})); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	result, err := scanner.ScanProject(ctx, ScanRequest{ProjectID: "project-1", RequestedAt: time.Now(), RequestedBy: "manual"})
	if err != nil {
		t.Fatalf("ScanProject() error = %v", err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("len(result.Findings) = %d, want 0", len(result.Findings))
	}

	count, err := findings.Count(ctx, advisory.SeverityUnknown)
	if err != nil {
		t.Fatalf("Count() error = %v", err)
	}
	if count != 0 {
		t.Fatalf("Count() = %d, want 0", count)
	}
}

func collectResults(stream <-chan ScanResult) []ScanResult {
	results := make([]ScanResult, 0)
	for result := range stream {
		results = append(results, result)
	}
	return results
}

func totalFindings(results []ScanResult) int {
	total := 0
	for _, result := range results {
		total += len(result.Findings)
	}
	return total
}

func dependencyVersions(records []DependencyRecord) []string {
	versions := make([]string, 0, len(records))
	for _, rec := range records {
		versions = append(versions, rec.Version)
	}
	return versions
}

func testDependencyRecord(projectID, workspaceID, repository, lockfilePath string, dep lockfile.Dependency) DependencyRecord {
	return DependencyRecord{
		ProjectID:    projectID,
		WorkspaceID:  workspaceID,
		Repository:   repository,
		Ecosystem:    dep.Ecosystem,
		Name:         dep.Name,
		Version:      dep.Version,
		Direct:       dep.Direct,
		Dev:          dep.Dev,
		LockfilePath: lockfilePath,
		IndexedAt:    time.Unix(1700000000, 0),
	}
}

func testAdvisory(id string, severity advisory.Severity, ecosystem advisory.Ecosystem, name, introduced, fixed, last string, modified time.Time) advisory.Advisory {
	return advisory.Advisory{
		ID:       id,
		Severity: severity,
		Title:    id,
		Modified: modified,
		Affected: []advisory.AffectedPackage{{
			Ecosystem:    ecosystem,
			Name:         name,
			IntroducedIn: introduced,
			FixedIn:      fixed,
			LastAffected: last,
		}},
	}
}
