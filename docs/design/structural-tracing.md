# Execution traces

Events are emitted at execution points and assigned increasing sequence numbers.
operation_index refers to the request operation, starting at zero. Layer is
request, cache, backing, or result. Node/page IDs are session-local identifiers,
not memory addresses. keys and children are bounded visual snapshots, not a
complete database export. A trace is a visited path, not a full-tree snapshot.

The recorder captures at most 200 events with at most 16 keys and 17 child IDs
per event. trace_truncated signals omitted events. Logical execution continues
when the trace fills. Tracing is disabled during initial dataset setup.

Go map bucket layout is not observable through the public map interface; its
events report logical lookup and scanning, without inventing bucket traversal.
Sorted-array comparisons are emitted from binary search. Cache hits skip backing
lookup events; misses and invalidations appear at the corresponding calls.

Playback uses sequence order, not measured timestamps. Tracing adds overhead
inside the measured loop. Use tracing disabled for performance baselines.
