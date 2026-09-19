# Database Architecture and Migration Plan

## Status and Scope

This document defines the target architecture for the embedded database and a
future shared LLM conversation service. It does not implement the refactor or
claim that planned packages already exist. Package extractions preserve public
behavior and file compatibility; correctness fixes and new features are separate
changes with their own tests.

The first service target is one process owning one database file, with a
TCP/TLS protocol. Browser access will use a later gateway. Snapshot readers with
one serialized writer precede independent writer branches. No WAL, LSM tree,
distributed replication, automatic text merging, or approximate vector index
is required for this migration.

## Current Implementation

- [db](../db/db.go) exposes in-memory and durable KV operations and transaction
  handles. Its table and index implementations own schemas and row encoding.
- [internal/btree](../internal/btree/kv.go) currently combines tree algorithms,
  page encoding, file management, metadata, free lists, commit, and recovery.
- [internal/sql](../internal/sql/executor.go) contains the lexer, parser, AST,
  and executor. The executor imports db; db imports internal/btree.
- There is no server, protocol client, conversation service, user authorization,
  snapshot registry, or live subscription implementation yet.

Current production-code package dependencies are acyclic:

```text
internal/sql -> db -> internal/btree
```

Durable writes use copy-on-write pages and alternating metadata generations.
The engine synchronizes data before publishing metadata and returns success
after metadata synchronization. This depends on the filesystem and device
honoring synchronization. Recovery validates candidate roots and can select
an older complete generation. Structural validation is not protection against
every possible corruption, and fallback is not a backup of all acknowledged data.

Buffered KV transactions and transactional table mutation methods exist. Reads
of unbuffered keys use current committed state, not a pinned snapshot. The
durable engine serializes reads and writes with a mutex. Direct write paths do
not uniformly participate in transaction admission; ordinary table/index
mutations and index backfill do not yet have universal atomicity. These are
correctness prerequisites, not guarantees supplied by a package rename.

Other current limits include no enforced exclusive file ownership, a decoding
path that can treat two invalid metadata slots as empty state, no checksums on
B+Tree data pages, a 3000-byte encoded-row/value limit, whole-tree loading on
open, and page-map copying during updates. Encoding and SQL validation findings
also require regression coverage before deployment.

See [durable storage](durable-storage.md), [transaction design](design/transactions.md),
and the [issue roadmap](design/shared-context-roadmap.md) for details.

## Target Package Responsibilities

These directories are created as working functionality is extracted or added.
Do not introduce empty packages solely to match this list.

```text
cmd/mydb-server/        process configuration, composition, startup, shutdown
internal/server/       TCP/TLS framing, authentication, dispatch, connection limits
internal/conversation/ permissions, messages, branches, generation jobs, context
internal/realtime/     bounded subscriptions, committed-event delivery and replay
internal/sql/          lexer, parser, AST, trusted SQL execution
db/                    public embedded API, tables, indexes, transaction facade
internal/engine/       durable KV coordination, writer admission, roots, snapshots
internal/btree/        node encoding, search, copy-on-write mutation, traversal
internal/storage/      file I/O, metadata encoding, allocation, free-list persistence
client/                public Go protocol client, no database implementation imports
```

The in-memory reference implementation remains behind db.New(). db.Open(path)
uses the extracted engine. The existing internal tree representation and naming
need not change merely to introduce storage boundaries.

## Dependency Direction

An arrow means a permitted production-code import, not the direction of every
runtime callback. Lower packages never import their callers.

```text
cmd/mydb-server -> internal/server, internal/conversation, db
internal/server -> internal/conversation, internal/realtime, internal/sql
internal/realtime -> internal/conversation
internal/conversation -> db
internal/sql -> db
db -> internal/engine
internal/engine -> internal/btree, internal/storage
internal/btree -> standard library
internal/storage -> standard library
client -> standard library and approved transport dependencies
```

The composition root constructs and closes the database, service, and listener.
Conversation mutations persist events through db, but never import realtime.
Realtime consumes committed events through conversation service methods, which
enforce authorization for replay and live delivery. This avoids an event/service
import cycle. SQL remains a trusted administrative interface; ordinary users
call conversation operations with explicit identity and permissions.

