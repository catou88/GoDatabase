# Map-Backed Database Performance Baseline

## Environment

- Date: 2026-09-16
- Database implementation: in-memory Go map
- Machine: Apple M1
- Operating system: macOS (`darwin/arm64`)
- Go version: `go1.27.0`
- Command: `go test -bench=. -benchmem ./...`

These results measure the current map-backed `db.Database`. They do not include
the B+Tree, disk persistence, synchronization, transactions, or network costs.

## Baseline Stage

This is the in-memory reference baseline captured before the planned concurrency
and storage-engine experiments. In particular, it predates:

- snapshot isolation and explicitly versioned B+Tree roots;
- concurrent readers that traverse snapshots without a tree-wide mutex;
- transaction batching and group commit;
- synchronization-path and one-sync commit experiments;
- a write-ahead log or asynchronous durability modes;
- write-amplification and allocation optimization;
- any LSM-tree implementation or B+Tree-versus-LSM comparison.

Keep this baseline unchanged. Record each future architecture under a new dated
result file rather than replacing these measurements.

## Methodology

- Each operation is measured with databases containing 10, 1,000, and 100,000
  keys.
- `Set` overwrites existing keys so the dataset size remains stable.
- `Get` reads existing keys.
- `Delete` removes existing keys; restoring the dataset is performed outside
  the timed region.
- `Range` scans the full requested dataset and includes sorting and result
  construction.
- Dataset construction is performed outside the timed regions.
- Memory allocations are reported by the Go benchmark runner.

## Summary

| Operation | 10 keys | 1,000 keys | 100,000 keys |
| --- | ---: | ---: | ---: |
| Set | 31.38 ns/op | 31.55 ns/op | 73.85 ns/op |
| Get | 22.35 ns/op | 28.10 ns/op | 45.65 ns/op |
| Delete | 67.25 ns/op | 61.65 ns/op | 142.4 ns/op |
| Range | 1.127 us/op | 216.901 us/op | 69.154 ms/op |

## Allocation Findings

Point operations perform no measured heap allocations. There are no obvious
allocation reductions to make in the current `Set`, `Get`, or `Delete` paths.

`Range` is the high-allocation path:

- 10 keys: 1,152 B/op and 6 allocs/op
- 1,000 keys: 86,624 B/op and 12 allocs/op
- 100,000 keys: 19,370,592 B/op and 30 allocs/op

The map-backed implementation must first collect every map key, sort the key
slice, and build a separate result slice. The result slice also grows through
multiple allocations because its final capacity is not known up front. A
future B+Tree range scan should avoid collecting and sorting all keys by walking
ordered leaf nodes directly. Result construction will still allocate unless a
streaming iterator or caller-provided buffer is introduced.

## Raw Output

```text
goos: darwin
goarch: arm64
pkg: godatabase/db
cpu: Apple M1
BenchmarkDatabaseSet/size=10-8          34595890        31.38 ns/op          0 B/op       0 allocs/op
BenchmarkDatabaseSet/size=1000-8        35104183        31.55 ns/op          0 B/op       0 allocs/op
BenchmarkDatabaseSet/size=100000-8      20800816        73.85 ns/op          0 B/op       0 allocs/op
BenchmarkDatabaseGet/size=10-8          56403972        22.35 ns/op          0 B/op       0 allocs/op
BenchmarkDatabaseGet/size=1000-8        57997056        28.10 ns/op          0 B/op       0 allocs/op
BenchmarkDatabaseGet/size=100000-8      38246503        45.65 ns/op          0 B/op       0 allocs/op
BenchmarkDatabaseDelete/size=10-8       15892107        67.25 ns/op          0 B/op       0 allocs/op
BenchmarkDatabaseDelete/size=1000-8     18449685        61.65 ns/op          0 B/op       0 allocs/op
BenchmarkDatabaseDelete/size=100000-8   12481077       142.4 ns/op           0 B/op       0 allocs/op
BenchmarkDatabaseRange/size=10-8         1000000      1127 ns/op          1152 B/op       6 allocs/op
BenchmarkDatabaseRange/size=1000-8          5496    216901 ns/op         86624 B/op      12 allocs/op
BenchmarkDatabaseRange/size=100000-8          21  69153768 ns/op      19370592 B/op      30 allocs/op
PASS
```

## Comparison Guidance

Future results should use the same benchmark definitions, machine, Go version,
and command where practical. Run benchmarks multiple times and compare them
with `benchstat` before making performance claims. Differences from this
baseline will show the cost or benefit of replacing the map with the B+Tree,
adding persistence, and introducing concurrency control.
