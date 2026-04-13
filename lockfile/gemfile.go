package lockfile

import (
	"bufio"
	"bytes"
	"fmt"
	"regexp"
	"strings"
)

var gemSpecPattern = regexp.MustCompile(`^\s{4}([^\s(]+) \(([^)]+)\)$`)

func parseGemfile(content []byte) ([]Dependency, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	var (
		deps    []Dependency
		inGem   bool
		inSpecs bool
	)

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		switch trimmed {
		case "GEM":
			inGem = true
			inSpecs = false
			continue
		case "specs:":
			if inGem && strings.HasPrefix(line, "  ") {
				inSpecs = true
			}
			continue
		case "PLATFORMS", "DEPENDENCIES", "BUNDLED WITH":
			if inGem {
				inGem = false
				inSpecs = false
			}
			continue
		}

		if !inGem || !inSpecs {
			continue
		}
		match := gemSpecPattern.FindStringSubmatch(line)
		if len(match) != 3 {
			continue
		}
		deps = append(deps, Dependency{
			Name:      match[1],
			Version:   match[2],
			Ecosystem: "rubygems",
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse Gemfile.lock: %w", err)
	}

	return deps, nil
}
