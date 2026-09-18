# Secondary Index Performance Baseline

This document records the first performance checkpoint after adding secondary
index creation, backfill, maintenance, and indexed lookup. Compare it with
`docs/benchmarks/baseline-table.md`, which records the pre-index table baseline.

## Environment

- Date: 2026-09-18
- Platform: macOS Darwin, arm64
- CPU: Apple M1
- Go module: `godatabase`
- Benchmark package: `godatabase/db`
- Measurement flags: `-benchmem`

## Benchmark Command

Run from the repository root:

```bash
go test ./db -run '^$' \
  -bench '^Benchmark(InMemory|Durable)Table(CreateIndex|FindByIndex|IndexedInsert|IndexedUpdate|IndexedDelete)' \
  -benchmem -benchtime=100ms -count=5
```

Fixtures and index creation are outside the timed sections for lookup and
mutation benchmarks. `CreateIndex` intentionally measures metadata creation and
backfill. Durable mutations include page writes and synchronization.

## Recorded Measurement

The focused selective-lookup run used `-benchtime=20ms -count=1`, queried one
distinct indexed value (`value-0`), and used dataset size 10:

| Operation | Mode | Dataset | Time | Memory | Allocations |
| --- | --- | ---: | ---: | ---: | ---: |
| `FindByIndex` | In-memory | 10 | 1.162 us/op | 960 B/op | 17 allocs/op |
| `FindByIndex` | Durable | 10 | 1.709 us/op | 888 B/op | 24 allocs/op |

These are initial checkpoint values, not stable optimization claims. Run the
repeated command above before comparing a later implementation.

## Profile

Generate a comparable indexed-lookup profile locally:

```bash
mkdir -p profiles
go test ./db -run '^$' \
  -bench '^BenchmarkInMemoryTableFindByIndex/size=100$' \
  -benchtime=1s \
  -cpuprofile=profiles/index-before-cpu.pprof \
  -memprofile=profiles/index-before-mem.pprof
```

Inspect it with:

```bash
go tool pprof -top profiles/index-before-cpu.pprof
go tool pprof -top -alloc_space profiles/index-before-mem.pprof
```

Profiles are local, machine-specific artifacts and are ignored by Git.

## Current Interpretation

Indexed lookup is intended to be faster than a full scan for selective
predicates, but the current result still allocates because it:

- materializes every matching row as `map[string]any`;
- decodes the index result and then fetches and decodes the primary row;
- returns a complete slice instead of a streaming iterator.

Index maintenance also adds work to every insert, update, and delete. Durable
index creation and mutations additionally pay for copy-on-write page writes and
file synchronization.

## Future Comparisons

Repeat the same command after:

1. adding more index types or index range scans;
2. adding concurrent readers and writers;
3. adding batched or group commits;
4. replacing dynamic row maps with typed or lower-allocation rows;
5. adding streaming indexed iterators;
6. adding page caching or index-only projections.

Save outputs and compare them with:

```bash
benchstat secondary-index-before.txt secondary-index-after.txt
```

Keep the dataset sizes, selectivity, machine, Go version, durability mode, and
benchmark flags identical.
