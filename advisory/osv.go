package advisory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const defaultOSVBaseURL = "https://api.osv.dev"

// OSVFeed fetches advisories from OSV.dev.
type OSVFeed struct {
	baseURL string
	client  *http.Client
}

// OSVOption configures an OSV feed client.
type OSVOption func(*OSVFeed)

// NewOSVFeed creates an OSV feed client.
func NewOSVFeed(opts ...OSVOption) *OSVFeed {
	feed := &OSVFeed{baseURL: defaultOSVBaseURL, client: http.DefaultClient}
	for _, opt := range opts {
		opt(feed)
	}
	if feed.client == nil {
		feed.client = http.DefaultClient
	}
	return feed
}

// WithOSVBaseURL overrides the OSV base URL.
func WithOSVBaseURL(rawURL string) OSVOption {
	return func(feed *OSVFeed) {
		feed.baseURL = rawURL
	}
}

// WithOSVHTTPClient configures the HTTP client used by the feed.
func WithOSVHTTPClient(client *http.Client) OSVOption {
	return func(feed *OSVFeed) {
		feed.client = client
	}
}

// Name returns the feed identifier.
func (f *OSVFeed) Name() string { return "osv" }

// Fetch retrieves advisories modified since the given time.
func (f *OSVFeed) Fetch(ctx context.Context, since time.Time) ([]Advisory, error) {
	requestBody := map[string]any{
		"ecosystems": []string{
			string(EcosystemNPM),
			string(EcosystemPyPI),
			string(EcosystemGo),
			string(EcosystemCargo),
			string(EcosystemRubyGems),
			string(EcosystemMaven),
			string(EcosystemNuGet),
		},
	}
	if !since.IsZero() {
		requestBody["since"] = since.UTC().Format(time.RFC3339)
	}
	decoded, err := f.query(ctx, requestBody)
	if err != nil {
		return nil, err
	}
	advisories := make([]Advisory, 0, len(decoded.Vulns))
	for _, vuln := range decoded.Vulns {
		advisories = append(advisories, normalizeOSV(vuln))
	}
	return advisories, nil
}

// QueryByPackage queries OSV for vulnerabilities affecting a specific package.
func (f *OSVFeed) QueryByPackage(ctx context.Context, ecosystem, name, version string) ([]Advisory, error) {
	decoded, err := f.query(ctx, map[string]any{
		"package": map[string]string{
			"ecosystem": ecosystem,
			"name":      name,
		},
		"version": version,
	})
	if err != nil {
		return nil, err
	}
	advisories := make([]Advisory, 0, len(decoded.Vulns))
	for _, vuln := range decoded.Vulns {
		advisories = append(advisories, normalizeOSV(vuln))
	}
	return advisories, nil
}

type osvResponse struct {
	Vulns []osvVuln `json:"vulns"`
}

type osvVuln struct {
	ID               string              `json:"id"`
	Summary          string              `json:"summary"`
	Details          string              `json:"details"`
	Aliases          []string            `json:"aliases"`
	Severity         []osvSeverity       `json:"severity"`
	Affected         []osvAffected       `json:"affected"`
	References       []osvRef            `json:"references"`
	Published        string              `json:"published"`
	Modified         string              `json:"modified"`
	Withdrawn        string              `json:"withdrawn"`
	DatabaseSpecific osvDatabaseSpecific `json:"database_specific"`
}

type osvSeverity struct {
	Type_ string `json:"type"`
	Score string `json:"score"`
}

type osvAffected struct {
	Package_ osvPackage `json:"package"`
	Ranges   []osvRange `json:"ranges"`
}

type osvPackage struct {
	Ecosystem string `json:"ecosystem"`
	Name      string `json:"name"`
}

type osvRange struct {
	Type_  string     `json:"type"`
	Events []osvEvent `json:"events"`
}

type osvEvent struct {
	Introduced   string `json:"introduced"`
	Fixed        string `json:"fixed"`
	LastAffected string `json:"lastAffected"`
}

type osvRef struct {
	Type_ string `json:"type"`
	URL   string `json:"url"`
}

type osvDatabaseSpecific struct {
	Severity string `json:"severity"`
}

