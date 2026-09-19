// Package engine coordinates durable database state transitions.
package engine

import "errors"

var ErrClosed = errors.New("database is closed")

// Mutation describes one atomic key-value change.
type Mutation struct {
	Key    []byte
	Value  []byte
	Delete bool
}

// Store is the durable coordination contract consumed by higher layers.
type Store interface {
	Get(key []byte) ([]byte, bool, error)
	Range(start, end []byte) ([]Entry, error)
	Set(key, value []byte) error
	Delete(key []byte) (bool, error)
	ApplyBatch(mutations []Mutation) error
	Close() error
}

// Entry is a key-value pair returned by a range operation.
type Entry struct {
	Key   []byte
	Value []byte
}
