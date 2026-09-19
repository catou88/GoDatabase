# Archived: Shared LLM Context Roadmap and GitHub Issues

> **Deprecated.** This document is retained as historical planning material
> only. The active project goal is the educational database lab described in
> [the architecture document](../architecture.md); this roadmap is not an
> implementation plan or a statement of current guarantees.

## Purpose and Status

Build a shared AI workspace where users resume conversations, branch prompts,
trace generated answers to source revisions, and control context costs.
This is planned work, not a statement of implemented guarantees. The current
Go copy-on-write B+Tree is the foundation; correctness repairs precede deployment.
Inspection findings require regression tests, not assumptions based on existing
green CI. No latency, cost, or hiring outcome is promised.

## Target Architecture First

The [Architecture and Migration Plan](../architecture.md) is the authoritative
package ownership and dependency design. This roadmap tracks implementation work.

Document the target architecture before implementation. Preserve the existing
public API and file format during package extractions. Fix known correctness
defects with regression tests before moving the affected code. The layout below
is a target, not a claim that these packages already exist. Create packages when
they acquire an implemented responsibility, not as empty scaffolding.

```text
cmd/mydb-server/       configuration, startup, shutdown
internal/server/      TCP/TLS framing, authentication, deadlines, connections
internal/conversation/ messages, branches, permissions, generation jobs
internal/realtime/    subscriptions, durable event replay, bounded delivery
internal/sql/         lexer, AST, parser, executor
internal/engine/      durable KV commit/recovery, writer admission, snapshot roots
internal/storage/     page I/O, metadata, recovery, allocation, free lists
internal/btree/       copy-on-write tree operations and node encoding
db/                   public embedded API, tables, transaction coordination
client/               Go TCP client when the protocol is implemented
```

Dependency direction: server calls conversation operations or trusted SQL;
conversation and SQL use db; db uses engine; engine coordinates storage and btree.
BTree operations use narrow page-access callbacks supplied by engine and never
import storage, db, or server. Storage never imports btree; engine coordinates
tree reachability and metadata validation to prevent an import cycle.
Realtime reads committed events through the application service; it cannot
publish uncommitted state or bypass authorization. The client depends on the
wire contract, not internal storage types. A future browser gateway calls the
same authorized application operations.

Engine owns snapshot handles and publication ordering behind the public db.Tx
facade; storage owns allocation mechanics and performs engine-authorized reuse.
Their explicit contract protects pages
reachable from active snapshots, writer state, and retained recovery roots.
Define acquisition, release, cancellation, Close, and reclamation behavior
before removing coarse locks. A buffer package is deferred until there is an
actual bounded page cache. Network writes and LLM calls never hold storage locks.

The execution order is A1, correctness issues 1-4 and baseline 5, then A2-A3,
then feature work below. A4 begins after conversation semantics issue 6 and
before server implementation. These architecture issues supplement the existing
backlog without renumbering it.

## Architecture Decisions

- Canonical messages are ordered within a conversation branch using server-assigned
  sequences. Similarity is a retrieval index, never the canonical message order.
- Keep immutable message revisions and explicit branches. Automatic text merging
  and optimistic conflict detection are not v1 requirements.
- Application conversation branches are distinct from retained physical tree roots.
  Branches reference existing immutable message bodies rather than copy transcripts.
- Start with one serialized writer and concurrent pinned snapshot readers.
  Acquire writer admission before reading mutable base state. Independent writer
  versions are a later design with explicit publication and retention rules.
- Keep the public embedded API. Add a conversation service enforcing identity,
  permissions, idempotency, and bounded work. Unrestricted SQL is for trusted tools.
- The first server uses TCP/TLS with a 4-byte big-endian length prefix and
  versioned JSON payloads, bounded frames, request IDs, and authentication.
  Service authentication does not replace individual user authorization.
  Browser access comes later through an HTTPS/SSE or WebSocket gateway using
  the same conversation operations. Initial connections process one request
  at a time; multiplexing is a separate measured extension.
