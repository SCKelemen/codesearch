package trigram

import (
	"sort"
)

// Trigram is a three-byte sequence used for index lookups.
type Trigram [3]byte

// Extract returns the unique valid trigrams found in content.
// Trigrams containing newlines or null bytes are skipped.
func Extract(content []byte) []Trigram {
	if len(content) < 3 {
		return nil
	}

	seen := make(map[Trigram]struct{}, len(content)-2)
	for i := 0; i+3 <= len(content); i++ {
		tri := Trigram{content[i], content[i+1], content[i+2]}
		if tri[0] == 0 || tri[1] == 0 || tri[2] == 0 {
			continue
		}
		if tri[0] == '\n' || tri[1] == '\n' || tri[2] == '\n' {
			continue
		}
		seen[tri] = struct{}{}
	}

	trigrams := make([]Trigram, 0, len(seen))
	for tri := range seen {
		trigrams = append(trigrams, tri)
	}

	sort.Slice(trigrams, func(i, j int) bool {
		return string(trigrams[i][:]) < string(trigrams[j][:])
	})

	return trigrams
}

// ExtractString returns the unique valid trigrams found in s.
func ExtractString(s string) []Trigram {
	return Extract([]byte(s))
}

func (t Trigram) String() string {
	return string(t[:])
}