The client implements the versioned wire contract independently of server
internals. Protocol conformance tests keep both ends aligned. A shared protocol
package may be extracted later if duplication becomes material.

## Storage, Engine, and Transaction Ownership

- **Storage owns bytes and allocation mechanics.** It handles short reads/writes,
  file handles, exclusive ownership once implemented, metadata codecs, page
  reservation, and free-list persistence. It does not interpret B+Tree keys or
  decide which application transaction should commit.
- **B+Tree owns structural rules.** It encodes nodes, searches, splits, merges,
  and validates parent/child relationships. Narrow page callbacks supplied by
  engine read, allocate, and retire pages with explicit errors. Tree code does
  not open files, synchronize metadata, or immediately reuse retired pages.
- **Engine owns durable state transitions.** It binds callbacks, coordinates
  commit ordering and recovery, manages pending pages, and restores state after
  failures. It combines storage metadata validation with B+Tree reachability
  validation and authorizes reuse only after checking protected roots.
- **db owns the public contract.** Tables and indexes encode logical mutations;
  Tx provides the public lifecycle and hands one mutation batch to engine.
  Engine admission must eventually cover every durable write path, including
  convenience methods and schema changes. The in-memory backend must preserve
  equivalent public semantics without importing engine from lower layers.

The durable commit sequence remains: prepare replacement pages; write and sync
tree pages; write and sync free-list state; write and sync alternate metadata;
publish the committed in-memory root. A CPU atomic pointer swap alone is not a
durable commit. Synchronization errors can leave an uncertain on-disk outcome;
repair/recovery must resolve it before accepting another write. Preserve these
failure semantics and fault-injection hooks during extraction.

In the snapshot milestone, engine owns root handles and pin registration.
Acquiring a root and registering its pin must be atomic with respect to reuse.
Protect pages reachable from active snapshots, retained recovery roots, and
writer state; creation generation alone is not evidence that a page is free.
Release pins on transaction completion or cancellation. Define bounded snapshot
lifetimes and Close behavior before allowing readers to outlive an operation.

Keep existing locks during refactoring. The snapshot implementation must document
lock ordering and use short coordination sections; it must not hold storage locks
during network writes or model calls. Concurrent readers are a target, not a
claim of formally lock-free progress. Initial writers acquire admission before
reading mutable base state and remain serialized through commit or rollback.

## Conversation and Real-Time Ownership

Conversation records are ordered by server-assigned sequence within a branch.
Immutable application revisions reference chunked bodies; branching does not
require a physical database root for every message or a copy of all history.
Engine snapshots provide consistent reads, not automatic merging of edits.

The conversation service owns membership checks, request idempotency, sequence
allocation, generation attempts, and token-budgeted context assembly. Content,
index changes, retry results, and event records commit together. LLM calls run
outside transactions using a recorded input revision manifest.

Realtime owns delivery, not durability. It publishes only committed events,
supports replay within retention, and bounds subscriber queues. Delivery may
duplicate events; clients deduplicate using event identifiers. Slow clients are
disconnected with a replay/resync path rather than blocking commits. Persistent
conversation events are application data, not a storage recovery WAL.

## TCP/TLS Transport Contract

This is a planned protocol outline, not an executable server example.

- Frame each UTF-8 JSON payload with a four-byte unsigned big-endian payload
  length. The length excludes the prefix. Read exact lengths, handle partial
  writes, and reject zero or oversized frames before allocating the body.
- Include a protocol version, request ID, operation, and operation-specific
  payload. Responses echo request IDs and carry either a result or a structured
  error. Subscription events have an event sequence/cursor and a distinct type.
- Authenticate before executing commands. Shared service tokens do not provide
  individual user identity; conversation authorization applies to every command,
  replay, and subscription. Do not expose unrestricted SQL to browser users.
- Require TLS for deployed access. Any plaintext development mode must be an
  explicit loopback-only setting. Bound handshake, read, write, frame, connection,
  and request resources. Exact limits belong in the protocol implementation spec.
- Initially process one command at a time per connection. Route responses and
  notifications through one bounded outbound writer so frames cannot interleave.
  Multiple connections may run concurrently. Request IDs do not eliminate TCP
  head-of-line blocking; request multiplexing is deferred.