- Atomically persist content changes and replayable events, then publish after
  commit. Delivery is resumable and may duplicate events; clients deduplicate.
  Event history is application data, not a replacement recovery WAL.
- Chunk message bodies to respect current encoded-row limits. Batch streamed output;
  label uncommitted previews explicitly. Provider calls run outside transactions.
- Context assembly selects instructions, approved facts, recent complete turns,
  versioned summaries, and authorized evidence within a measured token budget.
  Reserve output/tool capacity and retain exact input provenance.
- Keep summaries and embeddings rebuildable and permission-scoped. Storage deletion,
  derived-data invalidation, event retention, and backup expiry are explicit policies.
- Defer LSM, WAL, approximate vector indexes, distributed writes, and automatic
  character-level collaboration until requirements and benchmarks justify them.

## Issue Management

The numbers below are local roadmap identifiers, not GitHub issue numbers.
Search existing issues before creating new ones. Update or reopen matching issues
for recovery, snapshots, benchmarks, and server work rather than duplicate them.
Use existing labels where available. Proposed additional labels are:
`bug` (incorrect behavior), `server` (network service and operations), and
`security` (identity, authorization, isolation, and secret handling).
Other labels reuse feature, api, design, documentation, storage, indexing,
transactions, concurrency, sql, test, and performance. Use `refactor` for
behavior-preserving package and ownership changes.

Create only near-term issues as Ready; keep later work in Backlog. Each issue body
below contains Goal, Why, Tasks, and Acceptance Criteria for copying into GitHub.

## Suggested Milestones and Dependencies

- Architecture: A1 first; A2-A3 after correctness and baseline; A4 after issue 6.
- Correctness and baseline: issues 1-5, with 5 captured before major redesign.
- Shared conversation storage: issues 6-10; 7-9 depend on 6 and correctness.
- Secure live sharing: issues 11-13; 12 depends on 9, and 13 on 10-12.
- Generation and context budgets: issues 14-16; 14 depends on 8-13.
- Retrieval and evaluation: issues 17-19; 18 follows keyword evaluation.
- Retention and deployment: issues 20-21; retention design starts with 6 and must
  be enforced before deployment. Deployment requires the prior correctness,
  authorization, delivery, and recovery gates.
- Evidence-driven optimization: issue 22; create narrower implementation issues
  from measured findings instead of treating it as one large rewrite.

Record benchmarks before and after storage, snapshots, indexes, batching,
context selection, or transport changes. Open draft PRs for substantial changes.
Request focused correctness/security reviews for transaction, reclamation,
authorization, and recovery work before merging.

## A1. Document Target Architecture and Migration Plan

Labels: `design`, `documentation`

### Goal

Define the TCP/TLS server architecture and package ownership before refactoring.

### Why

Explicit dependency and durability boundaries keep later changes reviewable.

### Tasks

- [ ] Document current versus target packages and dependency direction
- [ ] Define transaction, snapshot, storage, and event ownership
- [ ] Specify TCP/TLS as the first transport and a later browser gateway
- [ ] Document staged migration, compatibility, and deferred features

### Acceptance Criteria

- [ ] Dependencies have no import cycles and responsibilities are explicit
- [ ] Implemented guarantees are distinguished from planned capabilities
- [ ] Public API and file-format compatibility requirements are documented

## A2. Separate Page Storage from B+Tree Operations

Labels: `refactor`, `storage`, `test`

### Goal

Extract file I/O, metadata, allocation, and free-list management into storage.

### Why

Separate ownership makes recovery and tree behavior independently testable.

### Tasks

- [ ] Define narrow page-access and retirement contracts with error propagation
- [ ] Keep tree encoding and structural operations in btree
- [ ] Keep reachability validation coordinated without cyclic imports
- [ ] Move existing tests with their owners and preserve failure hooks

### Acceptance Criteria

- [ ] Public API, file bytes, and durability ordering remain compatible
- [ ] Fake-store tree tests and temporary-file recovery tests pass
- [ ] Race tests and vet pass without new import cycles

