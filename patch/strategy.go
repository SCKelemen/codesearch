package patch

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/SCKelemen/codesearch/advisory"
	"github.com/SCKelemen/codesearch/lockfile"
	"github.com/SCKelemen/codesearch/vuln"
)

// Strategy generates patches for a specific fix type.
type Strategy interface {
	// CanFix reports whether this strategy can handle the given finding.
	CanFix(f vuln.Finding) bool
	// Fix generates a patch for the finding. content returns file content by path.
	Fix(f vuln.Finding, content func(path string) ([]byte, error)) (*Patch, error)
}

// DependencyBumpStrategy fixes vulnerable dependencies by updating version strings.
type DependencyBumpStrategy struct{}

// CanFix reports whether the finding is a fixable dependency finding.
func (s *DependencyBumpStrategy) CanFix(f vuln.Finding) bool {
	return f.Type == vuln.FindingDependency && f.Fixable && f.FixedIn != ""
}

// Fix generates a dependency version bump patch.
func (s *DependencyBumpStrategy) Fix(f vuln.Finding, content func(path string) ([]byte, error)) (*Patch, error) {
	path := findingPath(f)
	if path == "" {
		return nil, fmt.Errorf("finding %s has no file path", f.ID)
	}

	data, err := content(path)
	if err != nil {
		return nil, err
	}

	format := detectManifestFormat(path)
	var hunk Hunk
	var commands []string
	text := string(data)

	switch format {
	case lockfile.FormatNPM:
		hunk, err = replacePackageJSONDependency(text, f.Package, f.Version, f.FixedIn)
		commands = []string{"npm install"}
	case lockfile.FormatCargo:
		hunk, err = replaceCargoDependency(text, f.Package, f.Version, f.FixedIn)
		commands = []string{fmt.Sprintf("cargo update -p %s --precise %s", f.Package, f.FixedIn)}
	case lockfile.FormatPip:
		hunk, err = replaceRequirementsDependency(text, f.Package, f.Version, f.FixedIn)
		commands = []string{"pip install -r requirements.txt"}
	case lockfile.FormatGoSum:
		hunk, err = replaceGoModDependency(text, f.Package, f.Version, f.FixedIn)
		commands = []string{"go mod tidy"}
	default:
		return nil, fmt.Errorf("unsupported dependency manifest: %s", filepath.Base(path))
	}
	if err != nil {
		return nil, err
	}

	return &Patch{
		FindingID:   f.ID,
		AdvisoryID:  f.AdvisoryID,
		Description: dependencyPatchDescription(f),
		Files: []FilePatch{{
			Path:  path,
			Hunks: []Hunk{hunk},
		}},
		Commands:   commands,
		Confidence: ConfidenceHigh,
	}, nil
}

// PatternReplaceStrategy fixes code patterns by replacing vulnerable API usage.
type PatternReplaceStrategy struct{}

// CanFix reports whether the finding has an API migration template.
func (s *PatternReplaceStrategy) CanFix(f vuln.Finding) bool {
	return f.FixTemplate != nil && f.FixTemplate.Type == advisory.FixAPIMigration
}

// Fix generates a pattern replacement patch from the finding template.
func (s *PatternReplaceStrategy) Fix(f vuln.Finding, content func(path string) ([]byte, error)) (*Patch, error) {
	template := f.FixTemplate
	if template == nil {
		return nil, fmt.Errorf("finding %s has no fix template", f.ID)
	}
	if template.Pattern == "" {
		return nil, fmt.Errorf("finding %s template is missing a pattern", f.ID)
	}

	path := findingPath(f)
	if path == "" {
		return nil, fmt.Errorf("finding %s has no file path", f.ID)
	}

	data, err := content(path)
	if err != nil {
		return nil, err
	}

	hunk, err := replaceSnippet(string(data), template.Pattern, template.Replacement)
	if err != nil {
		return nil, err
	}

	description := template.Description
	if description == "" {
		description = f.Description
	}
	if description == "" {
		description = fmt.Sprintf("Migrate %s usage to a safer API", f.Package)
	}

	return &Patch{
		FindingID:   f.ID,
		AdvisoryID:  f.AdvisoryID,
		Description: description,
		Files: []FilePatch{{
			Path:  path,
			Hunks: []Hunk{hunk},
		}},
		Commands:    append([]string(nil), template.Commands...),
		Confidence:  ConfidenceMedium,
		AgentAssist: template.AgentAssist,
	}, nil
}

func findingPath(f vuln.Finding) string {
	if f.FilePath != "" {
		return f.FilePath
	}
	if f.FixTemplate != nil && len(f.FixTemplate.Files) > 0 {
		return f.FixTemplate.Files[0]
	}
	return ""
}

