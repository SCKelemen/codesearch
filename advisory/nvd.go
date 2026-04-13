package advisory

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultNVDBaseURL = "https://services.nvd.nist.gov"

// NVDFeed fetches advisories from the National Vulnerability Database.
type NVDFeed struct {
	baseURL string
	apiKey  string
	client  *http.Client
}

// NVDOption configures an NVDFeed.
type NVDOption func(*NVDFeed)

// NewNVDFeed creates an NVD feed client.
func NewNVDFeed(opts ...NVDOption) *NVDFeed {
	feed := &NVDFeed{
		baseURL: defaultNVDBaseURL,
		client:  http.DefaultClient,
	}
	for _, opt := range opts {
		opt(feed)
	}
	if feed.client == nil {
		feed.client = http.DefaultClient
	}
	return feed
}

// WithNVDAPIKey configures the optional NVD API key.
func WithNVDAPIKey(key string) NVDOption {
	return func(feed *NVDFeed) {
		feed.apiKey = key
	}
}

// WithNVDBaseURL overrides the base URL for the NVD API.
func WithNVDBaseURL(rawURL string) NVDOption {
	return func(feed *NVDFeed) {
		feed.baseURL = rawURL
	}
}

// WithNVDHTTPClient configures the HTTP client used by the feed.
func WithNVDHTTPClient(client *http.Client) NVDOption {
	return func(feed *NVDFeed) {
		feed.client = client
	}
}

// Name returns the feed identifier.
func (f *NVDFeed) Name() string { return "nvd" }

// Fetch retrieves advisories modified since the given time.
func (f *NVDFeed) Fetch(ctx context.Context, since time.Time) ([]Advisory, error) {
	client := f.client
	if client == nil {
		client = http.DefaultClient
	}

	startIndex := 0
	resultsPerPage := 2000
	var advisories []Advisory

	for {
		endpoint, err := url.Parse(strings.TrimRight(f.baseURL, "/") + "/rest/json/cves/2.0")
		if err != nil {
			return nil, fmt.Errorf("parse NVD base URL: %w", err)
		}
		query := endpoint.Query()
		query.Set("startIndex", fmt.Sprintf("%d", startIndex))
		query.Set("resultsPerPage", fmt.Sprintf("%d", resultsPerPage))
		if !since.IsZero() {
			query.Set("lastModStartDate", since.UTC().Format(time.RFC3339))
			query.Set("lastModEndDate", time.Now().UTC().Format(time.RFC3339))
		}
		endpoint.RawQuery = query.Encode()

		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("create NVD request: %w", err)
		}
		request.Header.Set("Accept", "application/json")
		if f.apiKey != "" {
			request.Header.Set("apiKey", f.apiKey)
		}

		response, err := client.Do(request)
		if err != nil {
			return nil, fmt.Errorf("fetch NVD advisories: %w", err)
		}

		payload, err := readJSONResponse(response)
		if err != nil {
			return nil, fmt.Errorf("read NVD response: %w", err)
		}

		var decoded nvdResponse
		if err := json.Unmarshal(payload, &decoded); err != nil {
			return nil, fmt.Errorf("decode NVD response: %w", err)
		}
		for _, vulnerability := range decoded.Vulnerabilities {
			advisories = append(advisories, normalizeNVDVuln(vulnerability))
		}

		if decoded.TotalResults == 0 || decoded.StartIndex+decoded.ResultsPerPage >= decoded.TotalResults || len(decoded.Vulnerabilities) == 0 {
			break
		}
		startIndex = decoded.StartIndex + decoded.ResultsPerPage
	}

	return advisories, nil
}

type nvdResponse struct {
	ResultsPerPage  int       `json:"resultsPerPage"`
	StartIndex      int       `json:"startIndex"`
	TotalResults    int       `json:"totalResults"`
	Vulnerabilities []nvdVuln `json:"vulnerabilities"`
}

type nvdVuln struct {
	CVE nvdCVE `json:"cve"`
}

type nvdCVE struct {
	ID             string      `json:"id"`
	Descriptions   []nvdDesc   `json:"descriptions"`
	Metrics        nvdMetrics  `json:"metrics"`
	Configurations []nvdConfig `json:"configurations"`
	References     []nvdRef    `json:"references"`
	Published      string      `json:"published"`
	LastModified   string      `json:"lastModified"`
}

type nvdMetrics struct {
	CVSSMetricV31 []nvdCVSSV31 `json:"cvssMetricV31"`
}

type nvdCVSSV31 struct {
	CVSSData nvdCVSSData `json:"cvssData"`
}

type nvdCVSSData struct {
	BaseScore    float64 `json:"baseScore"`
	BaseSeverity string  `json:"baseSeverity"`
}

type nvdDesc struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}

type nvdRef struct {
	URL string `json:"url"`
}

type nvdConfig struct {
	Nodes []nvdNode `json:"nodes"`
}

type nvdNode struct {
	CPEMatch []nvdCPEMatch `json:"cpeMatch"`
}

type nvdCPEMatch struct {
	Vulnerable            bool   `json:"vulnerable"`
	Criteria              string `json:"criteria"`
	VersionStartIncluding string `json:"versionStartIncluding"`
	VersionEndExcluding   string `json:"versionEndExcluding"`
}

func normalizeNVDVuln(vulnerability nvdVuln) Advisory {
	cve := vulnerability.CVE
	score := 0.0
	if len(cve.Metrics.CVSSMetricV31) > 0 {
		score = cve.Metrics.CVSSMetricV31[0].CVSSData.BaseScore
	}

	affected := make([]AffectedPackage, 0)
	fixedVersions := make(map[string]string)
	for _, configuration := range cve.Configurations {
		for _, node := range configuration.Nodes {
			for _, match := range node.CPEMatch {
				if !match.Vulnerable {
					continue
				}
				pkg := affectedFromCPE(match)
				affected = append(affected, pkg)
				if pkg.Name != "" && pkg.FixedIn != "" {
					fixedVersions[pkg.Name] = pkg.FixedIn
				}
			}
		}
	}

	references := make([]string, 0, len(cve.References))
	for _, ref := range cve.References {
		if ref.URL != "" {
			references = append(references, ref.URL)
		}
	}

	description := preferredDescription(cve.Descriptions)
	published, _ := parseFeedTime(cve.Published)
	modified, _ := parseFeedTime(cve.LastModified)

	return Advisory{
		ID:            cve.ID,
		Source:        "nvd",
		Severity:      SeverityFromCVSS(score),
		CVSS:          score,
		Title:         description,
		Description:   description,
		Published:     published,
		Modified:      modified,
		Affected:      affected,
		References:    references,
		FixedVersions: fixedVersions,
	}
}

func affectedFromCPE(match nvdCPEMatch) AffectedPackage {
	name, explicitVersion := parseCPECriteria(match.Criteria)
	pkg := AffectedPackage{
		Name:            name,
		IntroducedIn:    match.VersionStartIncluding,
		FixedIn:         match.VersionEndExcluding,
		VulnerableRange: formatRange(match.VersionStartIncluding, match.VersionEndExcluding, ""),
	}
	if pkg.VulnerableRange == "" && explicitVersion != "" && explicitVersion != "*" && explicitVersion != "-" {
		pkg.IntroducedIn = explicitVersion
		pkg.LastAffected = explicitVersion
		pkg.VulnerableRange = formatRange(explicitVersion, "", explicitVersion)
	}
	return pkg
}

func parseCPECriteria(criteria string) (string, string) {
	parts := strings.Split(criteria, ":")
	if len(parts) < 6 {
		return "", ""
	}
	return parts[4], parts[5]
}
