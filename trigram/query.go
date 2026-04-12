package trigram

import (
	"errors"
	"fmt"
	"regexp"
	"regexp/syntax"
	"sort"
	"strings"
)

var ErrNoExtractableTrigrams = errors.New("no extractable trigrams")

// QueryPlan describes the trigram prefilter and regex used for final verification.
type QueryPlan struct {
	Trigrams []Trigram
	Regex    *regexp.Regexp
}

// BuildQueryPlan compiles pattern and extracts a conservative set of required trigrams.
// If no required trigrams can be extracted, it returns an error because full scans are not allowed.
func BuildQueryPlan(pattern string) (*QueryPlan, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}

	syn, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		return nil, err
	}

	info := analyzeRegexp(syn.Simplify())
	if len(info.required) == 0 && info.exact {
		for _, tri := range ExtractString(info.fixed) {
			info.required[tri] = struct{}{}
		}
	}

	trigrams := sortedTrigramsFromSet(info.required)
	if len(trigrams) == 0 {
		return nil, fmt.Errorf("%w for pattern %q", ErrNoExtractableTrigrams, pattern)
	}
	return &QueryPlan{Trigrams: trigrams, Regex: re}, nil
}

type regexpInfo struct {
	required map[Trigram]struct{}
	prefix   string
	suffix   string
	exact    bool
	fixed    string
}

func analyzeRegexp(re *syntax.Regexp) regexpInfo {
	switch re.Op {
	case syntax.OpLiteral:
		literal := string(re.Rune)
		return regexpInfo{
			required: trigramSetFromString(literal),
			prefix:   literal,
			suffix:   literal,
			exact:    true,
			fixed:    literal,
		}
	case syntax.OpCapture:
		if len(re.Sub) == 0 {
			return emptyRegexpInfo()
		}
		return analyzeRegexp(re.Sub[0])
	case syntax.OpConcat:
		return analyzeConcat(re.Sub)
	case syntax.OpAlternate:
		return analyzeAlternate(re.Sub)
	case syntax.OpPlus:
		if len(re.Sub) == 0 {
			return emptyRegexpInfo()
		}
		child := analyzeRegexp(re.Sub[0])
		return regexpInfo{
			required: cloneTrigramSet(child.required),
			prefix:   child.prefix,
			suffix:   child.suffix,
			exact:    false,
		}
	case syntax.OpRepeat:
		if len(re.Sub) == 0 {
			return emptyRegexpInfo()
		}
		if re.Min == 0 {
			return emptyRegexpInfo()
		}
		child := analyzeRegexp(re.Sub[0])
		info := regexpInfo{
			required: cloneTrigramSet(child.required),
			prefix:   child.prefix,
			suffix:   child.suffix,
			exact:    false,
		}
		if child.exact {
			prefix := strings.Repeat(child.fixed, re.Min)
			suffix := prefix
			info.prefix = prefix
			info.suffix = suffix
			mergeTrigramSet(info.required, trigramSetFromString(prefix))
			if re.Max == re.Min {
				info.exact = true
				info.fixed = prefix
			}
		}
		return info
	case syntax.OpEmptyMatch, syntax.OpBeginLine, syntax.OpEndLine, syntax.OpBeginText,
		syntax.OpEndText, syntax.OpWordBoundary, syntax.OpNoWordBoundary:
		return regexpInfo{required: make(map[Trigram]struct{}), exact: true, fixed: ""}
	default:
		return emptyRegexpInfo()
	}
}

func analyzeConcat(subs []*syntax.Regexp) regexpInfo {
	if len(subs) == 0 {
		return regexpInfo{required: make(map[Trigram]struct{}), exact: true, fixed: ""}
	}

	parts := make([]regexpInfo, 0, len(subs))
	for _, sub := range subs {
		parts = append(parts, analyzeRegexp(sub))
	}

	required := make(map[Trigram]struct{})
	for _, part := range parts {
		mergeTrigramSet(required, part.required)
	}
	for i := 0; i+1 < len(parts); i++ {
		mergeTrigramSet(required, bridgeTrigramSet(parts[i].suffix, parts[i+1].prefix))
	}

	var prefix strings.Builder
	for _, part := range parts {
		prefix.WriteString(part.prefix)
		if !part.exact {
			break
		}
	}

	suffix := ""
	for i := len(parts) - 1; i >= 0; i-- {
		suffix = parts[i].suffix + suffix
		if !parts[i].exact {
			break
		}
	}

	exact := true
	var fixedBuilder strings.Builder
	for _, part := range parts {
		if !part.exact {
			exact = false
			break
		}
		fixedBuilder.WriteString(part.fixed)
	}

	info := regexpInfo{
		required: required,
		prefix:   prefix.String(),
		suffix:   suffix,
		exact:    exact,
	}
	if exact {
		info.fixed = fixedBuilder.String()
		mergeTrigramSet(info.required, trigramSetFromString(info.fixed))
	}
	return info
}

