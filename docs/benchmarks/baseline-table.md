# Table Layer Performance Baseline

This document records the table-layer baseline before secondary indexes,
versioned concurrency, and storage optimizations are implemented.

## Environment

- Date: 2026-09-17
- Platform: macOS Darwin, arm64
- CPU: Apple M1
- Go module: `godatabase`
- Benchmark package: `godatabase/db`
- Measurement flags: `-benchmem`
- Dataset sizes: 10, 100, and 1,000 rows

## Benchmark Command

Run from the repository root:

```bash
go test ./db -run '^$' \
  -bench '^Benchmark(InMemory|Durable)Table' \
  -benchmem -benchtime=100ms -count=5
```

The benchmark creates and seeds fixtures before `b.ResetTimer()`. The timed
sections measure table operations, not fixture creation. Durable writes still
include the storage engine's page writes and synchronization because those are
part of the public operation's durability guarantee.

## Recorded Results

These are representative local measurements. Durable writes are especially
variable because they depend on filesystem synchronization and the benchmark
uses temporary files.

### In-memory table operations

| Operation | Dataset | Time | Memory | Allocations |
| --- | ---: | ---: | ---: | ---: |
| Insert | 10 | 1.35 us/op | 472 B/op | 11 allocs/op |
| Insert | 100 | 1.40 us/op | 446 B/op | 11 allocs/op |
| Get | 10-1,000 | 0.85-1.28 us/op | 472-477 B/op | 9 allocs/op |
| Range | 10 | 7.7-9.7 us/op | 4,528 B/op | 55 allocs/op |
| Range | 100 | 193-303 us/op | 43,216 B/op | 415 allocs/op |
| Range | 1,000 | 2.6-6.5 ms/op | 431,568 B/op | 4,759 allocs/op |

### Durable table operations

| Operation | Dataset | Time | Memory | Allocations |
| --- | ---: | ---: | ---: | ---: |
| Insert | 10 | 26.99 ms/op | 39,000 B/op | 51 allocs/op |
| Insert | 100 | 37.98 ms/op | 59,672 B/op | 58 allocs/op |
| Get | 10 | 1.0-1.4 us/op | 568 B/op | 11 allocs/op |
| Get | 100 | 2.7-6.2 us/op | 568 B/op | 11 allocs/op |

The durable range benchmark was run, but the temporary-file setup and large
fixture commits made the short local run too noisy to record as a stable
comparison number. Rerun the standard command above when comparing a future
implementation.

## Profile Findings

Baseline profiles are saved locally in the ignored `profiles/` directory:

- `profiles/table-before-range-cpu.pprof`
- `profiles/table-before-range-mem.pprof`
- `profiles/table-before-durable-get-cpu.pprof`

Regenerate comparable profiles with:

```bash
mkdir -p profiles
go test ./db -run '^$' \
  -bench '^BenchmarkInMemoryTableRange/size=1000$' \
  -benchtime=1s \
  -cpuprofile=profiles/table-before-range-cpu.pprof \
  -memprofile=profiles/table-before-range-mem.pprof

go test ./db -run '^$' \
  -bench '^BenchmarkDurableTableGet/size=100$' \
  -benchtime=300ms \
  -cpuprofile=profiles/table-before-durable-get-cpu.pprof
```

Inspect them with:

```bash
go tool pprof -top profiles/table-before-range-cpu.pprof
go tool pprof -top -alloc_space profiles/table-before-range-mem.pprof
go tool pprof -http=:0 profiles/table-before-durable-get-cpu.pprof
```

A focused in-memory 1,000-row range profile showed the primary costs were:

- `decodeRow`: approximately 85% of measured allocation volume through row
  maps and decoded values;
- `Database.rangeLocked`: map-key collection and sorting;
- `decodeValue` and `decodePrimaryKey`: per-field and primary-key allocations.

A durable read profile also showed time in `KV.Get` and page-tree traversal.
Durable write profiles are expected to include `os.File.Sync`; removing that
cost would change the durability guarantee rather than simply optimize it.

## Future Comparison Checkpoints

Run the same command and record a new section after each change:

1. Secondary indexes: compare insert cost, primary-key lookup, and range scans.
2. Versioned concurrent readers and writers: compare single-thread latency and
   add separate throughput benchmarks for concurrent workloads.
3. Batched or group commits: compare durable write latency and explicitly record
   the changed durability boundary.
4. Typed rows or reduced-allocation decoding: compare `B/op` and `allocs/op`.
5. Streaming range iterators: compare range latency and peak memory usage.
6. Page caching or lazy loading: compare durable reads and restart behavior.

Use `benchstat` to compare saved outputs:

```bash
benchstat before.txt after.txt
```

Keep dataset sizes, machine, Go version, benchmark flags, and durability mode
identical. Do not claim an improvement from one noisy durable run; use repeated
runs and report the median or `benchstat` result.
