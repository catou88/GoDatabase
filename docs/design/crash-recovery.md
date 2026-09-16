# Crash Recovery Design

## Goal

Document how the future disk-backed database should recover after interrupted
writes.

The first durable design should prefer simple, testable guarantees over maximum
write performance. The storage layer should never expose a partially written
B+Tree as the current database state after restart.

## Durability Guarantee

After a successful write operation returns to the caller:

- the new root page is durable;
- all pages reachable from that root are durable;
- reopening the database returns either the complete old tree or the complete
  new tree;
- recovery must not require replaying user operations.

If the process crashes before a write operation returns, the database may lose
that in-flight operation, but it must still reopen to a valid previous tree.

This is atomicity and durability at the single-operation level, not full
transaction support.

## Failure Scenarios

Recovery must handle these cases:

- Process exits before writing any new pages.
- Process exits after writing some new pages but before updating metadata.
- Process exits while writing a new page.
- Process exits after writing all new pages but before syncing them.
- Process exits while updating the root pointer metadata.
- Process exits after metadata is written but before metadata is synced.
- Database file contains trailing pages that are not reachable from the root.
- Database file has a short or corrupt page.
- Metadata points to an invalid, corrupt, or unreachable root page.

The recovery rule is simple: only metadata that passes validation may define the
current root. Pages that are not reachable from the selected root are ignored
until a free-list implementation can reuse them.

## Write Strategy

The initial disk-backed B+Tree should use copy-on-write page updates.

For each mutating operation:

1. Read the current root page number from metadata.
2. Build updated B+Tree pages without overwriting existing reachable pages.
3. Write every new or changed page to newly allocated page IDs.
4. Sync the database file so the new pages reach durable storage.
5. Write metadata that points to the new root page.
6. Sync the metadata.

Existing pages reachable from the old root are not modified in place. This means
the old tree remains valid until the root pointer update commits the new tree.

## Metadata Layout

The database file should reserve fixed metadata slots at the beginning of the
file. Each metadata slot stores one complete root record.

```text
| metadata slot A | metadata slot B | page 1 | page 2 | ... |
```

Each metadata slot should contain:

```text
| magic | version | generation | root_page | page_count | checksum |
```

Field meanings:

- `magic` identifies the record as GoDatabase metadata.
- `version` identifies the metadata format.
- `generation` increases after every committed root update.
- `root_page` stores the page ID of the committed root.
- `page_count` records the number of allocated pages visible to the commit.
- `checksum` protects the metadata record from partial writes.

Two slots make the root update recoverable:

- write the next metadata record into the older slot;
- sync it;
- on open, choose the valid slot with the highest generation.

If a crash corrupts one slot, the previous valid slot can still be used.

## Root Pointer Update Strategy

The root page pointer is the commit point.

Before the root metadata update:

- the old root is current;
- newly written pages are unreachable;
- recovery should ignore the new pages.

After the root metadata update is synced:

- the new root is current;
- recovery should use the metadata slot with the highest valid generation;
- old pages become unreachable garbage until a free list reclaims them.

The implementation must never overwrite the only valid root metadata record
without first having another valid record available.

## Fsync Expectations

The storage layer should expose a durable write path that calls `Sync` in this
order:

1. Write all new B+Tree pages.
2. `fsync` the database file.
3. Write the new metadata slot.
4. `fsync` the database file again.

When creating or replacing a database file, the implementation should also sync
the parent directory after file creation or rename. This prevents a crash from
losing the directory entry on filesystems that require directory sync.

Tests may use dependency injection or a fake file implementation to verify sync
ordering without forcing real crashes.

## Recovery Procedure

On database open:

1. Open the database file.
2. Read both metadata slots.
3. Discard metadata slots with invalid magic, unsupported version, invalid
   checksum, zero root page, or impossible page count.
4. Select the valid metadata slot with the highest generation.
5. Verify the root page exists and decodes successfully.
6. Traverse reachable B+Tree pages from the root.
7. Validate page types, key order, child pointers, and tree invariants.
8. Ignore pages not reachable from the selected root.
9. If no valid metadata slot exists, open an empty database only when the file
   is new or explicitly initialized as empty.

If metadata exists but no valid root can be recovered, opening the database
should fail with a clear corruption error instead of silently returning an empty
database.

## Known Limitations

The first recovery design intentionally does not provide:

- multi-operation transactions;
- concurrent writer recovery;
- write-ahead logging;
- rollback of individual operations after metadata commit;
- automatic reuse of unreachable pages;
- protection from disk firmware or filesystem bugs that acknowledge `fsync`
  before data is durable;
- protection from torn writes inside a page unless page checksums are added;
- recovery from both metadata slots being corrupted.

These limitations are acceptable for the first durable B+Tree because the main
guarantee is that the root pointer only moves after all new pages are durable.

## Future Implementation Tasks

- Add metadata slot encoding and decoding.
- Add metadata checksums.
- Reserve metadata space before page ID `1`.
- Update the page manager so page offsets account for metadata slots.
- Add a durable commit path that writes pages before metadata.
- Add sync-order tests with a fake file.
- Add recovery tests for stale, corrupt, and partially written metadata slots.
- Add tests for unreachable pages after interrupted writes.
- Add page checksums if torn page detection is required.
- Add a free list after recovery can safely identify unreachable pages.
