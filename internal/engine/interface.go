// Package engine coordinates durable database state transitions.
package engine

import (
	"errors"

	"godatabase/internal/structures"
)

var ErrClosed = errors.New("database is closed")

// Mutation describes one atomic key-value change.
type Mutation struct {
	Key    []byte
	Value  []byte
	Delete bool
}

// Store is the durable coordination contract consumed by higher layers.
type Store interface {
	structures.KV
	ApplyBatch(mutations []Mutation) error
	Close() error
}

// Entry is an alias retained for engine callers while the common operation
// contract lives in internal/structures.
type Entry = structures.Entry
