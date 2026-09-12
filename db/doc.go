// Package db provides a minimal in-memory key-value database.
//
// A Database stores string keys and string values. Keys are unique, Set
// overwrites existing values, and Get reports whether a key exists.
//
// Range queries use inclusive string bounds and return results sorted by key in
// ascending order.
package db
