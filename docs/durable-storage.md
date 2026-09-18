# Durable Storage

## Overview

`db.Open` provides a string-based key-value database backed by a persistent,
copy-on-write B+Tree. Successful mutations are synchronized before they return,
and the database validates committed storage when it is reopened.

Use `db.New` instead when persistence is not required. It creates an in-memory
database with the same operation semantics.

## Opening And Creating Files

```go
database, err := db.Open("app.db")
if err != nil {
	return err
}
defer func() {
	if err := database.Close(); err != nil {
		log.Printf("close database: %v", err)
	}
}()
```

`Open` creates the file when it does not exist and reopens it when it does. The
parent directory must already exist. A newly created file uses owner-only
permissions (`0600`), and creation synchronizes both the file and its parent
directory.

Opening an existing file validates its size, metadata, free-list pages, and the
complete B+Tree reachable from the selected root. `Open` returns an error for an
invalid path, an unsupported format, or a committed tree that cannot be
validated. Committed tree pages are currently loaded into memory during open.

Do not open the same file through multiple `Database` handles or processes at
the same time. The implementation does not provide file locking or coordinate
independent page allocators.

## Closing And Ownership

The caller owns a `Database` returned by `Open` and must call `Close`. `Close`
releases the file descriptor and is idempotent. Mutations are synchronized
before they return, so `Close` does not act as a delayed commit or flush step.

After `Close`, `Set`, `Get`, `Delete`, and `Range` return `db.ErrClosed`. Do not
copy a `Database` value after first use; pass its pointer instead.

## Operation Semantics

### Set

`Set(key, value)` inserts a key or overwrites its existing value. Keys are
unique. An empty key returns `db.ErrEmptyKey`, while an empty value is valid.
Keys may contain at most 1000 bytes and values at most 3000 bytes in the current
format.

A successful durable `Set` means the new tree and its root metadata have been
synchronized. An error means the caller must not assume the update committed.

### Get

`Get(key)` returns `(value, true, nil)` when the key exists. A missing key,
including an empty lookup key, returns `("", false, nil)`. Stored empty values
are distinguishable from missing keys through the boolean result.

### Delete

`Delete(key)` returns `(true, nil)` when it removes an existing key. A missing
or empty key returns `(false, nil)` and does not create a new commit.

A successful deletion is durable before the method returns.

### Range

`Range(start, end)` returns every matching key/value pair in ascending key order.
Both bounds are inclusive. Keys are ordered lexicographically by their encoded
string bytes, and every matching key appears exactly once.

An empty bound is an ordinary empty string, not an unbounded marker. Therefore,
`Range("", "c")` includes non-empty keys through `"c"`, while
`Range("a", "")` returns no results because the start is greater than the end.
Empty and nonmatching ranges return an empty slice and a `nil` error.

## Commit And Durability

Mutations use copy-on-write pages. Existing committed B+Tree pages are not
modified in place. A mutation follows this order:

1. Build replacement pages and a new root while retaining the old tree.
2. Write all new and changed B+Tree pages.
3. Synchronize the B+Tree pages.
4. Write the page-backed free-list snapshot.
5. Synchronize the free-list pages.
6. Write the next metadata generation into the alternate metadata slot.
7. Synchronize the metadata.
8. Publish the new root to the in-memory handle and return success.

The synchronized metadata generation is the commit point. Before that point,
the previous root remains authoritative. After a successful return, all pages
reachable from the new root and the metadata selecting it have been passed to
the operating system's synchronization primitive.

This guarantee still depends on the operating system, filesystem, and storage
device honoring synchronization correctly.

## Recovery And Metadata Fallback

The file contains two checksummed metadata slots. Each committed update writes
the next generation to the alternate slot rather than overwriting both copies.

During `Open`, recovery examines candidates from newest to oldest. For each
candidate it validates metadata, the page-backed free list, every reachable
B+Tree page, child references, key bounds, node occupancy, and equal leaf depth.
If the newest generation is incomplete or structurally corrupt, recovery falls
back to the older complete generation. If metadata candidates exist but none
describe a valid committed tree, `Open` returns a corruption error.

Current limitation: if both metadata slots fail metadata decoding, an otherwise
page-aligned file is treated as an empty database. Applications requiring
stronger disaster recovery should keep external backups until this ambiguity is
removed from the format.

## Free-Page Protection And Reuse

Pages replaced by copy-on-write updates are not immediately available for
allocation. They are first recorded as protected retired pages so the previous
metadata generation can still be recovered safely. A later committed generation
moves eligible pages into the reusable set.

Allocation consumes reusable page IDs before extending the file. The free list
is itself stored in checksummed, linked pages and is committed with the root
metadata. On open, page ownership is reconciled so a page cannot simultaneously
belong to the live tree, reusable set, protected set, or free-list structure.

File size does not necessarily shrink when pages are deleted. Reuse prevents
unbounded growth during steady workloads by overwriting eligible page slots.

## Corruption Behavior

`Open` rejects unsupported versions, misaligned or short files, invalid
metadata checksums, malformed free-list pages, invalid page references, cycles,
duplicate references, unsorted nodes, inconsistent parent bounds, unequal leaf
depths, and other structural B+Tree violations.

B+Tree data pages do not currently have checksums. Corruption that also breaks
their structure is detected, but a bit change that leaves a page structurally
valid may not be. The database reports corruption; it does not repair damaged
user data.

## Thread Safety

A single public `*db.Database` may be called by multiple goroutines. Operations
are synchronized with the database lifecycle, so `Close` cannot race through an
active public operation. The durable engine currently serializes its operations,
including reads, and does not yet provide concurrent snapshot readers or
independent writer versions.

Thread safety does not extend across separately opened handles or processes.

## File-Format Compatibility

The current file uses 4096-byte pages, little-endian integer encoding, B+Tree
page format version 2, metadata format version 3, and free-list format version
1. The metadata reader recognizes older metadata versions 1 and 2, but B+Tree
pages must use the supported page format.

Unsupported versions return an error; they are not automatically migrated.
The on-disk format is still experimental, so future incompatible releases may
require an explicit migration or rebuilding the database from an export. Back
up durable files before upgrading across format changes.

## Known Limitations

- No multi-operation atomic transactions.
- No write-ahead log or group commit.
- No cross-process or multi-handle coordination.
- No public snapshots or retained historical versions.
- No online backup API.
- No encryption, compression, or overflow pages.
- Keys and values are limited to one-page-compatible sizes.
- B+Tree data pages do not have checksums.
- Opening loads all committed tree pages into memory.
- Corruption is detected where possible but is not repaired automatically.
