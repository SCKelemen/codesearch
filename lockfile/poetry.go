package lockfile

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

func parsePoetry(content []byte) ([]Dependency, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	var (
		deps    []Dependency
		current Dependency
		inBlock bool
	)

	flush := func() {
		if !inBlock || current.Name == "" || current.Version == "" {
			return
		}
		current.Ecosystem = "pypi"
		deps = append(deps, current)
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "[[package]]" {
			flush()
			current = Dependency{}
			inBlock = true
			continue
		}
		if !inBlock {
			continue
		}
		switch {
		case strings.HasPrefix(line, "name = "):
			current.Name = stripQuotes(strings.TrimSpace(strings.TrimPrefix(line, "name = ")))
		case strings.HasPrefix(line, "version = "):
			current.Version = stripQuotes(strings.TrimSpace(strings.TrimPrefix(line, "version = ")))
		case line == "optional = true":
			current.Dev = true
		case strings.HasPrefix(line, "category = "):
			if stripQuotes(strings.TrimSpace(strings.TrimPrefix(line, "category = "))) == "dev" {
				current.Dev = true
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse poetry.lock: %w", err)
	}
	flush()

	return deps, nil
}
