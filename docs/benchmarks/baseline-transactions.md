# Transaction Performance Baseline

This document records transaction performance before snapshot isolation,
concurrent readers, and transaction-specific optimizations.

## Environment

- Date: 2026-09-18
- Platform: macOS Darwin, arm64
- CPU: Apple M1
- Go module: `godatabase`
- Benchmark package: `godatabase/db`
- Dataset sizes: 1, 10, and 100 rows

## Benchmark Command

```bash
go test ./db -run '^$' \
  -bench '^Benchmark(InMemory|Durable)Transaction' \
  -benchmem -benchtime=100ms -count=5
```

Fixtures are created outside timed sections. Commit benchmarks overwrite fixed
keys so the dataset does not grow during measurement. Transactional table
insert benchmarks intentionally add a new row per operation.

## Recorded Focused Results

The following results used dataset size 1, `-benchtime=20ms`, and `-count=1`:

| Operation | Mode | Time | Memory | Allocations |
| --- | --- | ---: | ---: | ---: |
| Read-only `Get` + rollback | In-memory | 167.6 ns/op | 96 B/op | 2 allocs/op |
| Read-only `Get` + rollback | Durable | 302.0 ns/op | 112 B/op | 4 allocs/op |
| One-write commit | In-memory | 1.707 us/op | 912 B/op | 11 allocs/op |
| One-write commit | Durable | 13.851 ms/op | 28,820 B/op | 39 allocs/op |
| Ten-write commit | In-memory | 17.805 us/op | 4,064 B/op | 66 allocs/op |
| Ten-write commit | Durable | 16.947 ms/op | 199,468 B/op | 135 allocs/op |
| Rollback | In-memory | 527.5 ns/op | 464 B/op | 4 allocs/op |
| Rollback | Durable | 305.4 ns/op | 464 B/op | 4 allocs/op |

These are baseline measurements, not production capacity claims. Durable write
latency includes copy-on-write page writes and synchronization.

## Table and Index Transaction Control

The same benchmark suite includes transactional table/index inserts:

```bash
go test ./db -run '^$' \
  -bench '^Benchmark(InMemory|Durable)TransactionTableInsert' \
  -benchmem -benchtime=100ms -count=5
```

This measures row encoding, index maintenance, transaction buffering, and the
single commit together. Compare it with the non-transactional table and index
baselines in `baseline-table.md` and `baseline-secondary-index.md`.

## Profiles

Baseline profiles are stored locally under the ignored `profiles/` directory:

- `profiles/transaction-before-cpu.pprof`
- `profiles/transaction-before-mem.pprof`
- `profiles/transaction-before-durable-cpu.pprof`

Regenerate them with:

```bash
mkdir -p profiles
go test ./db -run '^$' \
  -bench '^BenchmarkInMemoryTransactionMultiWriteCommit/size=1$' \
  -benchtime=1s \
  -cpuprofile=profiles/transaction-before-cpu.pprof \
  -memprofile=profiles/transaction-before-mem.pprof

go test ./db -run '^$' \
  -bench '^BenchmarkDurableTransactionCommit/size=1$' \
  -benchtime=300ms \
  -cpuprofile=profiles/transaction-before-durable-cpu.pprof
```

Inspect them with:

```bash
go tool pprof -top profiles/transaction-before-cpu.pprof
go tool pprof -top -alloc_space profiles/transaction-before-mem.pprof
go tool pprof -top profiles/transaction-before-durable-cpu.pprof
```

The main current costs are expected to be map copying for in-memory commits,
B+Tree copy-on-write work, page writes, and `fsync` for durable commits.

## Future Comparisons

Repeat the same command after:

1. snapshot isolation and root pinning;
2. concurrent readers;
3. batched or group commits;
4. transaction-aware table/index optimizations;
5. reduced-allocation transaction buffers.

Save benchmark output and compare with:

```bash
benchstat transaction-before.txt transaction-after.txt
```

Keep dataset sizes, transaction sizes, durability mode, machine, Go version,
and benchmark flags identical.
