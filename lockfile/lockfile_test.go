package lockfile

import (
	"reflect"
	"testing"
)

const npmLockSnippet = `{
  "name": "example",
  "lockfileVersion": 3,
  "packages": {
    "": {
      "name": "example",
      "dependencies": {
        "lodash": "^4.17.21",
        "chalk": "^5.3.0"
      }
    },
    "node_modules/lodash": {
      "version": "4.17.21",
      "integrity": "sha512-lodash"
    },
    "node_modules/chalk": {
      "version": "5.3.0",
      "integrity": "sha512-chalk",
      "dev": true
    },
    "node_modules/ansi-styles": {
      "version": "6.2.1",
      "integrity": "sha512-ansi"
    }
  }
}`

const yarnLockSnippet = `lodash@^4.17.21:
  version "4.17.21"
  integrity sha512-lodash

"@types/node@^20.0.0":
  version "20.11.30"
  integrity sha512-types-node
`

const pnpmLockSnippet = `lockfileVersion: '9.0'
packages:
  /lodash@4.17.21:
    resolution: {integrity: sha512-lodash}
    dev: false
  /chalk@5.3.0:
    resolution: {integrity: sha512-chalk}
    dev: true
  /@types/node@20.11.30:
    resolution: {integrity: sha512-types-node}
`

const goSumSnippet = `github.com/google/go-cmp v0.6.0 h1:cmphash
github.com/google/go-cmp v0.6.0/go.mod h1:cmpmodhash
golang.org/x/text v0.14.0 h1:texthash
golang.org/x/text v0.14.0/go.mod h1:textmodhash
github.com/stretchr/testify v1.9.0 h1:testifyhash
`

const cargoLockSnippet = `version = 3

[[package]]
name = "serde"
version = "1.0.203"
checksum = "serde-checksum"

[[package]]
name = "toml"
version = "0.8.14"
checksum = "toml-checksum"
`

const requirementsSnippet = `# Base requirements
requests==2.31.0
urllib3>=2.2.1
charset-normalizer~=3.3
-r dev-requirements.txt
--index-url https://pypi.org/simple
`

const gemfileLockSnippet = `GEM
  remote: https://rubygems.org/
  specs:
    rake (13.2.1)
    rspec (3.13.0)
      diff-lcs (>= 1.2.0, < 2.0)

PLATFORMS
  ruby

DEPENDENCIES
  rake
  rspec
`

const poetryLockSnippet = `[[package]]
name = "requests"
version = "2.31.0"
category = "main"
optional = false

[[package]]
name = "pytest"
version = "8.2.2"
category = "dev"
optional = true
`

func TestDetectFormat(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		filename string
		want     Format
	}{
		{name: "npm", filename: "package-lock.json", want: FormatNPM},
		{name: "yarn", filename: "yarn.lock", want: FormatYarn},
		{name: "pnpm", filename: "pnpm-lock.yaml", want: FormatPNPM},
		{name: "go", filename: "go.sum", want: FormatGoSum},
		{name: "cargo", filename: "Cargo.lock", want: FormatCargo},
		{name: "pip", filename: "requirements.txt", want: FormatPip},
		{name: "gemfile", filename: "Gemfile.lock", want: FormatGemfile},
		{name: "poetry", filename: "poetry.lock", want: FormatPoetry},
		{name: "unknown", filename: "unknown.txt", want: FormatUnknown},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := DetectFormat(tc.filename); got != tc.want {
				t.Fatalf("DetectFormat(%q) = %q, want %q", tc.filename, got, tc.want)
			}
		})
	}
}

