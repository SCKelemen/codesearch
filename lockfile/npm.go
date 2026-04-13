package lockfile

import (
	"encoding/json"
	"fmt"
	"strings"
)

func parseNPM(content []byte) ([]Dependency, error) {
	var data struct {
		Packages map[string]struct {
			Version      string                 `json:"version"`
			Resolved     string                 `json:"resolved"`
			Integrity    string                 `json:"integrity"`
			Dev          bool                   `json:"dev"`
			Dependencies map[string]interface{} `json:"dependencies"`
		} `json:"packages"`
		Dependencies map[string]interface{} `json:"dependencies"`
	}

	if err := json.Unmarshal(content, &data); err != nil {
		return nil, fmt.Errorf("parse npm lockfile: %w", err)
	}

	rootDeps := map[string]bool{}
	if root, ok := data.Packages[""]; ok {
		for name := range root.Dependencies {
			rootDeps[name] = true
		}
	}
	for name := range data.Dependencies {
		rootDeps[name] = true
	}

	deps := make([]Dependency, 0, len(data.Packages))
	for key, pkg := range data.Packages {
		if key == "" {
			continue
		}
		name := key
		if strings.HasPrefix(name, "node_modules/") {
			name = strings.TrimPrefix(name, "node_modules/")
		}
		deps = append(deps, Dependency{
			Name:      name,
			Version:   pkg.Version,
			Ecosystem: "npm",
			Integrity: pkg.Integrity,
			Direct:    rootDeps[name],
			Dev:       pkg.Dev,
		})
	}

	return deps, nil
}