func dependencyPatchDescription(f vuln.Finding) string {
	if f.Description != "" {
		return f.Description
	}
	if f.Package == "" {
		return fmt.Sprintf("Update vulnerable dependency to %s", f.FixedIn)
	}
	if f.Version != "" {
		return fmt.Sprintf("Update %s from %s to %s", f.Package, f.Version, f.FixedIn)
	}
	return fmt.Sprintf("Update %s to %s", f.Package, f.FixedIn)
}

func detectManifestFormat(path string) lockfile.Format {
	switch filepath.Base(path) {
	case "package.json", "package-lock.json":
		return lockfile.FormatNPM
	case "Cargo.toml", "Cargo.lock":
		return lockfile.FormatCargo
	case "requirements.txt":
		return lockfile.FormatPip
	case "go.mod", "go.sum":
		return lockfile.FormatGoSum
	default:
		return lockfile.DetectFormat(path)
	}
}

func replacePackageJSONDependency(content, pkg, currentVersion, fixedIn string) (Hunk, error) {
	re := regexp.MustCompile(`^(\s*"` + regexp.QuoteMeta(pkg) + `"\s*:\s*")(.*?)("\s*,?\s*)$`)
	return replaceMatchingLine(content, re, currentVersion, fixedIn)
}

func replaceRequirementsDependency(content, pkg, currentVersion, fixedIn string) (Hunk, error) {
	re := regexp.MustCompile(`^(\s*` + regexp.QuoteMeta(pkg) + `\s*==\s*)(\S+)(\s*)$`)
	return replaceMatchingLine(content, re, currentVersion, fixedIn)
}

func replaceGoModDependency(content, pkg, currentVersion, fixedIn string) (Hunk, error) {
	re := regexp.MustCompile(`^(\s*` + regexp.QuoteMeta(pkg) + `\s+)(\S+)(\s*)$`)
	return replaceMatchingLine(content, re, currentVersion, fixedIn)
}

func replaceCargoDependency(content, pkg, currentVersion, fixedIn string) (Hunk, error) {
	lineRE := regexp.MustCompile(`^(\s*` + regexp.QuoteMeta(pkg) + `\s*=\s*")(.*?)(".*)$`)
	if hunk, err := replaceMatchingLine(content, lineRE, currentVersion, fixedIn); err == nil {
		return hunk, nil
	}

	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	nameLine := fmt.Sprintf(`name = "%s"`, pkg)
	for i := 0; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) != nameLine {
			continue
		}
		for j := i + 1; j < len(lines); j++ {
			if strings.TrimSpace(lines[j]) == "" {
				break
			}
			trimmed := strings.TrimSpace(lines[j])
			if !strings.HasPrefix(trimmed, `version = "`) || !strings.HasSuffix(trimmed, `"`) {
				continue
			}
			version := strings.TrimSuffix(strings.TrimPrefix(trimmed, `version = "`), `"`)
			if currentVersion != "" && version != currentVersion {
				continue
			}
			indent := lines[j][:len(lines[j])-len(strings.TrimLeft(lines[j], " \t"))]
			newLine := indent + `version = "` + fixedIn + `"`
			return lineReplacementHunk(j+1, lines[j], newLine), nil
		}
	}

	return Hunk{}, fmt.Errorf("package %q not found in Cargo.toml", pkg)
}

func replaceMatchingLine(content string, re *regexp.Regexp, currentVersion, fixedIn string) (Hunk, error) {
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	for i, line := range lines {
		matches := re.FindStringSubmatch(line)
		if matches == nil {
			continue
		}
		if currentVersion != "" && matches[2] != currentVersion {
			continue
		}
		newLine := matches[1] + fixedIn + matches[3]
		return lineReplacementHunk(i+1, line, newLine), nil
	}
	return Hunk{}, fmt.Errorf("matching version string not found")
}

func lineReplacementHunk(lineNumber int, oldLine, newLine string) Hunk {
	return Hunk{
		OldStart: lineNumber,
		OldCount: 1,
		NewStart: lineNumber,
		NewCount: 1,
		Lines: []Line{
			{Op: OpDelete, Content: oldLine},
			{Op: OpAdd, Content: newLine},
		},
	}
}

func replaceSnippet(content, oldSnippet, newSnippet string) (Hunk, error) {
	index := strings.Index(content, oldSnippet)
	if index < 0 {
		return Hunk{}, fmt.Errorf("pattern not found")
	}

	oldLines := splitLines(oldSnippet)
	newLines := splitLines(newSnippet)
	startLine := 1 + strings.Count(content[:index], "\n")
	lines := make([]Line, 0, len(oldLines)+len(newLines))
	for _, line := range oldLines {
		lines = append(lines, Line{Op: OpDelete, Content: line})
	}
	for _, line := range newLines {
		lines = append(lines, Line{Op: OpAdd, Content: line})
	}

	return Hunk{
		OldStart: startLine,
		OldCount: len(oldLines),
		NewStart: startLine,
		NewCount: len(newLines),
		Lines:    lines,
	}, nil
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}
