# Durable Database Pre-Optimization Baseline

## Environment

- Date: 2026-09-17
- Commit: `704850a`
- Branch: `93-document-durable-storage-guarantees`
- Public API: `db.Open` and exported `Database` operations
- Storage engine: page-backed copy-on-write B+Tree
- Machine: Apple M1
- Operating system: macOS (`darwin/arm64`)
- Go version: `go1.27.0`
- Command: `go test -bench=. -benchmem ./...`

## Baseline Stage

These measurements describe the correctness-first durable implementation before
the planned concurrency and write-path changes. The implementation currently
loads committed pages during `Open`, serializes durable operations with a mutex,
and synchronizes B+Tree pages, free-list pages, and root metadata during each
mutation.

This baseline was captured before:

- snapshot isolation and explicitly versioned B+Tree roots;
- concurrent snapshot readers that do not retain the current operation mutex;
- concurrently prepared private writer versions;
- multi-operation transactions, batching, or group commit;
- reducing or combining synchronization barriers;
- configurable synchronous, asynchronous, or manual durability modes;
- a write-ahead log and background checkpointing;
- write-amplification, page-encoding, and allocation optimization;
- page-cache policy changes or lazy page loading;
- any LSM-tree implementation or B+Tree-versus-LSM benchmark.

Future results must identify which of these changes are active. Buffered writes
must not be compared with synchronized writes as though they provide the same
durability guarantee.

## Methodology

- Durable datasets contain 10, 100, and 1,000 records.
- `DurableSet` inserts new keys from the named starting dataset size.
- `DurableOverwrite` updates existing keys.
- `DurableGet` reads existing keys.
- `DurableDelete` restores each key outside the timed region before measuring
  deletion.
- `DurableRange` scans the complete dataset.
- `DurableCloseOpen` intentionally measures both lifecycle operations.
- Fixture creation and key generation are outside timed regions.
- `B/op` and `allocs/op` are reported by the Go benchmark runner.
- The in-memory benchmark suite remains the reference for API overhead without
  persistence and synchronization.

## Durable Summary

| Operation | 10 records | 100 records | 1,000 records |
| --- | ---: | ---: | ---: |
| Set new key | 12.237 ms/op | 15.342 ms/op | 17.078 ms/op |
| Overwrite | 14.251 ms/op | 12.211 ms/op | 12.110 ms/op |
| Get | 248 ns/op | 1.223 us/op | 1.713 us/op |
| Delete | 31.229 ms/op | 12.820 ms/op | 12.152 ms/op |
| Range | 2.589 us/op | 30.920 us/op | 320.817 us/op |
| Close and Open | 70.212 us/op | 69.434 us/op | 113.997 us/op |

## Allocation Summary

| Operation | 10 records | 100 records | 1,000 records |
| --- | ---: | ---: | ---: |
| Set new key | 38,830 B / 40 allocs | 65,137 B / 52 allocs | 77,202 B / 58 allocs |
| Overwrite | 38,664 B / 40 allocs | 38,664 B / 40 allocs | 65,048 B / 55 allocs |
| Get | 16 B / 2 allocs | 16 B / 2 allocs | 16 B / 2 allocs |
| Delete | 30,071 B / 36 allocs | 30,456 B / 38 allocs | 43,856 B / 48 allocs |
| Range | 2,872 B / 47 allocs | 28,552 B / 410 allocs | 249,480 B / 4,014 allocs |
| Close and Open | 55,856 B / 58 allocs | 55,856 B / 58 allocs | 90,808 B / 83 allocs |

## Initial Observations

- Durable mutations are dominated by synchronized commit work and currently
  take approximately 12-31 milliseconds per operation.
- `Get` traverses pages loaded in memory during `Open`; it does not represent an
  uncached physical disk read for every call.
- Range time and allocations grow approximately with the number of returned
  entries.
- Close/Open cost grows as recovery validates and loads more committed pages.
- A single benchmark run is evidence of scale, not a statistically rigorous
  regression result. Use repeated runs and `benchstat` for claims.

## Raw Durable Output

```text
BenchmarkDurableSet/starting_size=10-8          99   12237455 ns/op   38830 B/op    40 allocs/op
BenchmarkDurableSet/starting_size=100-8         98   15341664 ns/op   65137 B/op    52 allocs/op
BenchmarkDurableSet/starting_size=1000-8       100   17078307 ns/op   77202 B/op    58 allocs/op
BenchmarkDurableOverwrite/size=10-8             99   14251082 ns/op   38664 B/op    40 allocs/op
BenchmarkDurableOverwrite/size=100-8            94   12210682 ns/op   38664 B/op    40 allocs/op
BenchmarkDurableOverwrite/size=1000-8          100   12109998 ns/op   65048 B/op    55 allocs/op
BenchmarkDurableGet/size=10-8              4475322        248.0 ns/op     16 B/op     2 allocs/op
BenchmarkDurableGet/size=100-8             1006604       1223 ns/op       16 B/op     2 allocs/op
BenchmarkDurableGet/size=1000-8             639064       1713 ns/op       16 B/op     2 allocs/op
BenchmarkDurableDelete/size=10-8                33   31229340 ns/op   30071 B/op    36 allocs/op
BenchmarkDurableDelete/size=100-8               99   12820271 ns/op   30456 B/op    38 allocs/op
BenchmarkDurableDelete/size=1000-8              99   12152407 ns/op   43856 B/op    48 allocs/op
BenchmarkDurableRange/size=10-8             470424       2589 ns/op     2872 B/op    47 allocs/op
BenchmarkDurableRange/size=100-8             69412      30920 ns/op    28552 B/op   410 allocs/op
BenchmarkDurableRange/size=1000-8             6487     320817 ns/op   249480 B/op  4014 allocs/op
BenchmarkDurableCloseOpen/size=10-8           17598      70212 ns/op   55856 B/op    58 allocs/op
BenchmarkDurableCloseOpen/size=100-8          16875      69434 ns/op   55856 B/op    58 allocs/op
BenchmarkDurableCloseOpen/size=1000-8         10000     113997 ns/op   90808 B/op    83 allocs/op
```

## Future Comparison Labels

Use separate result files and identify the architecture explicitly, for example:

- `snapshot-readers-before.md` and `snapshot-readers-after.md`
- `group-commit-before.md` and `group-commit-after.md`
- `sync-path-before.md` and `sync-path-after.md`
- `btree-vs-lsm.md`

Run each comparison multiple times with the same machine, Go version, dataset,
durability mode, and benchmark command.
