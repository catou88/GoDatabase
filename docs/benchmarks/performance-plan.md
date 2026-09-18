# Performance Measurement Plan

This document tracks when and how to measure GoDatabase performance so changes can be compared with evidence instead of guesses.

## Benchmark Reminder

Before starting any feature that can affect storage, synchronization,
concurrency, transactions, caching, indexing, or query execution:

1. Stop before changing the implementation.
2. Run and commit a repeated `before` benchmark from the current branch.
3. Implement and validate the feature.
4. Run the identical benchmark as `after` on the same machine and Go version.
5. Compare both files with `benchstat`.
6. Record the durability mode, dataset, commit IDs, and important tradeoffs.

Do not reconstruct a `before` result after implementation. The original code
may no longer be reproducible, and the comparison will be less credible.

When working on one of the checkpoints below, remind the contributor to capture
the `before` benchmark before implementation begins.

## What To Measure

Core key-value metrics:

- `Set` throughput and latency
- `Get` throughput and latency
- `Delete` throughput and latency
- `Range` query latency across different result sizes
- `B/op` and `allocs/op` for hot operations

Storage-engine metrics:

- Map-backed store vs B+Tree-backed store
- In-memory writes vs durable writes
- Page serialization/deserialization cost
- Recovery time after reopening persisted data

Concurrency metrics:

- Read throughput before and after versioned snapshot readers
- Write throughput before and after synchronization
- Mixed read/write workload behavior
- Race detector validation

Index/query metrics:

- Full scan vs secondary index lookup
- Primary-key lookup latency
- Range scan latency by dataset size

## Completed Baselines

- In-memory map baseline: `docs/benchmarks/baseline-map.md`
- Durable copy-on-write B+Tree baseline:
  `docs/benchmarks/baseline-durable.md`

Keep these files unchanged as historical reference points. Store future results
in new files.

## Future Checkpoints

### Streaming range iteration

Run before and after replacing slice-based results or adding an iterator.
Measure Range latency, bytes allocated, allocations, and result throughput for
small and large ranges.

Suggested files:

- `range-iterator-before.txt`
- `range-iterator-after.txt`

### Synchronization-path optimization

Run before and after combining synchronization barriers or introducing a
validated one-sync protocol. Measure durable Set, overwrite, Delete, commit
latency, and recovery behavior. Keep the durability guarantee identical.

Suggested files:

- `sync-path-before.txt`
- `sync-path-after.txt`

### Multi-operation transactions

Measure transactions containing 1, 10, 100, and 1,000 operations. Record total
transaction latency, amortized latency per operation, bytes written, pages
written, and allocations.

Suggested files:

- `transactions-before.txt`
- `transactions-after.txt`

### Group commit

Measure 1, 2, 4, 8, and 16 concurrent writers. Record throughput, average
latency, p50, p95, p99, and commits per synchronization operation.

Suggested files:

- `group-commit-before.txt`
- `group-commit-after.txt`

### Snapshot isolation and versioned roots

Measure concurrent Get and Range throughput, mixed readers and writers, writer
latency, retained memory, and delayed page reclamation. Compare 1, 2, 4, 8, 16,
and 32 readers. Run race tests with this checkpoint.

Suggested files:

- `snapshot-readers-before.txt`
- `snapshot-readers-after.txt`

### Removing the global read mutex

If this is separate from snapshot isolation, compare single-reader latency and
concurrent-reader scaling. An unchanged single-reader result can still be a
success when multi-reader throughput improves.

Suggested files:

- `read-lock-before.txt`
- `read-lock-after.txt`

### Page cache or lazy loading

The current durable Get traverses pages loaded during Open. Before changing
that behavior, capture Open time and memory use. Afterward measure warm-cache
Get, cold-cache Get, cache hit/miss latency, memory use, and random disk reads.

Suggested files:

- `page-cache-before.txt`
- `page-cache-after.txt`

### Write-amplification optimization

Measure pages written, bytes written, split/merge frequency, file growth,
latency, and allocations for Set, overwrite, and Delete.

Suggested files:

- `write-amplification-before.txt`
- `write-amplification-after.txt`

### Write-ahead log

Compare the current synchronized copy-on-write path with a WAL under equivalent
durability. Measure synchronous and batched commits, checkpoint cost, recovery
time, and log growth.

Suggested files:

- `wal-before.txt`
- `wal-after.txt`

### Secondary indexes and query execution

Compare full scans with indexed lookups and measure parser/execution overhead
separately from storage time where possible.

Suggested files:

- `indexes-before.txt` and `indexes-after.txt`
- `query-engine-before.txt` and `query-engine-after.txt`

### LSM-tree experiment

Treat an LSM tree as a separate engine and compare it with the B+Tree under the
same durability policy. Measure random writes, point reads, range scans, read
and write amplification, compaction stalls, disk usage, memory use, and recovery
time.

Suggested file: `btree-vs-lsm.txt`

### Tagged releases

Before every tagged release, save a complete repeated benchmark as
`vX.Y.Z.txt`. Use it for release notes and only make resume claims supported by
repeatable results.

## Benchmark Commands

Run all benchmarks with memory metrics:

```bash
go test -bench=. -benchmem ./...
```

Run benchmarks multiple times for more stable comparisons:

```bash
go test -run='^$' -bench=. -benchmem -count=10 ./... \
  > docs/benchmarks/feature-before.txt
```

Run race tests when concurrency behavior changes:

```bash
go test -race ./...
```

Install `benchstat` for before/after comparisons:

```bash
go install golang.org/x/perf/cmd/benchstat@latest
```

Compare two benchmark outputs:

```bash
benchstat \
  docs/benchmarks/feature-before.txt \
  docs/benchmarks/feature-after.txt
```

## Reporting Format

Use this format when documenting results:

```markdown
## Benchmark: short name

Date:
Commit:
Machine:
Go version:
Command:

### Summary

- Result 1
- Result 2
- Tradeoff or caveat

### Raw Output

```text
paste benchmark output here
```
```

## Resume Metric Ideas

Use real measured values only. Good resume bullets may look like:

- Built a Go storage engine with benchmark coverage for `Set`, `Get`, `Delete`, and `Range`, tracking `ns/op`, `B/op`, and `allocs/op` across milestone releases.
- Implemented B+Tree-backed range scans and measured range-query latency before and after replacing the initial map-backed store.
- Added durable copy-on-write B+Tree persistence and measured synchronized write
  overhead plus recovery time.
- Added versioned concurrent access and validated race safety with
  `go test -race` while measuring reader scaling and write latency.

It is fine if a feature improves one metric and worsens another. Database engineering is about tradeoffs; document them clearly.
