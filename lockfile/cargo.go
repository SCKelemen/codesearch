package lockfile

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

func parseCargo(content []byte) ([]Dependency, error) {
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
		current.Ecosystem = "cargo"
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
		case strings.HasPrefix(line, "checksum = "):
			current.Integrity = stripQuotes(strings.TrimSpace(strings.TrimPrefix(line, "checksum = ")))
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse Cargo.lock: %w", err)
	}
	flush()

	return deps, nil
}
