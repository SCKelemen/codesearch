package patch

import (
	"bufio"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var hunkHeaderRE = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

// FormatUnifiedDiff formats a FilePatch as a unified diff string.
func FormatUnifiedDiff(fp FilePatch) string {
	var b strings.Builder

	oldPath := "a/" + fp.Path
	newPath := "b/" + fp.Path
	if fp.Created {
		oldPath = "/dev/null"
	}
	if fp.Deleted {
		newPath = "/dev/null"
	}

	fmt.Fprintf(&b, "--- %s\n", oldPath)
	fmt.Fprintf(&b, "+++ %s\n", newPath)
	for _, hunk := range fp.Hunks {
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", hunk.OldStart, hunk.OldCount, hunk.NewStart, hunk.NewCount)
		for _, line := range hunk.Lines {
			b.WriteByte(opPrefix(line.Op))
			b.WriteString(line.Content)
			b.WriteByte('\n')
		}
	}

	return b.String()
}

// FormatPatch formats a complete Patch as a multi-file unified diff.
func FormatPatch(p Patch) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# Advisory: %s\n", p.AdvisoryID)
	fmt.Fprintf(&b, "# Description: %s\n", p.Description)
	for _, fp := range p.Files {
		b.WriteByte('\n')
		b.WriteString(FormatUnifiedDiff(fp))
	}

	return b.String()
}

// ParseUnifiedDiff parses a unified diff string back into a FilePatch.
func ParseUnifiedDiff(diff string) (FilePatch, error) {
	var (
		fp         FilePatch
		current    *Hunk
		haveOldHdr bool
		haveNewHdr bool
	)

	scanner := bufio.NewScanner(strings.NewReader(diff))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		switch {
		case strings.HasPrefix(line, "--- "):
			haveOldHdr = true
			path := strings.TrimSpace(strings.TrimPrefix(line, "--- "))
			if path == "/dev/null" {
				fp.Created = true
			} else {
				fp.Path = trimDiffPath(path)
			}
		case strings.HasPrefix(line, "+++ "):
			haveNewHdr = true
			path := strings.TrimSpace(strings.TrimPrefix(line, "+++ "))
			if path == "/dev/null" {
				fp.Deleted = true
			} else if fp.Path == "" {
				fp.Path = trimDiffPath(path)
			}
		case strings.HasPrefix(line, "@@ "):
			hunk, err := parseHunkHeader(line)
			if err != nil {
				return FilePatch{}, err
			}
			fp.Hunks = append(fp.Hunks, hunk)
			current = &fp.Hunks[len(fp.Hunks)-1]
		case strings.HasPrefix(line, `\ No newline at end of file`):
			continue
		default:
			if current == nil {
				return FilePatch{}, fmt.Errorf("diff line encountered before a hunk: %q", line)
			}
			op, content, err := parseDiffLine(line)
			if err != nil {
				return FilePatch{}, err
			}
			current.Lines = append(current.Lines, Line{Op: op, Content: content})
		}
	}
	if err := scanner.Err(); err != nil {
		return FilePatch{}, err
	}
	if !haveOldHdr || !haveNewHdr {
		return FilePatch{}, fmt.Errorf("diff is missing file headers")
	}
	if fp.Path == "" {
		return FilePatch{}, fmt.Errorf("diff is missing a file path")
	}

	return fp, nil
}

func opPrefix(op Op) byte {
	switch op {
	case OpAdd:
		return '+'
	case OpDelete:
		return '-'
	default:
		return ' '
	}
}

func trimDiffPath(path string) string {
	path = strings.TrimPrefix(path, "a/")
	path = strings.TrimPrefix(path, "b/")
	return path
}

func parseHunkHeader(line string) (Hunk, error) {
	matches := hunkHeaderRE.FindStringSubmatch(line)
	if matches == nil {
		return Hunk{}, fmt.Errorf("invalid hunk header: %q", line)
	}

	oldStart, err := strconv.Atoi(matches[1])
	if err != nil {
		return Hunk{}, err
	}
	oldCount := 1
	if matches[2] != "" {
		oldCount, err = strconv.Atoi(matches[2])
		if err != nil {
			return Hunk{}, err
		}
	}
	newStart, err := strconv.Atoi(matches[3])
	if err != nil {
		return Hunk{}, err
	}
	newCount := 1
	if matches[4] != "" {
		newCount, err = strconv.Atoi(matches[4])
		if err != nil {
			return Hunk{}, err
		}
	}

	return Hunk{OldStart: oldStart, OldCount: oldCount, NewStart: newStart, NewCount: newCount}, nil
}

func parseDiffLine(line string) (Op, string, error) {
	if line == "" {
		return 0, "", fmt.Errorf("invalid empty diff line")
	}

	switch line[0] {
	case ' ':
		return OpContext, line[1:], nil
	case '+':
		return OpAdd, line[1:], nil
	case '-':
		return OpDelete, line[1:], nil
	default:
		return 0, "", fmt.Errorf("invalid diff line prefix: %q", line)
	}
}
