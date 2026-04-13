package lockfile

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

func parseYarn(content []byte) ([]Dependency, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	var (
		deps    []Dependency
		current *Dependency
	)

	flush := func() {
		if current == nil || current.Name == "" || current.Version == "" {
			return
		}
		deps = append(deps, *current)
	}

	for scanner.Scan() {
		line := scanner.Text()
		if isYarnEntryStart(line) {
			flush()
			name, err := parseYarnEntryName(line)
			if err != nil {
				return nil, err
			}
			current = &Dependency{Name: name, Ecosystem: "npm"}
			continue
		}
		if current == nil {
			continue
		}
		if strings.HasPrefix(line, "  version ") {
			current.Version = stripQuotes(strings.TrimSpace(strings.TrimPrefix(line, "  version ")))
			continue
		}
		if strings.HasPrefix(line, "  integrity ") {
			current.Integrity = stripQuotes(strings.TrimSpace(strings.TrimPrefix(line, "  integrity ")))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse yarn lockfile: %w", err)
	}
	flush()

	return deps, nil
}

func isYarnEntryStart(line string) bool {
	if strings.TrimSpace(line) == "" {
		return false
	}
	if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
		return false
	}
	return strings.HasSuffix(strings.TrimSpace(line), ":")
}

func parseYarnEntryName(line string) (string, error) {
	line = strings.TrimSpace(strings.TrimSuffix(line, ":"))
	parts := strings.Split(line, ",")
	if len(parts) == 0 {
		return "", fmt.Errorf("invalid yarn entry: %q", line)
	}

	selector := strings.TrimSpace(parts[0])
	selector = stripQuotes(selector)
	at := strings.LastIndex(selector, "@")
	if at <= 0 {
		return "", fmt.Errorf("invalid yarn selector: %q", selector)
	}
	return selector[:at], nil
}
