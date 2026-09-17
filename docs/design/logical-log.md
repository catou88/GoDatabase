# Logical Log Architecture Decision

## Status

Accepted. The standalone logical Set/Delete log has been removed.

## Context

The repository previously contained two independent persistence mechanisms:

- The production copy-on-write B+Tree writes replacement pages, syncs them,
  publishes a checksummed root metadata generation, and syncs metadata.
- `internal/kvlog` appended logical Set/Delete records and reconstructed an
  unrelated in-memory map by replaying the complete file.

The logical log was not connected to B+Tree roots, metadata generations, page
allocation, or recovery. A durable record therefore did not prove that its
corresponding tree update committed, and a committed tree root did not identify
a safe log replay position.

## Decision

The page-backed B+Tree and its checksummed metadata are the sole authoritative
source of committed database state. The logical log has no production role and
has been removed rather than retained as an unbounded educational component.

The database does not currently use a write-ahead log. Copy-on-write root
publication already supplies atomicity and durability for individual updates,
so adding a second persistence stream would duplicate guarantees while making
recovery ambiguous.

## Storage Responsibilities

The page-backed storage engine owns:

- durable Set and Delete operations;
- B+Tree and free-list page encoding;
- writing and syncing replacement pages before publication;
- checksummed, generation-numbered root metadata;
- fallback to the previous complete valid generation;
- complete-tree and page-ownership validation during recovery; and
- delayed page reuse while an older root still protects a page.

The removed logical log previously:

- framed and checksummed individual Set/Delete records;
- synced every appended operation;
- replayed all records into an independent map;
- truncated a partial trailing record; and
- rejected checksum failures and malformed records.

It had no commit records, transaction identifiers, generation linkage,
checkpoints, compaction, or bounded retention. Those limitations made it
unsuitable as a production WAL.

## Authoritative Recovery

Recovery reads the metadata slots and selects the newest generation whose
metadata, free list, and complete reachable B+Tree all validate. Logical
operation replay is not part of recovery.

Production update ordering remains:

1. Build replacement B+Tree and free-list pages without modifying committed
   pages.
2. Write all replacement pages.
3. Sync the database file.
4. Publish the next root metadata generation.
5. Sync metadata before returning success.

This root publication is the only commit point.

## Future WAL Requirements

A WAL should be reconsidered only when a new feature requires it, such as
multi-operation transactions, grouped commits, or in-place page updates. A
future design must define:

- monotonic transaction or log-sequence identifiers;
- explicit commit records;
- WAL sync before dependent page writes;
- metadata linkage to an exact durable WAL position;
- replay of committed transactions only;
- idempotent redo behavior;
- checkpoint creation and durability ordering;
- truncation only after the checkpoint and metadata are durable; and
- bounded retention or compaction.

The required ordering would be:

1. Append complete transaction redo records and a commit record.
2. Sync the WAL.
3. Write and sync replacement or dirty pages.
4. Publish and sync metadata containing the root and checkpoint position.
5. Truncate records older than the durable checkpoint.

Until that protocol exists, no logical log should be inserted into the current
page commit sequence.

## Corruption Policy For A Future WAL

- A partial final header or payload is a torn tail. Recovery may truncate it to
  the beginning of that record after preserving all earlier committed records.
- A checksum failure in the committed region is corruption and must fail open;
  it must not be skipped.
- An invalid type, impossible length, or oversized record is corruption unless
  it is demonstrably part of an incomplete uncommitted tail.
- Tail repair must be synced before appending new records.

## Consequences

- Recovery has exactly one source of committed truth.
- The database performs no duplicate logical writes or full-log replay.
- Removing the log eliminates its unbounded storage growth.
- No log checkpoint, compaction, or truncation subsystem is currently needed.
- Introducing transactions or a WAL requires a new architecture decision and
  implementation rather than reviving the removed standalone log unchanged.

## Follow-Up Work

- Design transaction semantics before considering a WAL.
- If a WAL becomes necessary, add generation-linked recovery and fault tests
  before connecting it to `KV.Set` or `KV.Delete`.
- Benchmark any future WAL against the existing copy-on-write commit path to
  justify its operational cost.
