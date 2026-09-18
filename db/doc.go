// Package db provides in-memory and durable key-value databases.
//
// A Database stores string keys and string values. Keys are unique, Set
// overwrites existing values, and Get reports whether a key exists. New creates
// an in-memory database, while Open opens or creates a page-backed database whose
// contents survive Close and Open.
//
// Range queries use inclusive string bounds and return results sorted by key in
// ascending order. A Database opened from a path owns its file handle and should
// be closed by its caller. A single Database may be used by multiple goroutines;
// separately opened handles must not access the same file concurrently.
package db
