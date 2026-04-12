package memory

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/SCKelemen/codesearch/store"
)

func parseCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(cursor)
	if err != nil {
		return 0, fmt.Errorf("parse cursor: %w", err)
	}
	if offset < 0 {
		return 0, fmt.Errorf("parse cursor: negative offset %d", offset)
	}
	return offset, nil
}

func nextCursor(offset, total int) string {
	if offset >= total {
		return ""
	}
	return strconv.Itoa(offset)
}

func applyPage[T any](items []T, cursor string, limit int) ([]T, string, error) {
	start, err := parseCursor(cursor)
	if err != nil {
		return nil, "", err
	}
	if start > len(items) {
		start = len(items)
	}
	end := len(items)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	page := append([]T(nil), items[start:end]...)
	return page, nextCursor(end, len(items)), nil
}

func applySearchPage[T any](items []T, cursor string, limit int) ([]T, error) {
	start, err := parseCursor(cursor)
	if err != nil {
		return nil, err
	}
	if start > len(items) {
		start = len(items)
	}
	end := len(items)
	if limit > 0 && start+limit < end {
		end = start + limit
	}
	return append([]T(nil), items[start:end]...), nil
}

func containsFold(haystack, needle string) bool {
	if needle == "" {
		return true
	}
	return strings.Contains(strings.ToLower(haystack), strings.ToLower(needle))
}

func matchesMetadata(actual, required map[string]string) bool {
	if len(required) == 0 {
		return true
	}
	for key, value := range required {
		if actual[key] != value {
			return false
		}
	}
	return true
}

func matchesKinds(kind store.SymbolKind, kinds []store.SymbolKind) bool {
	if len(kinds) == 0 {
		return true
	}
	for _, candidate := range kinds {
		if kind == candidate {
			return true
		}
	}
	return false
}

func sortedStringKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func copyBytes(values []byte) []byte {
	if len(values) == 0 {
		return nil
	}
	return append([]byte(nil), values...)
}

func copyStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	return append([]string(nil), values...)
}

func copyMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	clone := make(map[string]string, len(values))
	for key, value := range values {
		clone[key] = value
	}
	return clone
}

func copyFloat32Slice(values []float32) []float32 {
	if len(values) == 0 {
		return nil
	}
	return append([]float32(nil), values...)
}

func cloneDocument(doc store.Document) store.Document {
	doc.Content = copyBytes(doc.Content)
	doc.Metadata = copyMap(doc.Metadata)
	return doc
}

func clonePostingList(list store.PostingList) store.PostingList {
	list.DocumentIDs = copyStringSlice(list.DocumentIDs)
	return list
}

func cloneSymbol(symbol store.Symbol) store.Symbol {
	symbol.Metadata = copyMap(symbol.Metadata)
	return symbol
}

func cloneReference(ref store.Reference) store.Reference {
	return ref
}

func cloneVector(vector store.StoredVector) store.StoredVector {
	vector.Values = copyFloat32Slice(vector.Values)
	vector.Metadata = copyMap(vector.Metadata)
	return vector
}

func normalizeVector(values []float32) []float32 {
	if len(values) == 0 {
		return nil
	}
	var norm float64
	for _, value := range values {
		norm += float64(value * value)
	}
	if norm == 0 {
		return copyFloat32Slice(values)
	}
	scale := float32(1 / math.Sqrt(norm))
	out := make([]float32, len(values))
	for index, value := range values {
		out[index] = value * scale
	}
	return out
}

func dotProduct(a, b []float32) float32 {
	limit := minInt(len(a), len(b))
	var sum float32
	for index := 0; index < limit; index++ {
		sum += a[index] * b[index]
	}
	return sum
}

func euclideanDistance(a, b []float32) float32 {
	limit := minInt(len(a), len(b))
	var sum float64
	for index := 0; index < limit; index++ {
		delta := float64(a[index] - b[index])
		sum += delta * delta
	}
	for index := limit; index < len(a); index++ {
		value := float64(a[index])
		sum += value * value
	}
	for index := limit; index < len(b); index++ {
		value := float64(b[index])
		sum += value * value
	}
	return float32(math.Sqrt(sum))
}

func cosineSimilarity(a, b []float32) float32 {
	an := normalizeVector(a)
	bn := normalizeVector(b)
	return dotProduct(an, bn)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
