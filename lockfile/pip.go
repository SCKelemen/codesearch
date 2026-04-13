package lockfile

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

var pipConstraintPattern = regexp.MustCompile(`^([a-zA-Z0-9._-]+)\s*(==|>=|<=|!=|~=|>|<)\s*(.+)$`)

func parsePip(content []byte) ([]Dependency, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	var deps []Dependency

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "-r") ||
			strings.HasPrefix(line, "--requirement") ||
			strings.HasPrefix(line, "-e") ||
			strings.HasPrefix(line, "--editable") ||
			strings.HasPrefix(line, "--index-url") ||
			strings.HasPrefix(line, "--extra-index-url") ||
			strings.HasPrefix(line, "--find-links") ||
			strings.HasPrefix(line, "--trusted-host") ||
			strings.HasPrefix(line, "-i ") {
			continue
		}

		match := pipConstraintPattern.FindStringSubmatch(line)
		if len(match) != 4 {
			continue
		}
		version := match[2] + match[3]
		if match[2] == "==" {
			version = match[3]
		}
		deps = append(deps, Dependency{
			Name:      match[1],
			Version:   version,
			Ecosystem: "pypi",
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse requirements.txt: %w", err)
	}

	return deps, nil
}
