package vuln

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/SCKelemen/codesearch/advisory"
	"github.com/SCKelemen/codesearch/lockfile"
)

// Scanner scans dependencies and code for known vulnerabilities.
type Scanner struct {
	advisories []advisory.Advisory
	opts       ScannerOptions
	idx        *Index
}

// ScannerOptions controls scanner behavior.
type ScannerOptions struct {
	MinSeverity    advisory.Severity
	IncludeDevDeps bool
}

// NewScanner returns a scanner configured with the provided advisories.
func NewScanner(advisories []advisory.Advisory, opts ScannerOptions) *Scanner {
	if opts.MinSeverity == advisory.SeverityUnknown {
		opts.MinSeverity = advisory.SeverityLow
	}

	return &Scanner{
		advisories: append([]advisory.Advisory(nil), advisories...),
		opts:       opts,
	}
}

// ScanDependencies checks a lockfile's dependencies against known advisories.
func (s *Scanner) ScanDependencies(lf *lockfile.Lockfile) []Finding {
	if s == nil || lf == nil {
		return nil
	}

	findings := make([]Finding, 0)
	for _, dep := range lf.Dependencies {
		if dep.Dev && !s.opts.IncludeDevDeps {
			continue
		}

		for _, adv := range s.advisoriesForDependency(dep) {
			if !s.shouldReport(adv.Severity) {
				continue
			}

			for _, affected := range adv.Affected {
				if string(affected.Ecosystem) != dep.Ecosystem || affected.Name != dep.Name {
					continue
				}
				if !affected.Contains(dep.Version) {
					continue
				}

				fixedIn := affected.FixedIn
				if fixedIn == "" && adv.FixedVersions != nil {
					fixedIn = adv.FixedVersions[dep.Name]
				}

				findings = append(findings, Finding{
					ID:          dependencyFindingID(adv.ID, dep.Name),
					AdvisoryID:  adv.ID,
					Severity:    adv.Severity,
					CVSS:        adv.CVSS,
					Title:       adv.Title,
					Description: adv.Description,
					Type:        FindingDependency,
					Package:     dep.Name,
					Version:     dep.Version,
					FixedIn:     fixedIn,
					FilePath:    lf.Path,
					Fixable:     fixedIn != "",
					FixTemplate: selectFixTemplate(adv, dep.Name),
					FoundAt:     time.Now(),
					Source:      "dependency",
				})
				break
			}
		}
	}

	sortFindings(findings)
	return findings
}

// ScanContent checks file content against advisory patterns and indicators.
func (s *Scanner) ScanContent(path string, content []byte) []Finding {
	if s == nil {
		return nil
	}

	text := string(content)
	findings := make([]Finding, 0)
	for _, adv := range s.advisories {
		if !s.shouldReport(adv.Severity) {
			continue
		}

		for _, pattern := range adv.Patterns {
			switch pattern.Type {
			case advisory.PatternTrigram:
				for _, offset := range findAllSubstrings(text, pattern.Query) {
					findings = append(findings, s.newContentFinding(path, text, adv, FindingPattern, offset, "pattern"))
				}
			case advisory.PatternRegex:
				re, err := regexp.Compile(pattern.Query)
				if err != nil {
					continue
				}
				for _, match := range re.FindAllStringIndex(text, -1) {
					if len(match) == 2 {
						findings = append(findings, s.newContentFinding(path, text, adv, FindingPattern, match[0], "pattern"))
					}
				}
			case advisory.PatternStructural:
				continue
			}
		}

		for _, ioc := range adv.IoCs {
			for _, offset := range findAllSubstrings(text, ioc.Value) {
				findings = append(findings, s.newContentFinding(path, text, adv, FindingIoC, offset, "pattern"))
			}
		}
	}

	sortFindings(findings)
	return findings
}

// ScanProject scans a complete project with the provided walker.
func (s *Scanner) ScanProject(walkFn func(yield func(path string, content []byte) bool)) []Finding {
	if s == nil || walkFn == nil {
		return nil
	}

	findings := make([]Finding, 0)
	walkFn(func(path string, content []byte) bool {
		if lockfile.DetectFormat(path) != lockfile.FormatUnknown {
			if lf, err := lockfile.Parse(path, content); err == nil {
				findings = append(findings, s.ScanDependencies(lf)...)
			}
		}
		findings = append(findings, s.ScanContent(path, content)...)
		return true
	})

	deduped := deduplicateFindings(findings)
	sortFindings(deduped)
	return deduped
}