## A3. Clarify Transaction and Snapshot Ownership Boundaries

Labels: `refactor`, `transactions`, `concurrency`

### Goal

Make commit coordination and page lifetime responsibilities explicit.

### Why

Snapshot readers require a reliable contract between root publication and reuse.

### Tasks

- [ ] Isolate transaction coordination behind the existing db API
- [ ] Document writer admission, root publication, and failure restoration
- [ ] Define snapshot acquisition/release, Close, and retirement contracts
- [ ] Preserve current synchronization until snapshot implementation is tested

### Acceptance Criteria

- [ ] Ownership and lock ordering are documented and testable
- [ ] Refactoring does not claim or introduce untested snapshot isolation
- [ ] Existing transaction, recovery, and race tests pass

## A4. Define Conversation Service and Transport Contracts

Labels: `design`, `api`, `server`

### Goal

Define authorized conversation operations shared by TCP and future browser access.

### Why

Shared application contracts prevent transport-specific permission and retry bugs.

### Tasks

- [ ] Specify append, history, branch, generation, and subscription operations
- [ ] Define authenticated identity, authorization, idempotency, and errors
- [ ] Define bounded TCP frames, protocol versioning, and lifecycle behavior
- [ ] Define committed-event replay and a later browser gateway boundary

### Acceptance Criteria

- [ ] Request/response examples cover success, denial, retries, and reconnects
- [ ] Browser users cannot invoke unrestricted SQL or raw storage operations
- [ ] Implementations can share authorization and transaction behavior

## 1. Fix SQL Validation and Query Semantics

Labels: `bug`, `sql`, `test`

### Goal

Reject malformed queries consistently and return correct results for supported predicates.

### Why

Query failures must be predictable before SQL is reachable through a service.

### Tasks

- [ ] Validate AST shapes, predicate columns, literal types, and operators before fetching rows
- [ ] remove fabricated maximum string keys
- [ ] support primary-key inequality and correct bound intersection
- [ ] make unsupported comparisons explicit errors

### Acceptance Criteria

- [ ] Invalid queries fail on both empty and populated tables without panics
- [ ] point, range, and scan results agree with a reference filter
- [ ] tests cover unknown columns, NULL policy, contradictory bounds, bytes, and malformed ASTs

## 2. Repair Row and Secondary Index Encoding

Labels: `bug`, `storage`, `indexing`

### Goal

Prevent ambiguous keys, incorrect equality matches, and lossy NULL round trips.

### Why

Ambiguous or lossy storage encoding can return wrong rows and corrupt shared context.

### Tasks

- [ ] Use unambiguous order-preserving component encoding
- [ ] align primary-key encoding and decoding
- [ ] distinguish NULL from empty and zero values
- [ ] version persisted formats and specify migration or explicit incompatibility rejection

### Acceptance Criteria

- [ ] String and byte index values cannot collide with primary-key suffixes
- [ ] equality does not match longer prefixes
- [ ] NULL and non-NULL values round-trip
- [ ] old-format files are migrated deliberately or rejected clearly

## 3. Unify Writer Admission and Atomic Table Mutations

Labels: `bug`, `transactions`, `indexing`

### Goal

Ensure every write path preserves transaction isolation and row/index consistency.

### Why

Multiple users and retries must not expose partial records or silently bypass constraints.

### Tasks

- [ ] Route KV, table, SQL, and schema mutations through common writer admission
- [ ] make direct table mutations atomic
- [ ] refresh catalog definitions across handles
- [ ] validate unique-index ownership
- [ ] publish index metadata only with completed backfill

### Acceptance Criteria

- [ ] Concurrent direct and transactional writes cannot bypass writer admission
- [ ] interrupted row/index mutations expose no partial state
- [ ] old handles maintain new indexes
- [ ] failed unique updates and backfills leave the previous state intact

## 4. Harden File Ownership and Corruption Recovery

Labels: `bug`, `storage`, `test`

### Goal