func TestParse(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		path    string
		content string
		want    Format
	}{
		{name: "npm", path: "package-lock.json", content: npmLockSnippet, want: FormatNPM},
		{name: "yarn", path: "yarn.lock", content: yarnLockSnippet, want: FormatYarn},
		{name: "pnpm", path: "pnpm-lock.yaml", content: pnpmLockSnippet, want: FormatPNPM},
		{name: "go", path: "go.sum", content: goSumSnippet, want: FormatGoSum},
		{name: "cargo", path: "Cargo.lock", content: cargoLockSnippet, want: FormatCargo},
		{name: "pip", path: "requirements.txt", content: requirementsSnippet, want: FormatPip},
		{name: "gemfile", path: "Gemfile.lock", content: gemfileLockSnippet, want: FormatGemfile},
		{name: "poetry", path: "poetry.lock", content: poetryLockSnippet, want: FormatPoetry},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			lf, err := Parse(tc.path, []byte(tc.content))
			if err != nil {
				t.Fatalf("Parse(%q) error = %v", tc.path, err)
			}
			if lf.Format != tc.want {
				t.Fatalf("Parse(%q) format = %q, want %q", tc.path, lf.Format, tc.want)
			}
			if lf.Path != tc.path {
				t.Fatalf("Parse(%q) path = %q, want %q", tc.path, lf.Path, tc.path)
			}
		})
	}

	if _, err := Parse("unknown.txt", []byte("hello")); err == nil {
		t.Fatal("Parse unknown format returned nil error, want error")
	}
}

func TestParseNPM(t *testing.T) {
	t.Parallel()

	deps, err := parseNPM([]byte(npmLockSnippet))
	if err != nil {
		t.Fatalf("parseNPM error = %v", err)
	}
	if len(deps) != 3 {
		t.Fatalf("len(deps) = %d, want 3", len(deps))
	}

	got := map[string]Dependency{}
	for _, dep := range deps {
		got[dep.Name] = dep
	}

	if got["lodash"].Version != "4.17.21" || !got["lodash"].Direct {
		t.Fatalf("lodash = %+v, want version 4.17.21 and direct true", got["lodash"])
	}
	if got["chalk"].Version != "5.3.0" || !got["chalk"].Dev || !got["chalk"].Direct {
		t.Fatalf("chalk = %+v, want version 5.3.0, dev true, direct true", got["chalk"])
	}
	if got["ansi-styles"].Version != "6.2.1" || got["ansi-styles"].Direct {
		t.Fatalf("ansi-styles = %+v, want version 6.2.1 and direct false", got["ansi-styles"])
	}
}

