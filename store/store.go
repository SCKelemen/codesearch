// Package store defines the core storage abstractions used by codesearch.
//
// The interfaces in this package are intentionally backend-agnostic so the
// index can be backed by in-memory maps, local files, or distributed systems
// such as Spanner, GCS, and vector databases without changing callers.
package store
