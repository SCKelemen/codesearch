package lockfile

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

func parsePNPM(content []byte) ([]Dependency, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	var (
		deps       []Dependency
		current    *Dependency
		inPackages bool
	)

	flush := func() {
		if current == nil || current.Name == "" || current.Version == "" {
			return
		}
		deps = append(deps, *current)
	}

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "packages:" {
			inPackages = true
			continue
		}
		if !inPackages {
			continue
		}
		if trimmed != "" && !strings.HasPrefix(line, " ") {
			flush()
			current = nil
			inPackages = false
			continue
		}
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "    ") && strings.HasSuffix(trimmed, ":") {
			flush()
			name, version, err := parsePNPMPackageKey(trimmed)
			if err != nil {
				return nil, err
			}
			current = &Dependency{Name: name, Version: version, Ecosystem: "npm"}
			continue
		}
		if current == nil {
			continue
		}
		if trimmed == "dev: true" || trimmed == "dev: true," {
			current.Dev = true
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse pnpm lockfile: %w", err)
	}
	flush()

	return deps, nil
}

func parsePNPMPackageKey(line string) (string, string, error) {
	key := strings.TrimSuffix(strings.TrimSpace(line), ":")
	key = stripQuotes(key)
	key = strings.TrimPrefix(key, "/")
	if idx := strings.Index(key, "("); idx >= 0 {
		key = key[:idx]
	}
	at := strings.LastIndex(key, "@")
	if at <= 0 || at == len(key)-1 {
		return "", "", fmt.Errorf("invalid pnpm package key: %q", line)
	}
	return key[:at], key[at+1:], nil
}
