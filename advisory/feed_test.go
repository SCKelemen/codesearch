package advisory

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestNVDFeedFetch(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if got := r.Header.Get("apiKey"); got != "test-key" {
			t.Fatalf("apiKey header = %q, want %q", got, "test-key")
		}
		if r.URL.Path != "/rest/json/cves/2.0" {
			t.Fatalf("path = %q, want /rest/json/cves/2.0", r.URL.Path)
		}

		startIndex := r.URL.Query().Get("startIndex")
		w.Header().Set("Content-Type", "application/json")
		switch startIndex {
		case "0":
			_, _ = io.WriteString(w, `{"resultsPerPage":2,"startIndex":0,"totalResults":3,"vulnerabilities":[{"cve":{"id":"CVE-2024-0001","descriptions":[{"lang":"en","value":"First NVD advisory."}],"metrics":{"cvssMetricV31":[{"cvssData":{"baseScore":7.5,"baseSeverity":"HIGH"}}]},"configurations":[{"nodes":[{"cpeMatch":[{"vulnerable":true,"criteria":"cpe:2.3:a:vendor:lodash:*:*:*:*:*:*:*:*","versionStartIncluding":"1.0.0","versionEndExcluding":"1.2.3"}]}]}],"references":[{"url":"https://nvd.nist.gov/vuln/detail/CVE-2024-0001"}],"published":"2024-01-02T03:04:05Z","lastModified":"2024-01-03T03:04:05Z"}},{"cve":{"id":"CVE-2024-0002","descriptions":[{"lang":"en","value":"Second NVD advisory."}],"metrics":{"cvssMetricV31":[{"cvssData":{"baseScore":9.8,"baseSeverity":"CRITICAL"}}]},"configurations":[],"references":[],"published":"2024-01-04T03:04:05Z","lastModified":"2024-01-05T03:04:05Z"}}]}`)
		case "2":
			_, _ = io.WriteString(w, `{"resultsPerPage":2,"startIndex":2,"totalResults":3,"vulnerabilities":[{"cve":{"id":"CVE-2024-0003","descriptions":[{"lang":"en","value":"Third NVD advisory."}],"metrics":{"cvssMetricV31":[{"cvssData":{"baseScore":5.0,"baseSeverity":"MEDIUM"}}]},"configurations":[],"references":[{"url":"https://example.com/CVE-2024-0003"}],"published":"2024-01-06T03:04:05Z","lastModified":"2024-01-07T03:04:05Z"}}]}`)
		default:
			t.Fatalf("unexpected startIndex = %q", startIndex)
		}
	}))
	defer server.Close()

	feed := NewNVDFeed(WithNVDBaseURL(server.URL), WithNVDAPIKey("test-key"), WithNVDHTTPClient(server.Client()))
	advisories, err := feed.Fetch(context.Background(), time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(advisories) != 3 {
		t.Fatalf("len(advisories) = %d, want 3", len(advisories))
	}
	if requestCount.Load() != 2 {
		t.Fatalf("request count = %d, want 2", requestCount.Load())
	}
	if advisories[0].ID != "CVE-2024-0001" || advisories[0].Severity != SeverityHigh || advisories[0].CVSS != 7.5 {
		t.Fatalf("unexpected first advisory: %#v", advisories[0])
	}
	if len(advisories[0].Affected) != 1 || advisories[0].Affected[0].Name != "lodash" || advisories[0].Affected[0].FixedIn != "1.2.3" {
		t.Fatalf("unexpected affected packages: %#v", advisories[0].Affected)
	}
}

func TestNVDFeedFetchErrors(t *testing.T) {
	t.Parallel()

	t.Run("server error", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, "boom", http.StatusInternalServerError) }))
		defer server.Close()
		feed := NewNVDFeed(WithNVDBaseURL(server.URL), WithNVDHTTPClient(server.Client()))
		if _, err := feed.Fetch(context.Background(), time.Time{}); err == nil {
			t.Fatal("Fetch() error = nil, want non-nil")
		}
	})

	t.Run("malformed json", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "not-json")
		}))
		defer server.Close()
		feed := NewNVDFeed(WithNVDBaseURL(server.URL), WithNVDHTTPClient(server.Client()))
		if _, err := feed.Fetch(context.Background(), time.Time{}); err == nil {
			t.Fatal("Fetch() error = nil, want non-nil")
		}
	})
}