Prevent simultaneous file owners and distinguish an empty database from damaged committed storage.

### Why

A service must reject unsafe ownership and damaged storage rather than misrepresent data loss.

### Tasks

- [ ] Acquire exclusive database-file ownership
- [ ] reject corrupt existing metadata instead of treating it as a fresh file
- [ ] validate referenced pages and free lists
- [ ] define fallback reporting
- [ ] add missing fault-injection coverage after auditing existing tests

### Acceptance Criteria

- [ ] Second owners fail clearly
- [ ] unsupported formats and unrecoverable corruption fail closed
- [ ] recovery selects a complete valid state
- [ ] tests distinguish interrupted writes from later corruption and do not promise recovery of destroyed acknowledged data

## 5. Capture Shared-Context Workload Baselines

Labels: `performance`, `test`

### Goal

Establish repeatable evidence before concurrency and context-storage changes.

### Why

Performance claims require reproducible evidence under equivalent durability settings.

### Tasks

- [ ] Benchmark small and large messages, history pagination, index lookup, startup, and durable writes
- [ ] record page-map cloning, allocations, memory, file growth, and sync costs
- [ ] preserve hardware, revision, dataset, and command details

### Acceptance Criteria

- [ ] Repeated results are stored under docs/benchmarks
- [ ] results include ns/op, B/op, allocs/op and workload throughput
- [ ] profiles stay in ignored profiles/
- [ ] before/after comparisons use identical workloads and benchstat

## 6. Define Shared Conversation and Revision Semantics

Labels: `design`, `documentation`

### Goal

Define a useful v1 for shared conversations without ambiguous automatic text merging.

### Why

A narrow user workflow makes storage and collaboration requirements testable.

### Tasks

- [ ] Specify conversations, branches, messages, immutable revisions, membership roles, and generation attempts
- [ ] assign conversation-local ordering
- [ ] define explicit branch selection and edit behavior
- [ ] distinguish application revisions from storage snapshots
- [ ] interview prospective users about handoff and repeated-context problems

### Acceptance Criteria

- [ ] Examples show append, edit-as-revision, branch, and resume
- [ ] message/tool relationships and provenance are explicit
- [ ] v1 excludes automatic character-level merging
- [ ] a concrete user workflow and success measure are documented

## 7. Implement Ordered Conversation Storage and Pagination

Labels: `feature`, `api`, `indexing`

### Goal

Retrieve complete authorized conversation history in deterministic order with bounded work.

### Why

Conversation replay requires ordering and limits that full scans cannot provide economically.

### Tasks

- [ ] Implement conversation/branch/message records
- [ ] encode tenant, conversation, branch, and sequence in unambiguous ordered keys or indexes
- [ ] add seek-based pagination and cursor scope validation
- [ ] preserve tool-call/result and revision relationships

### Acceptance Criteria

- [ ] Pagination returns each selected message once under documented consistency semantics
- [ ] scans do not cross conversation boundaries
- [ ] restart preserves ordering
- [ ] limits bound returned rows and traversal work
- [ ] timestamps are not the sole ordering source

## 8. Store Large Message Bodies in Chunks

Labels: `feature`, `storage`, `test`

### Goal

Support prompts and responses larger than the current 3000-byte encoded-row limit.

### Why

LLM prompts and messages routinely exceed the current single-row storage budget.

### Tasks

- [ ] Store immutable byte chunks with ordered references and a body manifest
- [ ] account for encoding overhead
- [ ] enforce message and chunk limits
- [ ] define atomic publication and staged-upload cleanup
- [ ] avoid copying full histories when branching

### Acceptance Criteria

- [ ] Large Unicode bodies reassemble exactly
- [ ] incomplete uploads never appear complete
- [ ] restart and retry are safe
- [ ] unreferenced chunks can be reclaimed without deleting shared revision content
- [ ] size-boundary tests pass

## 9. Add Idempotent Conversation Mutations

Labels: `feature`, `transactions`, `api`

### Goal

Make retries after lost responses safe and keep message updates internally consistent.

