# Profiling GoDatabase

## Purpose

Use profiling to identify measured bottlenecks before changing the storage
engine. Keep profiling and benchmarking separate:

- Benchmarks measure how much time and memory an operation consumes.
- Profiles show where the program spends CPU time or allocates memory.

Capture a repeated benchmark baseline before implementing an optimization. See
the [performance measurement plan](benchmarks/performance-plan.md) for the
before-and-after workflow.

## Preparation

Run commands from the repository root. Generated artifacts belong in the
ignored `profiles/` directory.

```bash
mkdir -p profiles
go test ./...
```

List the available benchmarks:

```bash
go test ./db -run='^$' -bench='.' -benchtime=1x
```

`-run='^$'` skips ordinary tests so their work does not appear in the profile.
Use `-count=1` while profiling to produce one profile with an easy-to-understand
workload. Use repeated `-count` runs for benchmark comparisons instead.

Benchmark timers exclude fixture creation from reported `ns/op`, but CPU and
memory profilers observe the complete test process and may still include setup
samples. Use a sufficiently long `-benchtime` so the measured operation
dominates, and confirm suspicious setup functions against the benchmark code
before optimizing them.

## CPU Profiles

### In-memory operations

Profile the map-backed reference implementation:

```bash
go test ./db \
  -run='^$' \
  -bench='^BenchmarkInMemory(Get|Range)$' \
  -benchtime=10s \
  -count=1 \
  -cpuprofile=profiles/in-memory-cpu.pprof
```

### Durable reads and ranges

Profile B+Tree traversal, conversion, and range-result construction:

```bash
go test ./db \
  -run='^$' \
  -bench='^BenchmarkDurable(Get|Range)$' \
  -benchtime=10s \
  -count=1 \
  -cpuprofile=profiles/durable-read-cpu.pprof
```

### Durable mutations

Profile page encoding and copy-on-write mutation work:

```bash
go test ./db \
  -run='^$' \
  -bench='^BenchmarkDurable(Set|Overwrite|Delete)$' \
  -benchtime=3s \
  -count=1 \
  -cpuprofile=profiles/durable-write-cpu.pprof
```

Durable mutations spend substantial wall-clock time waiting for filesystem
synchronization. CPU profiles sample active CPU work and therefore do not fully
explain synchronized write latency. Use the execution-trace workflow below when
investigating blocked I/O.

## Inspecting CPU Profiles

Print the functions responsible for the most sampled CPU time:

```bash
go tool pprof -top profiles/durable-read-cpu.pprof
```

Open the interactive terminal:

```bash
go tool pprof profiles/durable-read-cpu.pprof
```

Useful interactive commands include:

```text
top
top -cum
list functionName
web
```

Open the browser interface on an automatically selected local port:

```bash
go tool pprof -http=:0 profiles/durable-read-cpu.pprof
```

The browser graph requires Graphviz for full graph rendering. The `top` and
source views work without it.

## Memory And Allocation Profiles

The Go memory profile can be viewed by currently retained memory (`inuse_space`)
or by all memory allocated during the run (`alloc_space`). Allocation profiles
are usually more useful for short database benchmarks.

### In-memory allocation profile

```bash
go test ./db \
  -run='^$' \
  -bench='^BenchmarkInMemoryRange$' \
  -benchtime=5s \
  -count=1 \
  -memprofile=profiles/in-memory-memory.pprof \
  -memprofilerate=1
```

### Durable allocation profile

```bash
go test ./db \
  -run='^$' \
  -bench='^BenchmarkDurable(Get|Range|Overwrite)$' \
  -benchtime=5s \
  -count=1 \
  -memprofile=profiles/durable-memory.pprof \
  -memprofilerate=1
```

`-memprofilerate=1` records every allocation for detail, but adds overhead. Do
not use timings from that run as benchmark results.

Inspect total allocated bytes:

```bash
go tool pprof -top -alloc_space profiles/durable-memory.pprof
```

Inspect the number of allocation objects:

```bash
go tool pprof -top -alloc_objects profiles/durable-memory.pprof
```

Inspect memory still live when the profile was written:

```bash
go tool pprof -top -inuse_space profiles/durable-memory.pprof
```

Open the allocation profile in a browser:

```bash
go tool pprof -alloc_space -http=:0 profiles/durable-memory.pprof
```

## Execution Trace For Durable Writes

Use a trace when investigating synchronization waits, scheduler delays, or lock
contention that a CPU profile does not represent:

```bash
go test ./db \
  -run='^$' \
  -bench='^BenchmarkDurableOverwrite/size=100$' \
  -benchtime=1s \
  -count=1 \
  -trace=profiles/durable-write.trace
```

Inspect it with:

```bash
go tool trace profiles/durable-write.trace
```

## Focused Benchmark Commands

Run only the in-memory reference benchmarks:

```bash
go test ./db -run='^$' -bench='^BenchmarkInMemory' -benchmem -count=5
```

Run only the public durable benchmarks:

```bash
go test ./db -run='^$' -bench='^BenchmarkDurable' -benchmem -count=5
```

Run one operation and dataset size:

```bash
go test ./db \
  -run='^$' \
  -bench='^BenchmarkDurableGet/size=1000$' \
  -benchmem \
  -count=5
```

Run a short smoke check of every benchmark definition:

```bash
go test ./db -run='^$' -bench='.' -benchmem -benchtime=1x
```

## Before-And-After Profiling

Profiles support an optimization decision, while repeated benchmarks establish
whether the change helped. Keep profiles local and commit the summarized result
with the benchmark comparison.

Capture profiles with meaningful names:

```bash
go test ./db -run='^$' -bench='^BenchmarkDurableRange$' \
  -benchtime=10s -count=1 \
  -cpuprofile=profiles/range-before-cpu.pprof
```

After the implementation, run the identical command with an `after` filename:

```bash
go test ./db -run='^$' -bench='^BenchmarkDurableRange$' \
  -benchtime=10s -count=1 \
  -cpuprofile=profiles/range-after-cpu.pprof
```

Compare where cumulative CPU time moved:

```bash
go tool pprof \
  -top \
  -base=profiles/range-before-cpu.pprof \
  profiles/range-after-cpu.pprof
```

Do not claim an improvement from profile percentages alone. Confirm it with
repeated benchmark results under the same machine, Go version, dataset, and
durability policy.
