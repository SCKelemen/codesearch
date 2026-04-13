package vuln

import (
	"sort"

	"github.com/SCKelemen/codesearch/advisory"
)

// Index provides fast advisory lookup by ecosystem and package name.
type Index struct {
	byPackage  map[string][]advisory.Advisory
	byID       map[string]*advisory.Advisory
	total      int
	advisories []advisory.Advisory
}

// NewIndex builds an advisory lookup index.
func NewIndex(advisories []advisory.Advisory) *Index {
	idx := &Index{
		byPackage:  make(map[string][]advisory.Advisory),
		byID:       make(map[string]*advisory.Advisory, len(advisories)),
		total:      len(advisories),
		advisories: append([]advisory.Advisory(nil), advisories...),
	}

	for i := range idx.advisories {
		adv := &idx.advisories[i]
		idx.byID[adv.ID] = adv
		for _, affected := range adv.Affected {
			key := packageKey(string(affected.Ecosystem), affected.Name)
			idx.byPackage[key] = append(idx.byPackage[key], *adv)
		}
	}

	for key := range idx.byPackage {
		sort.Slice(idx.byPackage[key], func(i, j int) bool {
			return idx.byPackage[key][i].ID < idx.byPackage[key][j].ID
		})
	}

	return idx
}

// Lookup returns all advisories affecting the given package.
func (idx *Index) Lookup(ecosystem, name string) []advisory.Advisory {
	if idx == nil {
		return nil
	}

	matches := idx.byPackage[packageKey(ecosystem, name)]
	return append([]advisory.Advisory(nil), matches...)
}

// Get returns a specific advisory by ID.
func (idx *Index) Get(id string) *advisory.Advisory {
	if idx == nil {
		return nil
	}
	return idx.byID[id]
}

// Len returns the number of indexed advisories.
func (idx *Index) Len() int {
	if idx == nil {
		return 0
	}
	return idx.total
}

// ScannerFromIndex creates a scanner backed by an index for faster lookups.
func ScannerFromIndex(idx *Index, opts ScannerOptions) *Scanner {
	scanner := NewScanner(nil, opts)
	if idx == nil {
		return scanner
	}
	scanner.advisories = append([]advisory.Advisory(nil), idx.advisories...)
	scanner.idx = idx
	return scanner
}