func (s *Scanner) advisoriesForDependency(dep lockfile.Dependency) []advisory.Advisory {
	if s.idx != nil {
		return s.idx.Lookup(dep.Ecosystem, dep.Name)
	}
	return s.advisories
}

func (s *Scanner) shouldReport(severity advisory.Severity) bool {
	return severity >= s.opts.MinSeverity
}

func (s *Scanner) newContentFinding(path, text string, adv advisory.Advisory, findingType FindingType, offset int, source string) Finding {
	line, snippet := extractLineAndSnippet(text, offset)
	return Finding{
		ID:          contentFindingID(adv.ID, path, line, findingType),
		AdvisoryID:  adv.ID,
		Severity:    adv.Severity,
		CVSS:        adv.CVSS,
		Title:       adv.Title,
		Description: adv.Description,
		Type:        findingType,
		FilePath:    path,
		Line:        line,
		Snippet:     snippet,
		FixTemplate: selectFixTemplate(adv, ""),
		FoundAt:     time.Now(),
		Source:      source,
	}
}

func deduplicateFindings(findings []Finding) []Finding {
	seen := make(map[string]Finding, len(findings))
	order := make([]string, 0, len(findings))
	for _, finding := range findings {
		key := finding.AdvisoryID + "|" + finding.Package + "|" + finding.FilePath
		existing, ok := seen[key]
		if ok {
			if finding.Line != 0 && (existing.Line == 0 || finding.Line < existing.Line) {
				seen[key] = finding
			}
			continue
		}

		seen[key] = finding
		order = append(order, key)
	}

	result := make([]Finding, 0, len(order))
	for _, key := range order {
		result = append(result, seen[key])
	}
	return result
}

func extractLineAndSnippet(text string, offset int) (int, string) {
	if offset < 0 || offset > len(text) {
		return 0, ""
	}

	line := 1
	for i := 0; i < offset; i++ {
		if text[i] == '\n' {
			line++
		}
	}

	lines := strings.Split(text, "\n")
	start := line - 3
	if start < 0 {
		start = 0
	}
	end := line + 2
	if end > len(lines) {
		end = len(lines)
	}

	return line, strings.Join(lines[start:end], "\n")
}

func findAllSubstrings(text, query string) []int {
	if query == "" {
		return nil
	}

	matches := make([]int, 0)
	for offset := 0; offset < len(text); {
		idx := strings.Index(text[offset:], query)
		if idx < 0 {
			break
		}
		match := offset + idx
		matches = append(matches, match)
		offset = match + len(query)
	}
	return matches
}

func selectFixTemplate(adv advisory.Advisory, pkg string) *advisory.FixTemplate {
	for i := range adv.FixTemplates {
		template := &adv.FixTemplates[i]
		if template.Package == "" || template.Package == pkg {
			return template
		}
	}
	return nil
}

func dependencyFindingID(advisoryID, pkg string) string {
	return "f-" + sanitizeIDPart(advisoryID) + "-" + sanitizeIDPart(pkg)
}

func contentFindingID(advisoryID, path string, line int, findingType FindingType) string {
	return "f-" + sanitizeIDPart(advisoryID) + "-" + sanitizeIDPart(path) + "-" + findingType.String() + "-" + strconv.Itoa(line)
}

func sanitizeIDPart(value string) string {
	replacer := strings.NewReplacer("/", "-", "\\", "-", " ", "-", ":", "-", "@", "-")
	clean := replacer.Replace(strings.TrimSpace(value))
	clean = strings.Trim(clean, "-")
	if clean == "" {
		return "unknown"
	}
	return clean
}

func packageKey(ecosystem, name string) string {
	return ecosystem + ":" + name
}

func sortFindings(findings []Finding) {
	sort.Slice(findings, func(i, j int) bool {
		left := findings[i]
		right := findings[j]
		if left.Severity != right.Severity {
			return left.Severity > right.Severity
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
		return left.Type < right.Type
	})
}
