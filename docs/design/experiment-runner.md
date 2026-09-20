# Experiment execution

Each run creates an isolated map, sorted slice, in-memory B+Tree, or durable
B+Tree through an injected Factory. The experiment package owns request and
result models; HTTP models are aliases. Controllers do not own structure logic.

The executor serializes its runs and checks cancellation between operations.
Limits are 5,000 initial records (1,000 for durable runs), 128 operations,
256-byte keys and bounds, and 1,024-byte values. A 30-second deadline is checked
between operations; an operating-system fsync cannot be interrupted mid-call.
Durable files are closed and removed even after an operation fails.

Latency covers the operation loop, including result construction, but excludes
dataset setup, factory opening, cleanup, and memory-counter reads. Allocation
deltas are process-wide estimates and can include unrelated runtime activity.
They are not isolated per-request heap measurements or peak memory usage.
Durable writes include synchronization. The durable implementation caches
committed pages in memory, so reads are not cold-disk benchmarks.

Use go benchmarks for controlled performance comparisons. HTTP measurements
describe a single execution; they do not establish asymptotic complexity.