- On shutdown, stop admission, bound draining of active requests, stop delivery,
  and close the database after its users finish. A lost response is an uncertain
  client outcome, so mutating requests require scoped idempotency keys.

Illustrative application request and response, each carried inside a frame:

```json
{"version":1,"request_id":"r1","operation":"history","payload":{"conversation_id":"c1","limit":20}}
```

```json
{"version":1,"request_id":"r1","result":{"messages":[],"next_cursor":null}}
```

Field names are proposed, not a frozen public API. Finalize errors, authentication
exchange, cursor semantics, limits, and golden frame examples before implementing
the client. A later HTTPS/SSE or WebSocket gateway uses the same authorized
conversation service instead of duplicating transaction or permission rules.

## Compatibility Requirements

Preserve db.New(), db.Open(), existing method signatures, exported error behavior,
string-based keys, inclusive Range bounds, and ownership of Close during package
extractions. Keep SQL imports above db; adding db imports of internal/sql would
create a cycle. A future public SQL facade needs its own dependency design.

Do not change page size, offsets, encodings, checksums, or format versions as part
of package moves. The current documented versions are B+Tree 2, metadata 3, and
free-list 1 with 4096-byte pages. Regression fixtures must verify reopening old
files and unchanged logical contents. Separate encoding fixes may need versioned
migration or explicit rejection; never silently reinterpret old bytes.

Network protocol versioning is independent of disk format versioning. Neither
internal package extraction nor a network release implies a supported on-disk
upgrade path or compatibility guarantee beyond tested versions.

## Migration Stages and Exit Gates

1. **Document the target.** Review this ownership model and align roadmap links.
   Exit: current versus planned behavior and dependency direction are explicit.
2. **Repair correctness and capture baseline.** Add regression coverage for SQL,
   row/index encoding, writer admission, atomicity, file ownership, and recovery.
   Capture measurements before major changes; record necessary format changes
   separately. Exit: focused regressions pass and limitations are documented.
3. **Extract storage.** Move file/metadata/allocation code and its tests. Introduce
   only contracts needed to bridge packages. Exit: fake-page tree tests and
   temporary-file recovery tests pass with unchanged extraction behavior.
4. **Extract engine and clarify transactions.** Move durable KV orchestration and
   recovery, bind storage and tree callbacks, and redirect db's durable backend.
   Exit: compatible public behavior and fixture reopening; no import cycles.
5. **Add snapshots as a feature.** Implement pins, transaction-aware reads, and
   reader-safe reuse with serialized writers. Exit: deterministic interleaving,
   reclamation, cancellation, and restart tests pass; benchmarks compare baseline.
6. **Add the conversation service.** Implement records, chunks, permissions,
   idempotency, events, and retention behind the agreed contracts. Exit: retry,
   cross-tenant, large-message, and interrupted-commit tests pass.
7. **Add transport and deployment.** Implement the server, realtime delivery, and
   client, then the browser gateway and single-node deployment. Exit: framing,
   authentication, slow-client, reconnect, shutdown, backup, and restore tests
   pass. No replication, availability, or latency guarantee is inferred.

Use small PRs for each extraction; do not combine a file-format redesign with a
package move. Carry test coverage with the owning component and preserve old
baselines. Introduce a cache, WAL, or alternative index only through a separately
measured design decision.

## Verification

For implementation/refactoring stages, run from the repository root:

```sh
go list -deps ./...
go test ./...
go test -race ./...
go vet ./...
git diff --check
```

The compiler checks actual import cycles; reviewers also check the permitted
dependency direction above. Use fake page stores for tree tests, temporary
files and fault injection for storage/engine tests, and public API tests for db.
Transport tests add fragmented frames, oversized lengths, authorization denial,
duplicate requests, event replay, slow consumers, and graceful shutdown.

For documentation-only changes, check local links, fences, and Markdown source
structure. Do not claim runtime behavior was verified by a documentation edit.
See [profiling](profiling.md) and the [performance plan](benchmarks/performance-plan.md)
for repeatable measurements; profiles remain local and ignored.