### Why

Network uncertainty makes duplicate requests normal; durable deduplication avoids repeated effects.

### Tasks

- [ ] Atomically commit message metadata, chunk references, indexes, sequence allocation, and scoped idempotency records
- [ ] hash request payloads
- [ ] reject key reuse with different content
- [ ] define retention and behavior after expiry
- [ ] serialize mutations before reading their base state

### Acceptance Criteria

- [ ] Retries within retention return the original result without duplicate messages
- [ ] crash tests expose all related records together
- [ ] simultaneous requests receive unique ordered sequences
- [ ] all service mutations use the same writer boundary

## 10. Implement Pinned Snapshots and Reader-Safe Reclamation

Labels: `feature`, `concurrency`, `storage`

### Goal

Allow stable conversation reads while a serialized writer publishes new versions.

### Why

Prompt assembly must not mix revisions while concurrent users change a conversation.

### Tasks

- [ ] Atomically capture and register immutable roots
- [ ] add transaction-aware table reads
- [ ] protect active readers and recovery roots from page reuse
- [ ] define Close, cancellation, snapshot limits, and retention behavior
- [ ] measure retained-page growth

### Acceptance Criteria

- [ ] A reader sees one committed version across multiple reads
- [ ] writers can commit while readers traverse
- [ ] active and fallback pages are not reused
- [ ] releasing snapshots permits reclamation
- [ ] deterministic race and restart tests pass

## 11. Implement User Identity and Conversation Authorization

Labels: `feature`, `security`, `api`

### Goal

Make shared conversations accessible only to authorized users.

### Why

Authenticating a service connection alone does not authorize access to a user's messages.

### Tasks

- [ ] Integrate a maintained authentication implementation/provider
- [ ] define owner/editor/viewer membership
- [ ] authorize every read, write, generation, replay, and subscription
- [ ] enforce revocation
- [ ] keep service credentials separate from user identity
- [ ] redact credentials and message bodies from routine logs

### Acceptance Criteria

- [ ] Cross-user and cross-tenant tests cover guessed IDs and cursors
- [ ] viewers cannot write
- [ ] revoked access stops subsequent event delivery under documented semantics
- [ ] unauthenticated calls fail
- [ ] raw unrestricted SQL is not exposed to browser users

## 12. Persist Conversation Events with Mutations

Labels: `feature`, `transactions`, `server`

### Goal

Prevent lost notifications between a successful database commit and event publication.

### Why

An in-memory broadcast can be lost after a successful commit and before network delivery.

### Tasks

- [ ] Write ordered event records in the same transaction as conversation changes and idempotency results
- [ ] reference content instead of duplicating large bodies
- [ ] dispatch only committed events
- [ ] define retention and replay cursors

### Acceptance Criteria

- [ ] Crashing after commit but before delivery does not lose the replayable event
- [ ] rolled-back mutations emit none
- [ ] duplicate delivery is harmless to consumers
- [ ] event and message visibility agree after restart
- [ ] this event history is not a storage WAL

## 13. Add Resumable Real-Time Conversation Delivery

Labels: `feature`, `server`, `test`

### Goal

Allow connected users to follow changes and recover missed updates.

### Why

Real-time sharing requires recovery from disconnection and protection from slow consumers.

### Tasks

- [ ] Expose authenticated commands and subscriptions through the TCP/TLS service, then add a browser-compatible SSE or WebSocket gateway
- [ ] establish a consistent initial-state/event-cursor boundary
- [ ] implement replay, deduplication, retention-expiry resync, bounded queues, deadlines, and connection limits
- [ ] share application handlers with the later browser gateway

### Acceptance Criteria

- [ ] Disconnect/reconnect converges without gaps within retention
- [ ] initial load cannot race event subscription
- [ ] slow consumers cannot block commits or grow memory indefinitely
- [ ] authorization applies to both live events and replay
- [ ] TLS is required for deployed access

## 14. Implement Recoverable Streaming Generation Jobs

