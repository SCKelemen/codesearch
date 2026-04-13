package advisory

import (
	"fmt"
	"strconv"
	"strings"
)

// Version represents a parsed semantic version.
type Version struct {
	Major      int
	Minor      int
	Patch      int
	Prerelease string
	Raw        string
}

// ParseVersion parses a semantic version string.
func ParseVersion(s string) (Version, error) {
	raw := strings.TrimSpace(s)
	if raw == "" {
		return Version{}, fmt.Errorf("parse version %q: empty version", s)
	}

	trimmed := strings.TrimPrefix(strings.TrimPrefix(raw, "v"), "V")
	core, prerelease, _ := strings.Cut(trimmed, "-")
	parts := strings.Split(core, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return Version{}, fmt.Errorf("parse version %q: invalid core version", s)
	}

	values := [3]int{}
	for i, part := range parts {
		if part == "" {
			return Version{}, fmt.Errorf("parse version %q: empty component", s)
		}
		value, err := strconv.Atoi(part)
		if err != nil {
			return Version{}, fmt.Errorf("parse version %q: %w", s, err)
		}
		values[i] = value
	}

	return Version{
		Major:      values[0],
		Minor:      values[1],
		Patch:      values[2],
		Prerelease: prerelease,
		Raw:        raw,
	}, nil
}

// Compare compares v to other using semantic version precedence.
func (v Version) Compare(other Version) int {
	switch {
	case v.Major < other.Major:
		return -1
	case v.Major > other.Major:
		return 1
	case v.Minor < other.Minor:
		return -1
	case v.Minor > other.Minor:
		return 1
	case v.Patch < other.Patch:
		return -1
	case v.Patch > other.Patch:
		return 1
	}

	switch {
	case v.Prerelease == "" && other.Prerelease == "":
		return 0
	case v.Prerelease == "":
		return 1
	case other.Prerelease == "":
		return -1
	}

	return comparePrerelease(v.Prerelease, other.Prerelease)
}

// String returns the canonical string form of the version.
func (v Version) String() string {
	base := fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch)
	if v.Prerelease == "" {
		return base
	}
	return base + "-" + v.Prerelease
}

// LessThan reports whether v is less than other.
func (v Version) LessThan(other Version) bool {
	return v.Compare(other) < 0
}

// GreaterThanOrEqual reports whether v is greater than or equal to other.
func (v Version) GreaterThanOrEqual(other Version) bool {
	return v.Compare(other) >= 0
}

func comparePrerelease(left, right string) int {
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	max := len(leftParts)
	if len(rightParts) > max {
		max = len(rightParts)
	}

	for i := 0; i < max; i++ {
		if i >= len(leftParts) {
			return -1
		}
		if i >= len(rightParts) {
			return 1
		}
		cmp := compareIdentifier(leftParts[i], rightParts[i])
		if cmp != 0 {
			return cmp
		}
	}

	return 0
}

func compareIdentifier(left, right string) int {
	leftNum, leftErr := strconv.Atoi(left)
	rightNum, rightErr := strconv.Atoi(right)
	if leftErr == nil && rightErr == nil {
		switch {
		case leftNum < rightNum:
			return -1
		case leftNum > rightNum:
			return 1
		default:
			return 0
		}
	}
	if leftErr == nil {
		return -1
	}
	if rightErr == nil {
		return 1
	}
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}