func analyzeAlternate(subs []*syntax.Regexp) regexpInfo {
	if len(subs) == 0 {
		return emptyRegexpInfo()
	}

	parts := make([]regexpInfo, 0, len(subs))
	for _, sub := range subs {
		parts = append(parts, analyzeRegexp(sub))
	}

	required := cloneTrigramSet(parts[0].required)
	for _, part := range parts[1:] {
		required = intersectTrigramSets(required, part.required)
	}

	prefix := parts[0].prefix
	for _, part := range parts[1:] {
		prefix = commonPrefix(prefix, part.prefix)
	}

	suffix := parts[0].suffix
	for _, part := range parts[1:] {
		suffix = commonSuffix(suffix, part.suffix)
	}

	info := regexpInfo{
		required: required,
		prefix:   prefix,
		suffix:   suffix,
	}

	allExact := true
	fixed := parts[0].fixed
	for _, part := range parts {
		if !part.exact || part.fixed != fixed {
			allExact = false
			break
		}
	}
	if allExact {
		info.exact = true
		info.fixed = fixed
		mergeTrigramSet(info.required, trigramSetFromString(fixed))
	}

	return info
}

func emptyRegexpInfo() regexpInfo {
	return regexpInfo{required: make(map[Trigram]struct{})}
}

func trigramSetFromString(s string) map[Trigram]struct{} {
	set := make(map[Trigram]struct{})
	for _, tri := range ExtractString(s) {
		set[tri] = struct{}{}
	}
	return set
}

func bridgeTrigramSet(left, right string) map[Trigram]struct{} {
	if left == "" || right == "" {
		return map[Trigram]struct{}{}
	}
	combined := left
	if len(combined) > 2 {
		combined = combined[len(combined)-2:]
	}
	combined += right
	if len(combined) > 4 {
		combined = combined[:4]
	}
	return trigramSetFromString(combined)
}

func cloneTrigramSet(src map[Trigram]struct{}) map[Trigram]struct{} {
	dst := make(map[Trigram]struct{}, len(src))
	for tri := range src {
		dst[tri] = struct{}{}
	}
	return dst
}

func mergeTrigramSet(dst, src map[Trigram]struct{}) {
	for tri := range src {
		dst[tri] = struct{}{}
	}
}

func intersectTrigramSets(a, b map[Trigram]struct{}) map[Trigram]struct{} {
	if len(a) == 0 || len(b) == 0 {
		return map[Trigram]struct{}{}
	}
	out := make(map[Trigram]struct{})
	if len(a) > len(b) {
		a, b = b, a
	}
	for tri := range a {
		if _, ok := b[tri]; ok {
			out[tri] = struct{}{}
		}
	}
	return out
}

func sortedTrigramsFromSet(set map[Trigram]struct{}) []Trigram {
	trigrams := make([]Trigram, 0, len(set))
	for tri := range set {
		trigrams = append(trigrams, tri)
	}
	sort.Slice(trigrams, func(i, j int) bool {
		return string(trigrams[i][:]) < string(trigrams[j][:])
	})
	return trigrams
}

func commonPrefix(a, b string) string {
	maxLen := min(len(a), len(b))
	i := 0
	for i < maxLen && a[i] == b[i] {
		i++
	}
	return a[:i]
}

func commonSuffix(a, b string) string {
	maxLen := min(len(a), len(b))
	i := 0
	for i < maxLen && a[len(a)-1-i] == b[len(b)-1-i] {
		i++
	}
	return a[len(a)-i:]
}