Labels: `feature`, `api`, `transactions`

### Goal

Track streamed LLM output without holding database transactions open during network calls.

### Why

Long external calls and uncertain provider outcomes should not hold storage locks or corrupt retries.

### Tasks

- [ ] Persist input revision manifests, model/configuration, attempt IDs, and job states
- [ ] run provider calls outside transactions
- [ ] batch output by configurable bytes/time
- [ ] distinguish previews from durable chunks
- [ ] handle cancellation, retries, and stale callbacks
- [ ] define provider-uncertain outcomes

### Acceptance Criteria

- [ ] Restart preserves committed chunks and clearly marks interrupted attempts
- [ ] superseded callbacks cannot change current output
- [ ] duplicate requests do not create duplicate logical jobs
- [ ] documentation does not promise exactly-once provider execution or billing
- [ ] no long-lived storage snapshot is held for an entire generation

## 15. Implement Token-Budgeted Context Assembly

Labels: `feature`, `api`, `test`

### Goal

Bound model input costs while preserving relevant conversation structure.

### Why

Retaining history does not require resending the entire transcript on every model request.

### Tasks

- [ ] Use a model-appropriate tokenizer
- [ ] budget instructions, pinned facts, recent complete turns, summaries, retrieved evidence, tools, and output reserve
- [ ] define deterministic selection
- [ ] record exact selected revisions and token estimates
- [ ] treat retrieved content as data rather than trusted instructions

### Acceptance Criteria

- [ ] Constructed requests fit configured model/application limits
- [ ] required content that cannot fit produces a clear error
- [ ] tool relationships are preserved
- [ ] equivalent input produces reproducible selection
- [ ] permission and branch boundaries hold

## 16. Add Versioned Summaries and User-Pinned Facts

Labels: `feature`, `api`, `test`

### Goal

Reduce repeated history tokens while retaining provenance and correction paths.

### Why

Summaries save input budget but can omit facts or preserve superseded instructions.

### Tasks

- [ ] Create summaries asynchronously with source revision ranges and model/prompt versions
- [ ] let users approve pinned facts
- [ ] invalidate derived content after relevant edits, deletion, or access changes
- [ ] retain source references and allow regeneration

### Acceptance Criteria

- [ ] Summaries never replace canonical history
- [ ] stale summaries are not silently selected
- [ ] pinned facts are scoped and editable
- [ ] summary-generation cost is recorded
- [ ] evaluations include factual omissions, stale instructions, and lost references

## 17. Add Authorized Keyword Retrieval

Labels: `feature`, `indexing`, `test`

### Goal

Find exact identifiers and useful historical passages without embedding every message.

### Why

Exact search provides a measurable retrieval baseline and handles names and identifiers well.

### Tasks

- [ ] Index stable message revisions or semantic chunks
- [ ] filter tenant/conversation/branch permissions
- [ ] return source IDs and neighboring context
- [ ] define index freshness, deletion, and rebuild behavior
- [ ] bound query work and result size

### Acceptance Criteria

- [ ] Exact names, quoted phrases, and error identifiers are retrievable
- [ ] unauthorized and invalidated revisions never reach context assembly
- [ ] deleted sources leave the index
- [ ] results have usable provenance and are evaluated against a labeled query set

## 18. Evaluate Semantic and Hybrid Retrieval

Labels: `performance`, `indexing`, `test`

### Goal

Determine whether embeddings improve retrieval quality enough to justify their cost.

### Why

Semantic relevance is useful only if the quality gain outweighs added indexing and inference costs.

### Tasks

- [ ] Build a representative relevance dataset
- [ ] compare keyword, exact vector similarity, and hybrid retrieval
- [ ] embed stable chunks rather than token deltas
- [ ] record embedding model/version and source revision
- [ ] measure memory, embedding cost, query latency, and quality

### Acceptance Criteria

- [ ] Results include recall at k and downstream task success where appropriate
- [ ] permission and deletion tests pass
- [ ] embeddings are rebuildable derived data
- [ ] an approximate vector index is proposed only with measured scale and quality needs

