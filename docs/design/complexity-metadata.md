# Complexity Metadata

The experiment API exposes theoretical complexity metadata for each supported
structure and operation. This metadata explains asymptotic behavior; it is not
a substitute for benchmark measurements such as `ns/op`, `B/op`, or
`allocs/op`.

Supported operations are `Set`, `Get`, `Delete`, and inclusive `Range`. For
range operations, `k` is the number of returned entries, `n` is the number of
stored entries, and `B` is the page fanout for the durable B+Tree.

The map and sorted-slice implementations are reference implementations. The
in-memory and durable B+Trees model balanced-tree behavior. Durable metadata
describes page complexity and intentionally excludes variable filesystem and
`fsync` latency, which must be measured by durable benchmarks.

Example response metadata:

```json
{
  "structure": "btree-memory",
  "operation": "range",
  "complexity": {
    "best": "O(log n + k)",
    "average": "O(log n + k)",
    "worst": "O(log n + k)",
    "memory": "O(n + k)",
    "assumptions": "k is the number of returned entries and leaves are ordered."
  }
}
```

Measured results should be returned in separate fields and labeled with the
dataset size, workload, seed, Go version, and whether the implementation is
durable. This keeps observed performance from being mistaken for a universal
complexity guarantee.
