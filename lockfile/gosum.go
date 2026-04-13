package lockfile

import (
	"bufio"
	"bytes"
	"fmt"
	"strings"
)

func parseGoSum(content []byte) ([]Dependency, error) {
	scanner := bufio.NewScanner(bytes.NewReader(content))
	seen := map[string]bool{}
	var deps []Dependency

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			continue
		}
		name := fields[0]
		version := fields[1]
		if strings.HasSuffix(version, "/go.mod") {
			continue
		}
		key := name + "@" + version
		if seen[key] {
			continue
		}
		seen[key] = true
		deps = append(deps, Dependency{
			Name:      name,
			Version:   version,
			Ecosystem: "go",
			Integrity: fields[2],
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse go.sum: %w", err)
	}

	return deps, nil
}
