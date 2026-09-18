# Transaction API and Isolation Design

## Goal

Define a transaction API for grouping database operations while preserving the
durability and versioned-tree guarantees of the storage engine.

This document defines the intended contract. The transaction API is not yet
implemented.

## Proposed API

The public API should expose explicit transaction ownership:

```go
type TxOptions struct {
	ReadOnly bool
}

func (db *Database) Begin(options TxOptions) (*Tx, error)

type Tx struct { /* private state */ }

func (tx *Tx) Get(key string) (string, bool, error)
func (tx *Tx) Set(key, value string) error
func (tx *Tx) Delete(key string) (bool, error)
func (tx *Tx) Range(start, end string) ([]Item, error)
func (tx *Tx) Commit() error
func (tx *Tx) Rollback() error
```

Convenience methods may be added later:

```go
func (db *Database) View(fn func(*Tx) error) error
func (db *Database) Update(fn func(*Tx) error) error
```

The callback helpers would automatically commit when the callback succeeds and
roll back when it returns an error or panics. Explicit `Tx` ownership remains
the underlying model.

## Transaction Lifecycle

1. `Begin` captures a committed root generation.
2. Operations read from the transaction's pinned version.
3. A read-write transaction records changes in private copy-on-write pages.
4. `Commit` writes new pages, synchronizes them, and publishes a new root.
5. `Rollback` discards private pages and leaves the committed root unchanged.
6. A transaction becomes closed after `Commit` or `Rollback`.

Operations on a closed transaction return `ErrTransactionClosed`. Committing or
rolling back twice is either idempotent or returns that error; the final API
should choose one behavior and test it consistently. This design prefers an
idempotent `Rollback` and an error for a second `Commit`.

## Read-Only Transactions

Read-only transactions pin one immutable committed root generation. They may
run concurrently with other readers and with a writer creating a newer private
version. They continue to see the same data for their entire lifetime:

```go
tx, err := database.Begin(db.TxOptions{ReadOnly: true})
if err != nil {
	return err
}
defer tx.Rollback()

value, found, err := tx.Get("user:1")
```

Read-only transactions cannot call `Set`, `Delete`, or `Commit`. They must use
`Rollback` or an equivalent close operation to release their version pin.

## Read-Write Transactions

A read-write transaction starts from a committed root and applies changes to a
private copy-on-write version. Existing committed pages are never modified in
place. Other readers continue using their pinned roots while the writer works.

The initial implementation permits one active read-write transaction per
database handle. This serializes writers during the transaction lifetime and
avoids optimistic conflict detection or automatic merge behavior.

Example:

```go
tx, err := database.Begin(db.TxOptions{})
if err != nil {
	return err
}

if err := tx.Set("user:1", "Ada"); err != nil {
	_ = tx.Rollback()
	return err
}
if err := tx.Commit(); err != nil {
	return err
}
```

A read-write transaction that is rolled back publishes no changes. A failed
commit must restore the transaction and database in-memory state to the last
known committed generation so retrying or reopening remains safe.

## Isolation Guarantee

The initial isolation level is **snapshot isolation for readers with serialized
writes**:

- A transaction reads one pinned root generation.
- Reads do not observe later commits made after `Begin`.
- Read-only transactions can run concurrently with writers.
- A writer sees its own writes.
- A writer's uncommitted pages are invisible to other transactions.
- Only one read-write transaction commits at a time on a database handle.
- A successful commit becomes visible as one new root generation.

This is not full serializable isolation. A future version may provide explicit
version handles and multiple independent writers, but it must define how a
version becomes canonical before allowing applications to assume merged state.

## Conflict Behavior

The initial design does not use optimistic conflict detection or automatic
merging. Writer conflicts are prevented by the per-handle writer admission
rule:

- `Begin(ReadOnly: false)` returns `ErrWriteTransactionActive` if another
  read-write transaction is active;
- a read-only transaction never blocks a writer;
- a writer never changes a reader's pinned root;
- a transaction cannot commit after it has been rolled back or closed.

Multiple database handles for the same durable file remain unsupported. The
single-handle rule is required until cross-process coordination and file-locking
semantics are designed.

## Commit and Recovery

Commit follows the existing copy-on-write durability sequence:

1. Encode and write new data pages.
2. Synchronize data pages.
3. Write and synchronize free-list state as required.
4. Publish metadata containing the new generation and root page.
5. Synchronize metadata before returning success.

The old root remains a valid recovery target until the new metadata generation
is durable. A crash before metadata publication exposes the old generation. A
crash after publication must recover the complete new generation or fall back to
the previous valid generation.

Pages belonging to pinned reader roots cannot be reclaimed. Once the final
reader releases a generation and no recovery metadata protects it, obsolete
pages may enter the reusable free list.

## Non-Goals

The initial transaction implementation does not provide:

- cross-process or multi-handle transactions;
- full serializable isolation;
- optimistic conflict detection;
- automatic merging of independent writer versions;
- distributed transactions;
- two-phase commit;
- savepoints or nested transactions;
- durable transaction IDs or an external transaction log;
- atomic schema changes and index backfills;
- streaming cursors that outlive their transaction;
- automatic retry after a failed commit.

## Required Tests

Before implementation is considered complete, add tests for:

- read-only `Begin`, `Get`, and `Range`;
- read-only rejection of `Set`, `Delete`, and `Commit`;
- read-your-writes inside a read-write transaction;
- rollback leaving the committed database unchanged;
- commit visibility after reopening;
- one active writer and concurrent readers;
- readers retaining old values while a writer commits a new root;
- write admission failure for a second writer;
- closed transaction operations;
- interrupted page and metadata writes;
- protection and later reuse of pages pinned by readers;
- race-free concurrent read workloads.

## Follow-Up Decisions

The implementation issue must decide the exact error names, whether `Rollback`
is idempotent, the maximum reader lifetime, and how long old generations remain
available for recovery. Those choices affect free-page pressure and benchmark
results, so transaction benchmarks should be recorded before adding version
pruning or concurrent-writer optimizations.