func TestGHSAFeedFetch(t *testing.T) {
	t.Parallel()

	var requestCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount.Add(1)
		if got := r.Header.Get("Authorization"); got != "Bearer gh-token" {
			t.Fatalf("Authorization = %q, want %q", got, "Bearer gh-token")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		variables, _ := body["variables"].(map[string]any)
		cursor, _ := variables["cursor"].(string)
		w.Header().Set("Content-Type", "application/json")
		switch cursor {
		case "":
			_, _ = io.WriteString(w, `{"data":{"securityAdvisories":{"pageInfo":{"hasNextPage":true,"endCursor":"cursor-2"},"nodes":[{"ghsaId":"GHSA-aaaa-bbbb-cccc","summary":"First GHSA","description":"First GHSA description.","severity":"HIGH","cvss":{"score":8.1},"publishedAt":"2024-02-01T00:00:00Z","updatedAt":"2024-02-02T00:00:00Z","withdrawnAt":"","identifiers":[{"type":"GHSA","value":"GHSA-aaaa-bbbb-cccc"},{"type":"CVE","value":"CVE-2024-1111"}],"references":[{"url":"https://github.com/advisories/GHSA-aaaa-bbbb-cccc"}],"vulnerabilities":{"nodes":[{"package":{"ecosystem":"NPM","name":"left-pad"},"vulnerableVersionRange":"< 1.2.0","firstPatchedVersion":{"identifier":"1.2.0"}}]}}]}}}`)
		case "cursor-2":
			_, _ = io.WriteString(w, `{"data":{"securityAdvisories":{"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[{"ghsaId":"GHSA-dddd-eeee-ffff","summary":"Second GHSA","description":"Second GHSA description.","severity":"LOW","cvss":{"score":2.2},"publishedAt":"2024-02-03T00:00:00Z","updatedAt":"2024-02-04T00:00:00Z","withdrawnAt":"2024-02-05T00:00:00Z","identifiers":[{"type":"GHSA","value":"GHSA-dddd-eeee-ffff"},{"type":"CVE","value":"CVE-2024-2222"}],"references":[{"url":"https://github.com/advisories/GHSA-dddd-eeee-ffff"}],"vulnerabilities":{"nodes":[{"package":{"ecosystem":"GO","name":"example.com/lib"},"vulnerableVersionRange":">= 0.1.0, < 0.2.0","firstPatchedVersion":{"identifier":"0.2.0"}}]}}]}}}`)
		default:
			t.Fatalf("unexpected cursor = %q", cursor)
		}
	}))
	defer server.Close()

	client := &http.Client{Transport: rewriteTransport{baseURL: server.URL, base: http.DefaultTransport}}
	feed := NewGHSAFeed("gh-token", WithGHSAHTTPClient(client))
	advisories, err := feed.Fetch(context.Background(), time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if len(advisories) != 2 || requestCount.Load() != 2 {
		t.Fatalf("unexpected GHSA result count: len=%d requests=%d", len(advisories), requestCount.Load())
	}
	if advisories[0].ID != "GHSA-aaaa-bbbb-cccc" || advisories[0].Severity != SeverityHigh || !containsString(advisories[0].Aliases, "CVE-2024-1111") {
		t.Fatalf("unexpected first GHSA advisory: %#v", advisories[0])
	}
}

func TestOSVFeedFetchAndQueryByPackage(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/query" {
			t.Fatalf("path = %q, want /v1/query", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("Decode() error = %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		if pkg, ok := body["package"].(map[string]any); ok && pkg["name"] == "left-pad" {
			_, _ = io.WriteString(w, `{"vulns":[{"id":"OSV-2024-0002","summary":"Package query match","details":"OSV package-specific advisory.","aliases":["CVE-2024-3333"],"severity":[{"type":"CVSS_V3","score":"8.8"}],"affected":[{"package":{"ecosystem":"npm","name":"left-pad"},"ranges":[{"type":"ECOSYSTEM","events":[{"introduced":"1.0.0"},{"fixed":"1.2.0"}]}]}],"references":[{"type":"WEB","url":"https://osv.dev/OSV-2024-0002"}],"published":"2024-03-02T00:00:00Z","modified":"2024-03-03T00:00:00Z"}]}`)
			return
		}
		_, _ = io.WriteString(w, `{"vulns":[{"id":"OSV-2024-0001","summary":"General fetch match","details":"OSV advisory details.","aliases":["GHSA-zzzz-yyyy-xxxx"],"database_specific":{"severity":"critical"},"affected":[{"package":{"ecosystem":"go","name":"example.com/module"},"ranges":[{"type":"SEMVER","events":[{"introduced":"0"},{"lastAffected":"1.4.0"}]}]}],"references":[{"type":"WEB","url":"https://osv.dev/OSV-2024-0001"}],"published":"2024-03-01T00:00:00Z","modified":"2024-03-01T12:00:00Z"}]}`)
	}))
	defer server.Close()

	feed := NewOSVFeed(WithOSVBaseURL(server.URL), WithOSVHTTPClient(server.Client()))
	advisories, err := feed.Fetch(context.Background(), time.Time{})
	if err != nil || len(advisories) != 1 {
		t.Fatalf("Fetch() advisories=%d err=%v", len(advisories), err)
	}
	if advisories[0].ID != "OSV-2024-0001" || advisories[0].Severity != SeverityCritical || advisories[0].Affected[0].LastAffected != "1.4.0" {
		t.Fatalf("unexpected OSV advisory: %#v", advisories[0])
	}
	packageAdvisories, err := feed.QueryByPackage(context.Background(), "npm", "left-pad", "1.1.0")
	if err != nil || len(packageAdvisories) != 1 || packageAdvisories[0].Affected[0].FixedIn != "1.2.0" {
		t.Fatalf("unexpected package advisories: %#v err=%v", packageAdvisories, err)
	}
}

func TestOSVFeedFetchEmptyResults(t *testing.T) {
	t.Parallel()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"vulns": []}`)
	}))
	defer server.Close()
	feed := NewOSVFeed(WithOSVBaseURL(server.URL), WithOSVHTTPClient(server.Client()))
	advisories, err := feed.Fetch(context.Background(), time.Time{})
	if err != nil || len(advisories) != 0 {
		t.Fatalf("Fetch() len=%d err=%v", len(advisories), err)
	}
}

type rewriteTransport struct {
	baseURL string
	base    http.RoundTripper
}

func (rt rewriteTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	target, err := url.Parse(rt.baseURL)
	if err != nil {
		return nil, err
	}
	clone := r.Clone(r.Context())
	clone.URL.Scheme = target.Scheme
	clone.URL.Host = target.Host
	clone.URL.Path = strings.TrimRight(target.Path, "/") + clone.URL.Path
	clone.Host = target.Host
	return rt.base.RoundTrip(clone)
}
