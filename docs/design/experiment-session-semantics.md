# Experiment Session Semantics

An experiment session is a reproducible description of one data-structure
workload. Sessions are stateless: the server may execute them without keeping
session data after the response, and a client can replay a session by sending
the same request again.

## Session Request

The request contains the selected structure, a deterministic workload, dataset
size, seed, and whether trace events should be collected:

```json
{
  "structure": "btree-memory",
  "dataset_size": 100,
  "seed": 42,
  "trace": true,
  "operations": [
    {"name": "set", "key": "alpha", "value": "one"},
    {"name": "get", "key": "alpha"},
    {"name": "range", "start": "a", "end": "z"}
  ]
}
```

`structure` must name a supported implementation: `map`, `sorted-slice`,
`btree-memory`, or `btree-durable`. Each operation is one of `set`, `get`,
`delete`, or `range`. Set, get, and delete require a non-empty key. Set may
include an empty value. Range bounds are inclusive and `start` must not be
greater than `end`.

`dataset_size` is a non-negative number of generated records. `seed` is a
non-negative integer used for every generated key, value, and operation choice.
The same structure, request, implementation version, dataset size, and seed
must produce the same logical workload. The server applies a documented limit
to dataset size and operation count before execution.

## Response Schema

The response identifies the selected structure and returns one result per
requested operation:

```json
{
  "structure": "btree-memory",
  "results": [
    {"found": true, "value": "one"},
    {"count": 1}
  ],
  "complexity": [
    {
      "structure": "btree-memory",
      "operation": "get",
      "complexity": {
        "best": "O(1)",
        "average": "O(log n)",
        "worst": "O(log n)",
        "memory": "O(n)"
      }
    }
  ],
  "metrics": {
    "duration_ns": 1200,
    "bytes_allocated": 256,
    "allocations": 4
  },
  "trace": []
}
```

`complexity` contains theoretical claims and assumptions. `metrics` contains
measurements from this run and must not be presented as a complexity proof.
Measured values are allowed to vary with hardware, Go version, filesystem,
warm-up, and scheduler behavior. A missing measurement is represented by zero
until the runner collects it.

## Trace Events

When `trace` is true, events are returned in emission order. Each event has a
monotonic `sequence` assigned by the recorder:

```json
{
  "sequence": 3,
  "type": "traversal",
  "operation": "get",
  "structure": "btree-memory",
  "node_id": 4,
  "detail": "descend to child"
}
```

Supported event types are `operation_selected`, `lookup`, `comparison`,
`traversal`, and `result_delivered`. `node_id` and `page_id` are optional and
are included only when the implementation has those identifiers. Trace events
are observational and must not change logical results.

## Validation and Errors

Invalid JSON, unknown fields, unsupported structures or operations, negative
seeds, reversed range bounds, empty required keys, and resource-limit
violations fail before the runner is called. The HTTP API returns a stable
public error code such as `invalid_request`; it does not expose storage paths,
checksums, page contents, or stack traces. Internal details belong in server
logs and metrics.

## Replay Requirements

To replay a session, persist the complete request JSON, implementation version,
Go version, and environment description. Replay must use the same operation
ordering and seed. Logical results and trace event ordering must match. Timing,
allocation counts, page-cache state, and durable I/O latency are measurements,
so they may differ between runs and should be compared statistically rather
than byte-for-byte.

The first implementation does not promise cross-version or cross-platform
byte-identical traces. A future session format version must be added before
changing operation encoding, random generation, validation semantics, or event
names.