func TestParseYarn(t *testing.T) {
	t.Parallel()

	deps, err := parseYarn([]byte(yarnLockSnippet))
	if err != nil {
		t.Fatalf("parseYarn error = %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("len(deps) = %d, want 2", len(deps))
	}

	want := map[string]Dependency{
		"lodash":      {Name: "lodash", Version: "4.17.21", Ecosystem: "npm", Integrity: "sha512-lodash"},
		"@types/node": {Name: "@types/node", Version: "20.11.30", Ecosystem: "npm", Integrity: "sha512-types-node"},
	}
	for _, dep := range deps {
		if !reflect.DeepEqual(dep, want[dep.Name]) {
			t.Fatalf("dep %q = %+v, want %+v", dep.Name, dep, want[dep.Name])
		}
	}
}

func TestParsePNPM(t *testing.T) {
	t.Parallel()

	deps, err := parsePNPM([]byte(pnpmLockSnippet))
	if err != nil {
		t.Fatalf("parsePNPM error = %v", err)
	}
	if len(deps) != 3 {
		t.Fatalf("len(deps) = %d, want 3", len(deps))
	}

	got := map[string]Dependency{}
	for _, dep := range deps {
		got[dep.Name] = dep
	}
	if got["lodash"].Version != "4.17.21" || got["lodash"].Dev {
		t.Fatalf("lodash = %+v, want version 4.17.21 and dev false", got["lodash"])
	}
	if got["chalk"].Version != "5.3.0" || !got["chalk"].Dev {
		t.Fatalf("chalk = %+v, want version 5.3.0 and dev true", got["chalk"])
	}
	if got["@types/node"].Version != "20.11.30" {
		t.Fatalf("@types/node = %+v, want version 20.11.30", got["@types/node"])
	}
}

func TestParseGoSum(t *testing.T) {
	t.Parallel()

	deps, err := parseGoSum([]byte(goSumSnippet))
	if err != nil {
		t.Fatalf("parseGoSum error = %v", err)
	}
	if len(deps) != 3 {
		t.Fatalf("len(deps) = %d, want 3", len(deps))
	}

	got := map[string]Dependency{}
	for _, dep := range deps {
		got[dep.Name] = dep
	}
	if got["github.com/google/go-cmp"].Version != "v0.6.0" {
		t.Fatalf("go-cmp = %+v, want version v0.6.0", got["github.com/google/go-cmp"])
	}
	if got["golang.org/x/text"].Version != "v0.14.0" {
		t.Fatalf("x/text = %+v, want version v0.14.0", got["golang.org/x/text"])
	}
	if got["github.com/stretchr/testify"].Version != "v1.9.0" {
		t.Fatalf("testify = %+v, want version v1.9.0", got["github.com/stretchr/testify"])
	}
}

func TestParseCargo(t *testing.T) {
	t.Parallel()

	deps, err := parseCargo([]byte(cargoLockSnippet))
	if err != nil {
		t.Fatalf("parseCargo error = %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("len(deps) = %d, want 2", len(deps))
	}

	got := map[string]Dependency{}
	for _, dep := range deps {
		got[dep.Name] = dep
	}
	if got["serde"].Version != "1.0.203" || got["serde"].Integrity != "serde-checksum" {
		t.Fatalf("serde = %+v, want version 1.0.203 and checksum serde-checksum", got["serde"])
	}
	if got["toml"].Version != "0.8.14" || got["toml"].Integrity != "toml-checksum" {
		t.Fatalf("toml = %+v, want version 0.8.14 and checksum toml-checksum", got["toml"])
	}
}

func TestParsePip(t *testing.T) {
	t.Parallel()

	deps, err := parsePip([]byte(requirementsSnippet))
	if err != nil {
		t.Fatalf("parsePip error = %v", err)
	}
	if len(deps) != 3 {
		t.Fatalf("len(deps) = %d, want 3", len(deps))
	}

	got := map[string]Dependency{}
	for _, dep := range deps {
		got[dep.Name] = dep
	}
	if got["requests"].Version != "2.31.0" {
		t.Fatalf("requests = %+v, want version 2.31.0", got["requests"])
	}
	if got["urllib3"].Version != ">=2.2.1" {
		t.Fatalf("urllib3 = %+v, want version >=2.2.1", got["urllib3"])
	}
	if got["charset-normalizer"].Version != "~=3.3" {
		t.Fatalf("charset-normalizer = %+v, want version ~=3.3", got["charset-normalizer"])
	}
}

func TestParseGemfile(t *testing.T) {
	t.Parallel()

	deps, err := parseGemfile([]byte(gemfileLockSnippet))
	if err != nil {
		t.Fatalf("parseGemfile error = %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("len(deps) = %d, want 2", len(deps))
	}

	got := map[string]Dependency{}
	for _, dep := range deps {
		got[dep.Name] = dep
	}
	if got["rake"].Version != "13.2.1" {
		t.Fatalf("rake = %+v, want version 13.2.1", got["rake"])
	}
	if got["rspec"].Version != "3.13.0" {
		t.Fatalf("rspec = %+v, want version 3.13.0", got["rspec"])
	}
}

func TestParsePoetry(t *testing.T) {
	t.Parallel()

	deps, err := parsePoetry([]byte(poetryLockSnippet))
	if err != nil {
		t.Fatalf("parsePoetry error = %v", err)
	}
	if len(deps) != 2 {
		t.Fatalf("len(deps) = %d, want 2", len(deps))
	}

	got := map[string]Dependency{}
	for _, dep := range deps {
		got[dep.Name] = dep
	}
	if got["requests"].Version != "2.31.0" || got["requests"].Dev {
		t.Fatalf("requests = %+v, want version 2.31.0 and dev false", got["requests"])
	}
	if got["pytest"].Version != "8.2.2" || !got["pytest"].Dev {
		t.Fatalf("pytest = %+v, want version 8.2.2 and dev true", got["pytest"])
	}
}
