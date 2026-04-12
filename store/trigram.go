package store

import (
	"context"
	"errors"
)

// Trigram is a three-byte n-gram encoded in a uint32.
type Trigram uint32

// NewTrigram encodes three bytes into a trigram value.
func NewTrigram(a, b, c byte) Trigram {
	return Trigram(uint32(a)<<16 | uint32(b)<<8 | uint32(c))
}

// ParseTrigram parses a three-byte string into a trigram value.
func ParseTrigram(s string) (Trigram, error) {
	if len(s) != 3 {
		return 0, errors.New("trigram must be exactly three bytes")
	}
	return NewTrigram(s[0], s[1], s[2]), nil
}

// Bytes decodes the trigram into its three-byte representation.
func (t Trigram) Bytes() [3]byte {
	return [3]byte{byte(t >> 16), byte(t >> 8), byte(t)}
}

// String returns the trigram as a three-byte string.
func (t Trigram) String() string {
	b := t.Bytes()
	return string(b[:])
}

// PostingList stores the set of documents associated with a trigram.
type PostingList struct {
	Trigram     Trigram
	DocumentIDs []string
}

// PostingResult describes a document matched by a trigram search.
type PostingResult struct {
	DocumentID        string
	MatchedTrigrams   int
	CandidateTrigrams int
}

// TrigramStore stores trigram posting lists and candidate lookups.
type TrigramStore interface {
	// Put creates or replaces a posting list.
	Put(ctx context.Context, list PostingList) error

	// Lookup returns the posting list for a single trigram.
	Lookup(ctx context.Context, trigram Trigram, opts ...LookupOption) (*PostingList, error)

	// List returns posting lists and the next cursor.
	// An empty next cursor means there are no more results.
	List(ctx context.Context, opts ...ListOption) ([]PostingList, string, error)

	// Search returns candidate documents for the supplied trigram set.
	Search(ctx context.Context, trigrams []Trigram, opts ...SearchOption) ([]PostingResult, error)

	// Delete removes a posting list.
	Delete(ctx context.Context, trigram Trigram) error
}
