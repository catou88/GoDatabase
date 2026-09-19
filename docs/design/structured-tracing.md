# Structured Operation Tracing

The educational engine can receive an optional `trace.Sink` when an
experiment is created. The disabled sink is the default, so tracing does not
change operation results or require an event allocation in normal runs.

Events are emitted in operation order:

1. `operation_selected` identifies the chosen structure and operation.
2. `lookup` identifies a key lookup and, when available, the node visited.
3. `comparison` records a key comparison or insertion decision.
4. `traversal` records movement through a child or page, including a page ID
   when the durable implementation has one.
5. `result_delivered` records the final result or an error detail.

Each event has a monotonic sequence number assigned by the recorder. This
makes deterministic workloads reproducible without treating wall-clock time as
part of the trace contract. Event fields are JSON serializable and omit
optional identifiers when an implementation cannot provide them.

Tracing is observational. It must not mutate the selected structure, alter
error behavior, or claim that a page was visited when the implementation did
not visit it. Measured duration and allocation data belong in metrics, not in
the theoretical trace event itself.

Example:

```json
{
  "sequence": 2,
  "type": "lookup",
  "operation": "get",
  "structure": "btree-memory",
  "key": "alpha",
  "node_id": 3
}
```
