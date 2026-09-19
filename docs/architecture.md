# Educational Database Lab Architecture

## Status and Scope

This document defines the target architecture for an educational database and
data-structure laboratory. The project remains a Go database implementation,
but its primary user experience is comparing data structures, measuring their
behavior, and tracing how operations execute.

The target is a small, reproducible single-process application. It is not a
distributed database, hosted LLM context service, or production replacement
for Redis, MySQL, or an LSM-based system.

This document distinguishes current database guarantees from planned
educational capabilities. A package should not be created merely to match this
list; introduce it when a real dependency or test boundary requires it.

## Current Guarantees

The current repository provides:

- `db.New()` for the in-memory key-value implementation.
- `db.Open(path)` for the durable public database API.
- Set, Get, Delete, and inclusive Range behavior with documented validation.
- Tables, primary-key lookup, table scans, secondary indexes, and transactions.
- A page-backed B+Tree with copy-on-write updates and recovery tests.
- SQL lexing, parsing, validation, and execution for the documented SQL subset.
- Go tests, race tests, vet checks, linting, CodeQL, and benchmark coverage.

Durable writes use fixed-size pages, metadata generations, copy-on-write
updates, and synchronized commits. Snapshot isolation, a buffer pool, an LSM
tree, vector search, replication, and a multi-user network service are planned
features, not current guarantees. See [`docs/design`](design/) for detailed
limitations and format decisions.

## Target Package Responsibilities

```text
cmd/lab-server/       process configuration and HTTP server startup
internal/structures/  common adapters and future structure implementations
internal/btree/       B+Tree nodes, search, mutation, and page traversal
internal/storage/     future file, page, metadata, and free-list boundary
internal/engine/      experiment execution and structure selection
internal/metrics/     timing, allocation, benchmark, and complexity results
internal/trace/       structured execution events and trace collection
internal/sql/         lexer, parser, AST, and SQL execution adapter
db/                   public embedded database, tables, indexes, and Tx API
internal/server/      HTTP handlers, validation, and response mapping
web/                  browser client and visualization
```

The existing `internal/btree` and `db` packages remain the source of truth while
these boundaries are introduced incrementally. A future `internal/lsm`
implementation can satisfy the same structure interface without changing the
experiment engine or frontend.

### Structures

The initial implementations are a Go map, sorted slice, in-memory B+Tree, and
durable B+Tree. A future LSM tree is a separately designed and measured
implementation. Every adapter exposes the same logical operations: `Set`,
`Get`, `Delete`, and `Range`. Structure-specific details appear as trace events
and metrics instead of leaking into the common API.

### Engine

The experiment engine selects a structure, loads a deterministic dataset, runs
a workload, and returns logical results. It coordinates metrics and tracing,
but does not implement B+Tree algorithms or file formats. Each run accepts a
seed, dataset size, and operation sequence so it can be reproduced locally or
through HTTP.

### Metrics

Metrics record elapsed time, allocations, bytes allocated, operation counts,
and structure-specific counters when available. The layer also exposes the
documented theoretical time and memory complexity of each operation. Measured
performance is evidence for one workload and environment, not a universal
complexity claim.

### Tracing

Tracing records ordered, serializable events such as operation selection, key
comparison, node traversal, page access, split, merge, and result delivery.
Tracing is optional, must not change logical results, and should avoid work
when disabled. It belongs behind a narrow event-sink interface rather than
coupling structure algorithms to the frontend.

### Server and Frontend

The server exposes experiments over HTTP/JSON. It validates requests, applies
resource limits, invokes the experiment engine, and maps errors to responses.
It must not expose internal nodes, pages, locks, or database implementation
types. The frontend chooses a structure, workload, dataset size, and seed, then
displays results, complexity, metrics, and trace events. It consumes HTTP and
does not import Go packages.

The initial service is HTTP-only. TLS, authentication, subscriptions, and a
separate client protocol are later concerns and must not complicate the first
educational release.

## Dependency Direction

Production dependencies point downward toward simpler layers:

```text
cmd/lab-server -> internal/server -> internal/engine
web             -> HTTP API
internal/engine -> internal/structures, internal/metrics, internal/trace
internal/sql    -> db
db              -> internal/btree
internal/btree  -> standard library
internal/storage -> standard library
```

Rules that prevent cycles:

- Structures never import the server, frontend, metrics implementation, or SQL.
- Metrics and tracing depend on small interfaces or standard-library types, not
  concrete B+Tree or database packages.
- The engine depends on structure interfaces and observers, never on HTTP.
- HTTP handlers depend on the engine, never on internal node or page types.
- `internal/sql` may call `db`; `db` must not import SQL.
- The composition root creates concrete implementations and wires dependencies.

Page, allocation, metrics, and tracing callbacks are dependency-injection
boundaries; they do not reverse the package import graph.

## HTTP Experiment Boundary

The initial API is a small JSON contract. An illustrative request is:

```json
{
  "structure": "btree-memory",
  "seed": 7,
  "dataset_size": 1000,
  "operations": [
    {"op": "set", "key": "k0001", "value": "v"},
    {"op": "get", "key": "k0001"}
  ],
  "trace": true
}
```

An illustrative response is:

```json
{
  "results": [{"found": true, "value": "v"}],
  "complexity": {"get": "O(log n) expected"},
  "metrics": {"ns_per_op": 1200, "allocs_per_op": 2},
  "trace": [{"kind": "lookup", "key": "k0001"}]
}
```

These fields are examples until the API issue defines the versioned schema.
Requests must have bounded dataset sizes, operation counts, key sizes, and
trace output. Invalid input returns a structured error without panicking.

## Migration Stages

1. **Architecture and contracts.** Finalize this document, define the common
   operation interface, and record current versus planned behavior.
2. **Reference structures.** Adapt the map, sorted slice, in-memory B+Tree,
   and durable B+Tree behind the same interface.
3. **Metrics and workloads.** Add deterministic workloads, complexity metadata,
   benchmarks, allocation measurements, and reproducible result documents.
4. **Tracing.** Add optional structured events and deterministic ordering tests.
5. **Experiment API.** Add HTTP validation, execution, metrics, trace, and error
   responses without exposing internal node types.
6. **Frontend.** Build the interactive experiment view against the HTTP API.
7. **Deployment and polish.** Add health checks, deployment instructions,
   end-to-end tests, architecture documentation, and release notes.

Storage hardening, page/B+Tree separation, transaction boundaries, and recovery
tests remain database-engine work. They can proceed independently, but do not
block a first in-memory educational demonstration. Snapshot isolation, LSM
compaction, vector indexes, and distributed deployment require separate design
and measurement before implementation.

## Verification

Run from the repository root:

```sh
go list -deps ./...
go test ./...
go test -race ./...
go vet ./...
```

For benchmark work:

```sh
go test -bench=. -benchmem -count=5 ./...
```

Keep benchmark output and profiles under ignored local paths. Record the
workload, Go version, operating system, CPU, dataset size, and command beside
any result used for comparison.
