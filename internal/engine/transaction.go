package engine

import "context"

// WriterAdmission serializes read-write transaction ownership. Implementations
// must release admission on commit, rollback, cancellation, and Close.
type WriterAdmission interface {
	Acquire(ctx context.Context) error
	Release()
}

// SnapshotRegistry owns the lifetime of immutable committed roots. It is a
// future boundary; the current engine does not provide pinned snapshots.
type SnapshotRegistry interface {
	Acquire() (Snapshot, error)
	Release(snapshot Snapshot) error
}

// Snapshot identifies a committed root protected from page reuse.
type Snapshot interface {
	Generation() uint64
	RootPageID() uint64
	Cancel() error
}
