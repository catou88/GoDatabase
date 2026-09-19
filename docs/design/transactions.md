# Transaction API and Isolation Design

## Goal

Define a transaction API for grouping database operations while preserving the
durability and versioned-tree guarantees of the storage engine.

This document defines the intended contract. The current implementation
provides buffered atomic KV transactions, serialized read-write transaction
admission, rollback, and transactional table/index mutation methods. Immutable
snapshot roots, reader version pinning, and universal write-path admission and
atomicity remain follow-up work. Unbuffered reads observe current committed
state. See the [architecture plan](../architecture.md) for current limitations
and the target engine ownership boundary.

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

## Current Coordination Rules

The current implementation owns transaction coordination in `db.Database` and
durable commit coordination in `internal/engine`. The public `db.Tx` API is
unchanged; callers do not manage roots or pages directly.

- `Database.Begin` checks closed state and admits at most one read-write
  transaction per handle.
- The database mutex protects transaction state, writer admission, and public
  operation coordination. Code holding it must not perform network I/O or call
  user callbacks.
- The engine owns page preparation, data synchronization, metadata publication,
  and recovery restoration. It returns errors without exposing page internals.
- A failed commit keeps the previous committed root visible and leaves the
  transaction active until the caller rolls it back.
- `Rollback` releases writer admission and is safe to call after a failed
  commit. `Close` prevents new work and releases owned resources.

The current durable engine delegates the established page commit and recovery
algorithm through `internal/engine.Durable`. Physical extraction of metadata,
free-list, and recovery code remains a follow-up that must preserve these
contracts.

## Transaction Lifecycle

1. `Begin` opens a transaction view.
2. Operations read buffered writes first and otherwise use the current
   committed state.
3. A read-write transaction records changes in private copy-on-write pages.
4. `Commit` writes new pages, synchronizes them, and publishes a new root.
5. `Rollback` discards private pages and leaves the committed root unchanged.
6. A transaction becomes closed after `Commit` or `Rollback`.

Operations on a closed transaction return `ErrTransactionClosed`. Committing or
rolling back twice is either idempotent or returns that error; the final API
should choose one behavior and test it consistently. This design prefers an
idempotent `Rollback` and an error for a second `Commit`.

## Future Snapshot Contract

The future snapshot implementation will pin one immutable committed root
generation. It is not implemented by the current transaction code. When added,
the engine will expose the following ownership rules:

- Acquisition and page-pin registration happen atomically.
- Release is idempotent and makes the generation eligible for reuse only when
  no snapshot, recovery slot, or active writer protects it.
- Cancellation releases the snapshot before returning.
- Database `Close` stops new acquisitions, waits for or cancels owned handles
  according to the documented shutdown policy, and then closes storage.
- Snapshot readers may run concurrently, while writers remain serialized until
  writer admission and reclamation are independently tested.

The public read-only transaction API may later use this contract:

```go
tx, err := database.Begin(db.TxOptions{ReadOnly: true})
if err != nil {
	return err
}
defer func() { _ = tx.Rollback() }()

value, found, err := tx.Get("user:1")
```

Until that implementation exists, do not claim that read-only transactions are
repeatable snapshots or that they protect pages from reuse. Read-only
transactions cannot call `Set`, `Delete`, or `Commit` under the current API.

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
defer func() { _ = tx.Rollback() }()

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
The current transaction remains active when Commit fails; the deferred Rollback
in the example releases writer admission on that error path. It is harmless
after a successful Commit because Rollback is idempotent.

## Isolation Guarantee

The planned isolation level is **snapshot isolation for readers with serialized
writes**:

- A transaction reads one pinned root generation.
- Reads do not observe later commits made after `Begin`.
- Read-only transactions can run concurrently with writers.
- A writer sees its own writes.
- A writer's uncommitted pages are invisible to other transactions.
- Only one read-write transaction commits at a time on a database handle.
- A successful commit becomes visible as one new root generation.

The current implementation has not added immutable root pinning. Buffered writes
are isolated until commit, but reads of keys that are not buffered use the
database's current committed state. Full snapshot isolation therefore remains
a follow-up implementation task.

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

## Lock Ordering and Commit Publication

The required lock order is:

1. Acquire the database coordination lock for public state and writer admission.
2. Call the engine while retaining ownership of the mutation decision.
3. Let the engine coordinate page writes and metadata publication; it must not
   call back into `db.Database` while the database lock is held.
4. Release database ownership after commit or rollback has restored the state.

Storage locks, when introduced, must be acquired below database coordination
and released before network writes or user callbacks. No lock may be held while
waiting for an external model, client, or unbounded stream.

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

The current atomic-transaction implementation tests commit, rollback, durable
reopen, read-only write rejection, closed transactions, and failed batch
commits. The remaining tests below apply to the future snapshot implementation.

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
