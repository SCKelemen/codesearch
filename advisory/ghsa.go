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

const ghsaAPIURL = "https://api.github.com/graphql"

const ghsaQuery = `query($since: DateTime, $cursor: String) {
  securityAdvisories(updatedSince: $since, first: 100, after: $cursor) {
    pageInfo { hasNextPage endCursor }
    nodes {
      ghsaId
      summary
      description
      severity
      cvss { score }
      publishedAt
      updatedAt
      withdrawnAt
      identifiers { type value }
      references { url }
      vulnerabilities(first: 25) {
        nodes {
          package { ecosystem name }
          vulnerableVersionRange
          firstPatchedVersion { identifier }
        }
      }
    }
  }
}`

// GHSAFeed fetches advisories from the GitHub GraphQL API.
type GHSAFeed struct {
	token  string
	client *http.Client
}

// GHSAOption configures a GHSA feed client.
type GHSAOption func(*GHSAFeed)

// NewGHSAFeed creates a GHSA feed client.
func NewGHSAFeed(token string, opts ...GHSAOption) *GHSAFeed {
	feed := &GHSAFeed{token: token, client: http.DefaultClient}
	for _, opt := range opts {
		opt(feed)
	}
	if feed.client == nil {
		feed.client = http.DefaultClient
	}
	return feed
}

// WithGHSAHTTPClient configures the HTTP client used by the feed.
func WithGHSAHTTPClient(client *http.Client) GHSAOption {
	return func(feed *GHSAFeed) {
		feed.client = client
	}
}

// Name returns the feed identifier.
func (f *GHSAFeed) Name() string { return "ghsa" }

// Fetch retrieves advisories modified since the given time.
func (f *GHSAFeed) Fetch(ctx context.Context, since time.Time) ([]Advisory, error) {
	client := f.client
	if client == nil {
		client = http.DefaultClient
	}

	var advisories []Advisory
	cursor := ""
	for {
		variables := map[string]any{"since": nil, "cursor": nil}
		if !since.IsZero() {
			variables["since"] = since.UTC().Format(time.RFC3339)
		}
		if cursor != "" {
			variables["cursor"] = cursor
		}
		body, err := json.Marshal(map[string]any{"query": ghsaQuery, "variables": variables})
		if err != nil {
			return nil, fmt.Errorf("encode GHSA query: %w", err)
		}

		request, err := http.NewRequestWithContext(ctx, http.MethodPost, ghsaAPIURL, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create GHSA request: %w", err)
		}
		request.Header.Set("Content-Type", "application/json")
		request.Header.Set("Accept", "application/json")
		if f.token != "" {
			request.Header.Set("Authorization", "Bearer "+f.token)
		}

		response, err := client.Do(request)
		if err != nil {
			return nil, fmt.Errorf("fetch GHSA advisories: %w", err)
		}
		payload, err := readJSONResponse(response)
		if err != nil {
			return nil, fmt.Errorf("read GHSA response: %w", err)
		}

		var decoded ghsaResponse
		if err := json.Unmarshal(payload, &decoded); err != nil {
			return nil, fmt.Errorf("decode GHSA response: %w", err)
		}
		if len(decoded.Errors) > 0 {
			return nil, fmt.Errorf("GHSA GraphQL error: %s", decoded.Errors[0].Message)
		}
		for _, node := range decoded.Data.SecurityAdvisories.Nodes {
			advisories = append(advisories, normalizeGHSA(node))
		}
		if !decoded.Data.SecurityAdvisories.PageInfo.HasNextPage {
			break
		}
		cursor = decoded.Data.SecurityAdvisories.PageInfo.EndCursor
		if cursor == "" {
			break
		}
	}
	return advisories, nil
}

type ghsaResponse struct {
	Data struct {
		SecurityAdvisories struct {
			PageInfo ghsaPageInfo `json:"pageInfo"`
			Nodes    []ghsaNode   `json:"nodes"`
		} `json:"securityAdvisories"`
	} `json:"data"`
	Errors []ghsaError `json:"errors"`
}

type ghsaPageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type ghsaError struct {
	Message string `json:"message"`
}

type ghsaNode struct {
	GHSAID          string              `json:"ghsaId"`
	Summary         string              `json:"summary"`
	Description     string              `json:"description"`
	Severity        string              `json:"severity"`
	CVSS            ghsaCVSS            `json:"cvss"`
	PublishedAt     string              `json:"publishedAt"`
	UpdatedAt       string              `json:"updatedAt"`
	WithdrawnAt     string              `json:"withdrawnAt"`
	Identifiers     []ghsaIdentifier    `json:"identifiers"`
	References      []ghsaReference     `json:"references"`
	Vulnerabilities ghsaVulnerabilities `json:"vulnerabilities"`
}

type ghsaCVSS struct {
	Score float64 `json:"score"`
}
type ghsaIdentifier struct{ Type, Value string }
type ghsaReference struct {
	URL string `json:"url"`
}
type ghsaVulnerabilities struct {
	Nodes []ghsaVulnerability `json:"nodes"`
}

type ghsaVulnerability struct {
	Package                ghsaPackage             `json:"package"`
	VulnerableVersionRange string                  `json:"vulnerableVersionRange"`
	FirstPatchedVersion    ghsaFirstPatchedVersion `json:"firstPatchedVersion"`
}

type ghsaPackage struct{ Ecosystem, Name string }
type ghsaFirstPatchedVersion struct {
	Identifier string `json:"identifier"`
}

func normalizeGHSA(node ghsaNode) Advisory {
	aliases := make([]string, 0, len(node.Identifiers))
	for _, identifier := range node.Identifiers {
		if identifier.Value != "" && identifier.Value != node.GHSAID {
			aliases = append(aliases, identifier.Value)
		}
	}
	references := make([]string, 0, len(node.References))
	for _, ref := range node.References {
		if ref.URL != "" {
			references = append(references, ref.URL)
		}
	}
	affected := make([]AffectedPackage, 0, len(node.Vulnerabilities.Nodes))
	fixedVersions := make(map[string]string)
	for _, vulnerability := range node.Vulnerabilities.Nodes {
		pkg := AffectedPackage{Ecosystem: ecosystemFromString(vulnerability.Package.Ecosystem), Name: vulnerability.Package.Name, FixedIn: vulnerability.FirstPatchedVersion.Identifier, VulnerableRange: vulnerability.VulnerableVersionRange}
		affected = append(affected, pkg)
		if pkg.Name != "" && pkg.FixedIn != "" {
			fixedVersions[pkg.Name] = pkg.FixedIn
		}
	}
	published, _ := parseFeedTime(node.PublishedAt)
	modified, _ := parseFeedTime(node.UpdatedAt)
	withdrawn, _ := parseFeedTime(node.WithdrawnAt)
	var withdrawnPtr *time.Time
	if !withdrawn.IsZero() {
		withdrawnPtr = &withdrawn
	}
	return Advisory{ID: node.GHSAID, Aliases: aliases, Source: "ghsa", Severity: severityFromGHSALabel(node.Severity, node.CVSS.Score), CVSS: node.CVSS.Score, Title: node.Summary, Description: node.Description, Published: published, Modified: modified, Withdrawn: withdrawnPtr, Affected: affected, References: references, FixedVersions: fixedVersions}
}

func severityFromGHSALabel(label string, score float64) Severity {
	switch strings.ToUpper(label) {
	case "LOW":
		return SeverityLow
	case "MODERATE", "MEDIUM":
		return SeverityMedium
	case "HIGH":
		return SeverityHigh
	case "CRITICAL":
		return SeverityCritical
	default:
		return SeverityFromCVSS(score)
	}
}
