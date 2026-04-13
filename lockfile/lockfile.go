package lockfile

import (
	"fmt"
	"path/filepath"
)

// Dependency is a resolved package from a lockfile.
type Dependency struct {
	Name      string // package name (e.g. "lodash", "serde")
	Version   string // resolved version (e.g. "4.17.21")
	Ecosystem string // "npm", "pypi", "go", "cargo", "rubygems"
	Integrity string // integrity hash if available (e.g. "sha512-...")
	Direct    bool   // true if a direct (non-transitive) dependency
	Dev       bool   // true if a dev/test dependency
}

// Lockfile is a parsed lockfile with its dependencies.
type Lockfile struct {
	Path         string       // file path
	Format       Format       // detected format
	Dependencies []Dependency // all resolved dependencies
}

type Format string

const (
	FormatNPM     Format = "npm"     // package-lock.json
	FormatYarn    Format = "yarn"    // yarn.lock
	FormatPNPM    Format = "pnpm"    // pnpm-lock.yaml
	FormatGoSum   Format = "go.sum"  // go.sum
	FormatCargo   Format = "cargo"   // Cargo.lock
	FormatPip     Format = "pip"     // requirements.txt
	FormatGemfile Format = "gemfile" // Gemfile.lock
	FormatPoetry  Format = "poetry"  // poetry.lock
	FormatUnknown Format = "unknown"
)

// Parse reads a lockfile and returns its dependencies.
func Parse(path string, content []byte) (*Lockfile, error) {
	format := DetectFormat(filepath.Base(path))
	if format == FormatUnknown {
		return nil, fmt.Errorf("unknown lockfile format: %s", filepath.Base(path))
	}

	var (
		deps []Dependency
		err  error
	)

	switch format {
	case FormatNPM:
		deps, err = parseNPM(content)
	case FormatYarn:
		deps, err = parseYarn(content)
	case FormatPNPM:
		deps, err = parsePNPM(content)
	case FormatGoSum:
		deps, err = parseGoSum(content)
	case FormatCargo:
		deps, err = parseCargo(content)
	case FormatPip:
		deps, err = parsePip(content)
	case FormatGemfile:
		deps, err = parseGemfile(content)
	case FormatPoetry:
		deps, err = parsePoetry(content)
	default:
		return nil, fmt.Errorf("unsupported lockfile format: %s", format)
	}
	if err != nil {
		return nil, err
	}

	return &Lockfile{
		Path:         path,
		Format:       format,
		Dependencies: deps,
	}, nil
}

// DetectFormat determines the lockfile format from a filename.
func DetectFormat(filename string) Format {
	switch filepath.Base(filename) {
	case "package-lock.json":
		return FormatNPM
	case "yarn.lock":
		return FormatYarn
	case "pnpm-lock.yaml":
		return FormatPNPM
	case "go.sum":
		return FormatGoSum
	case "Cargo.lock":
		return FormatCargo
	case "requirements.txt":
		return FormatPip
	case "Gemfile.lock":
		return FormatGemfile
	case "poetry.lock":
		return FormatPoetry
	default:
		return FormatUnknown
	}
}

func stripQuotes(s string) string {
	if len(s) >= 2 {
		if s[0] == '\'' && s[len(s)-1] == '\'' {
			return s[1 : len(s)-1]
		}
		if s[0] == '"' && s[len(s)-1] == '"' {
			return s[1 : len(s)-1]
		}
	}
	return s
}