func (f *OSVFeed) query(ctx context.Context, payload map[string]any) (osvResponse, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return osvResponse{}, fmt.Errorf("encode OSV query: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(f.baseURL, "/")+"/v1/query", bytes.NewReader(body))
	if err != nil {
		return osvResponse{}, fmt.Errorf("create OSV request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	client := f.client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return osvResponse{}, fmt.Errorf("query OSV: %w", err)
	}
	responseBody, err := readJSONResponse(response)
	if err != nil {
		return osvResponse{}, fmt.Errorf("read OSV response: %w", err)
	}
	var decoded osvResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return osvResponse{}, fmt.Errorf("decode OSV response: %w", err)
	}
	return decoded, nil
}

func (f *OSVFeed) fetchVuln(ctx context.Context, id string) (Advisory, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(f.baseURL, "/")+"/v1/vulns/"+id, nil)
	if err != nil {
		return Advisory{}, fmt.Errorf("create OSV vulnerability request: %w", err)
	}
	client := f.client
	if client == nil {
		client = http.DefaultClient
	}
	response, err := client.Do(request)
	if err != nil {
		return Advisory{}, fmt.Errorf("fetch OSV vulnerability: %w", err)
	}
	responseBody, err := readJSONResponse(response)
	if err != nil {
		return Advisory{}, fmt.Errorf("read OSV vulnerability response: %w", err)
	}
	var decoded osvVuln
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return Advisory{}, fmt.Errorf("decode OSV vulnerability response: %w", err)
	}
	return normalizeOSV(decoded), nil
}

func normalizeOSV(vuln osvVuln) Advisory {
	affected := make([]AffectedPackage, 0, len(vuln.Affected))
	fixedVersions := make(map[string]string)
	for _, entry := range vuln.Affected {
		introduced, fixed, lastAffected := rangeFromOSV(entry.Ranges)
		pkg := AffectedPackage{
			Ecosystem:       ecosystemFromString(entry.Package_.Ecosystem),
			Name:            entry.Package_.Name,
			IntroducedIn:    introduced,
			FixedIn:         fixed,
			LastAffected:    lastAffected,
			VulnerableRange: formatRange(introduced, fixed, lastAffected),
		}
		affected = append(affected, pkg)
		if pkg.Name != "" && pkg.FixedIn != "" {
			fixedVersions[pkg.Name] = pkg.FixedIn
		}
	}

	references := make([]string, 0, len(vuln.References))
	for _, ref := range vuln.References {
		if ref.URL != "" {
			references = append(references, ref.URL)
		}
	}

	score, severity := severityFromOSV(vuln)
	published, _ := parseFeedTime(vuln.Published)
	modified, _ := parseFeedTime(vuln.Modified)
	withdrawn, _ := parseFeedTime(vuln.Withdrawn)
	var withdrawnPtr *time.Time
	if !withdrawn.IsZero() {
		withdrawnPtr = &withdrawn
	}

	return Advisory{
		ID:            vuln.ID,
		Aliases:       append([]string(nil), vuln.Aliases...),
		Source:        "osv",
		Severity:      severity,
		CVSS:          score,
		Title:         vuln.Summary,
		Description:   vuln.Details,
		Published:     published,
		Modified:      modified,
		Withdrawn:     withdrawnPtr,
		Affected:      affected,
		References:    references,
		FixedVersions: fixedVersions,
	}
}

func rangeFromOSV(ranges []osvRange) (string, string, string) {
	var introduced string
	var fixed string
	var lastAffected string
	for _, currentRange := range ranges {
		for _, event := range currentRange.Events {
			if introduced == "" && event.Introduced != "" {
				introduced = event.Introduced
			}
			if fixed == "" && event.Fixed != "" {
				fixed = event.Fixed
			}
			if lastAffected == "" && event.LastAffected != "" {
				lastAffected = event.LastAffected
			}
		}
	}
	return introduced, fixed, lastAffected
}

func severityFromOSV(vuln osvVuln) (float64, Severity) {
	if vuln.DatabaseSpecific.Severity != "" {
		switch strings.ToLower(vuln.DatabaseSpecific.Severity) {
		case "low":
			return 0, SeverityLow
		case "medium", "moderate":
			return 0, SeverityMedium
		case "high":
			return 0, SeverityHigh
		case "critical":
			return 0, SeverityCritical
		}
	}
	for _, entry := range vuln.Severity {
		if entry.Score == "" {
			continue
		}
		score, err := parseSeverityScore(entry.Score)
		if err == nil {
			return score, SeverityFromCVSS(score)
		}
	}
	return 0, SeverityUnknown
}