## 19. Evaluate Context Quality and Cost per Task

Labels: `performance`, `test`, `documentation`

### Goal

Measure savings without hiding a loss of answer quality.

### Why

Lower token counts are not useful if the system becomes less accurate or needs more retries.

### Tasks

- [ ] Compare bounded recent history, summary-assisted context, and retrieval-assisted context on fixed tasks
- [ ] record input/output tokens, summary and embedding usage, latency, retries, and model/version
- [ ] state price date and caching assumptions
- [ ] collect handoff workflow feedback

### Acceptance Criteria

- [ ] Report quality and cost together
- [ ] publish reproducible workloads with no private user data
- [ ] record actual provider usage separately from estimates
- [ ] report cost per successful task and uncertainty
- [ ] preserve before/after baselines without claiming unmeasured improvements

## 20. Define Retention and Delete Derived Context Data

Labels: `feature`, `security`, `storage`

### Goal

Bound growth and honor deletion across messages and their derived data.

### Why

Context storage includes multiple copies and derived representations that can grow without bounds.

### Tasks

- [ ] Specify retention for bodies, branches, events, idempotency records, summaries, embeddings, snapshots, and backups
- [ ] reclaim only unreferenced content
- [ ] invalidate context caches and active access
- [ ] define replay-expiry behavior and backup expiry

### Acceptance Criteria

- [ ] Deleted or unauthorized content cannot be newly retrieved
- [ ] shared chunks remain while referenced
- [ ] logical deletion is distinguished from physical erasure
- [ ] retention tests cover restart, expired cursors, and space reuse
- [ ] indefinite event and snapshot growth is prevented

## 21. Deploy and Validate the Shared-Context Service on AWS

Labels: `feature`, `server`, `documentation`

### Goal

Demonstrate reproducible operation and recovery of a single-node service.

### Why

A deployed demonstration should prove lifecycle and recovery behavior as well as startup.

### Tasks

- [ ] Provision one EC2 process owner and persistent encrypted EBS with infrastructure as code
- [ ] configure TLS, secrets, restricted access, monitoring, budgets, graceful shutdown, and teardown
- [ ] add consistent backup/restore procedures and browser demo
- [ ] document protocol choice and limits

### Acceptance Criteria

- [ ] A fresh environment can be reproduced
- [ ] acknowledged writes survive tested process restarts
- [ ] backup restoration is demonstrated
- [ ] multi-user authorization and replay work end to end
- [ ] deployment costs are recorded
- [ ] no high-availability or replication guarantee is claimed

## 22. Optimize Measured Storage and Delivery Bottlenecks

Labels: `performance`, `storage`, `concurrency`

### Goal

Improve capacity or latency based on representative workload evidence.

### Why

Adding storage architectures without evidence risks complexity without user benefit.

### Tasks

- [ ] Profile page-map cloning, resident pages, startup validation, scans, sync batching, and subscriber queues
- [ ] select one measured bottleneck per implementation issue
- [ ] evaluate demand-loaded pages or bounded caching if memory growth warrants it
- [ ] compare equivalent durability and workload settings

### Acceptance Criteria

- [ ] Before/after results include latency percentiles, throughput, allocations, memory, retained pages, and file growth as applicable
- [ ] fault and race tests still pass
- [ ] CPU improvements are not mislabeled as network or inference savings
- [ ] LSM, WAL, and approximate vector search remain separate justified decisions

## References

- [Context engineering and compaction](https://www.anthropic.com/engineering/effective-context-engineering-for-ai-agents)
- [Transactional outbox and duplicate delivery](https://docs.aws.amazon.com/en_en/prescriptive-guidance/latest/cloud-design-patterns/transactional-outbox.html)
- [Hybrid text and semantic retrieval](https://aws.amazon.com/blogs/machine-learning/amazon-bedrock-knowledge-bases-now-supports-hybrid-search/)

These references inform design choices; they are not evidence that this repository
already implements or has measured those capabilities.
