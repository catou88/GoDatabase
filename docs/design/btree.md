# B+Tree Storage Model

## Goal

Design the initial B+Tree structure for GoDatabase before implementing node search, insertion, deletion, and range scans.

The B+Tree will replace the current map-backed storage while preserving the public key-value behavior already covered by tests.

## Non-Goals

- Disk page serialization for the first in-memory implementation
- Crash recovery
- Transactions
- SQL parsing or query planning
- Concurrent access control

Those concerns should be designed separately after the in-memory B+Tree behavior is correct.

## Key And Value Model

The current public API uses string keys and string values:

```go
Set(key string, value string) error
Get(key string) (string, bool)
Range(start string, end string) []Item
```

The first B+Tree implementation should store keys and values as strings to match the existing API. A later disk-backed storage layer may encode keys and values as `[]byte` when page serialization is introduced.

Keys are unique. Inserting an existing key overwrites its value.

## Node Types

The tree has two node types:

- Leaf nodes store user keys and values.
- Internal nodes store separator keys and child references.

All user values live in leaf nodes. Internal nodes guide search and do not store user values.

## Proposed In-Memory Node Layout

```go
type node struct {
	leaf     bool
	keys     []string
	values   []string
	children []*node
	next     *node
}
```

Field meanings:

- `leaf` marks whether the node is a leaf or internal node.
- `keys` is sorted ascending in every node.
- `values` is used only by leaf nodes.
- `children` is used only by internal nodes.
- `next` links leaf nodes for ordered range scans.

For a leaf node:

```text
len(keys) == len(values)
len(children) == 0
```

For an internal node:

```text
len(values) == 0
len(children) == len(keys) + 1
```

## Disk-Backed Components

The repository now includes the building blocks for a disk-backed B+Tree:

- 4 KiB serialized pages
- leaf and internal page formats
- page-number child references
- encoded headers and key/value offsets
- little-endian integer encoding
- maximum key/value size limits
- page allocation and fixed-position reads and writes
- alternating root metadata slots
- metadata generations and checksums
- durable root publication with two `fsync` phases
- persistent free-page tracking and delayed page reuse

These components are not yet connected to the in-memory B+Tree or the public
`db.Database` API. The current B+Tree still uses direct `*node` pointers and
mutates nodes in memory.

## Book Alignment

The persistent implementation should follow Chapters 4 through 7 of *Build
Your Own Database From Scratch in Go* whenever the book's design fits the
existing project. In particular:

- Represent persistent nodes as `BNode []byte` and operate on the encoded page
  through accessors such as `btype`, `nkeys`, `getPtr`, `getOffset`, `getKey`,
  and `getVal`.
- Use the book's internal-node convention in which each key is the lower bound
  for its corresponding child and the number of keys equals the number of
  child pointers.
- Reserve an internal empty-key sentinel as the lowest key so lookups always
  find a containing child. The sentinel is an implementation detail; empty
  user keys remain invalid in the public API.
- Isolate the B+Tree from storage through `get`, `new`, and `del` page
  callbacks so the same tree algorithms can use fake in-memory pages in tests
  and real pages in the durable KV store.
- Build updates by copying encoded entries into replacement nodes. Committed
  B+Tree pages must never be modified in place.
- Split and merge nodes according to encoded byte size rather than a fixed key
  count.
- Use the book's two-phase update order: write pages, sync pages, publish root
  metadata, and sync metadata.
- Replace the bounded metadata-inline free list with the book's page-backed,
  self-managing FIFO free list.

Two deliberate implementation choices remain compatible with the book:

- Keep the two checksummed metadata slots. Chapter 6 describes this
  double-buffering scheme as the stronger alternative when a single metadata
  write cannot be assumed power-loss atomic.
- Continue using `ReadAt` and `WriteAt` initially instead of `mmap`. Chapter 6
  treats `mmap` as a convenience rather than a requirement. The storage API
  should not prevent adding `mmap` later.

The existing page format uses the conventional `len(children) == len(keys)+1`
layout. It should be migrated to the book's key-pointer-pair layout before the
on-disk format is declared stable. No backward-compatibility promise should be
made for development database files before that point.

## Search Behavior

The current in-memory tree uses the conventional separator layout. Search
starts at the root and, for each internal node:

1. Find the first separator greater than the search key.
2. Follow the child pointer at that position.
3. Repeat until reaching a leaf.

The persistent tree should instead follow the book's key-pointer-pair layout:

1. Use `nodeLookupLE` to find the last key less than or equal to the search key.
2. Follow the child pointer stored at that same index.
3. Rely on the internal sentinel key to cover values below the first user key.
4. Repeat until reaching a leaf.

For each leaf node:

1. Binary search the sorted `keys`.
2. If the key exists, return the value.
3. If the key does not exist, return the missing-key result.

This preserves the public `Get` behavior:

```go
value, ok := db.Get(key)
```

## In-Memory Tree Structure

The first B+Tree implementation should store a direct pointer to the root node.

```go
type btree struct {
	root     *node
	maxKeys  int
	minKeys  int
}
```

Field meanings:

- `root` points to the current root node.
- `maxKeys` is the maximum number of keys a node may hold before splitting.
- `minKeys` is the minimum number of keys a non-root node should hold after
  rebalancing.

Keeping the initial tree pointer-based makes search, insertion, splitting,
deletion, and invariant tests easier to implement before adding disk I/O.

## Insert Behavior

Insert follows the search path to the target leaf.

At the leaf:

1. If the key already exists, overwrite its value.
2. Otherwise insert the key and value in sorted order.
3. If the node exceeds capacity, split it.

When a node splits:

1. Create a right sibling.
2. Move roughly half the keys into the right sibling.
3. Promote a separator key to the parent.
4. If the parent overflows, split the parent.
5. If the root splits, create a new root.

For an in-memory leaf node, insertion should:

1. Copy entries before the insertion index.
2. Append the new key/value pair.
3. Copy entries from the insertion index onward.
4. Split the node if it exceeds `maxKeys`.

Updating an existing key should:

1. Find the matching key index.
2. Replace the value at the same index.
3. Leave the tree shape unchanged.

The insertion index should be found by searching sorted keys. This can be a
linear search at first and upgraded to binary search later if benchmarks show it
matters.

Insertion should recurse down the tree:

1. Start at the root node.
2. Find the child range that should contain the key.
3. Recurse until reaching a leaf.
4. Insert or update the leaf.
5. Split an overfull child and insert the promoted separator into its parent.
6. Propagate parent growth upward.

If a split reaches the root, the tree creates a new root and the tree height
grows by one.

When replacing a split child in an internal node, one child pointer becomes two
child pointers and the promoted separator key is inserted between them.

## Split Behavior

### Leaf Split

When a leaf node overflows:

1. Split `keys` and `values` around the midpoint.
2. Keep the lower half in the left leaf.
3. Move the upper half into the new right leaf.
4. Set the new right leaf's `next` pointer to the old left leaf's `next`.
5. Set the left leaf's `next` pointer to the new right leaf.
6. Promote the first key of the right leaf to the parent.

Promoting the first key of the right leaf keeps internal separators aligned with leaf contents.

### Internal Split

When an internal node overflows:

1. Split around the midpoint key.
2. Promote the midpoint key to the parent.
3. Keep keys before the midpoint in the left node.
4. Move keys after the midpoint into the right node.
5. Split child pointers so each internal node keeps `len(keys) + 1` children.

The promoted midpoint key should not remain in either child internal node.

## Range Scan Behavior

Range scans use leaf ordering.

To evaluate:

```go
Range(start, end)
```

the tree should:

1. Return an empty slice immediately when `start > end`.
2. Find the first leaf that could contain `start`.
3. Walk keys in ascending order.
4. Append keys where `start <= key <= end`.
5. Follow `next` leaf links until keys exceed `end` or the leaf chain ends.

Range bounds are inclusive. Empty string bounds are compared as ordinary strings and are not unbounded range markers.

## Delete Behavior

Deletion can be implemented after search and insertion are stable.

The initial delete design should:

1. Find the target leaf.
2. Remove the key and value if present.
3. Return whether a key was removed.
4. Rebalance underfull nodes with redistribution or merge.
5. Shrink the root when it has only one child.

Delete should preserve all B+Tree invariants after each operation.

## Invariants

The implementation should maintain these invariants:

- Keys in every node are sorted ascending.
- Leaf node `keys` and `values` have the same length.
- Internal nodes have no user values.
- Internal node child count is `len(keys) + 1`.
- All leaves are at the same depth.
- Leaf `next` pointers preserve ascending key order.
- Duplicate keys are not stored.
- Inserting an existing key overwrites the existing value.
- Range scans return keys in ascending order.
- Non-root nodes respect minimum and maximum occupancy after rebalancing.
- The root may have fewer keys than non-root nodes.

## Testing Strategy

The B+Tree should be tested at two levels.

Public behavior tests:

- `Set`
- `Get`
- `Delete`
- `Range`
- overwrite behavior
- missing-key behavior

Internal invariant tests:

- node keys remain sorted
- internal child counts are valid
- leaf value counts match leaf key counts
- all leaves have the same depth
- leaf links are ordered
- inserts and deletes preserve invariants after each operation

Fuzz tests should compare B+Tree behavior against a simple `map[string]string` reference model.

## Remaining Work

### Integrate the B+Tree with disk pages

- Add the book-style `BTree` with a root page ID and `get`, `new`, and `del`
  callbacks.
- Add byte-page accessors and setters for headers, pointers, offsets, keys, and
  values.
- Add `nodeAppendKV`, `nodeAppendRange`, `leafInsert`, and `leafUpdate` helpers.
- Implement `nodeLookupLE` over encoded keys.
- Implement recursive copy-on-write `treeInsert`.
- Replace a modified child with one to three newly written children.
- Propagate splits through replacement parents and create a new root when the
  old root splits.
- Call `del` for every page replaced by the new tree version.
- Test the tree first with the book's fake in-memory page callbacks, then bind
  the same callbacks to the page manager.

### Use encoded byte size for node balancing

- Determine overflow from the encoded node size rather than only key count.
- Add `nbytes`, `nodeSplit2`, and `nodeSplit3` equivalents.
- Split nodes so every resulting serialized node fits in one 4 KiB page.
- Produce one, two, or three pages when uneven key/value sizes require it.
- Trigger deletion merges when an updated node uses no more than one quarter
  of a page and the combined siblings fit in one page.
- Test nodes containing keys and values near their maximum permitted sizes.

### Add disk-backed deletion

- Add book-style `leafDelete`, `nodeMerge`, `nodeReplace2Kid`, `shouldMerge`,
  `treeDelete`, and `nodeDelete` operations over encoded pages.
- Remove keys by creating replacement leaf pages.
- Merge with a left or right sibling when `shouldMerge` permits it.
- Propagate empty nodes and merged children through replacement parents.
- Replace or remove an empty root as described by the high-level tree API.
- Send replaced child, sibling, and root pages through the `del` callback.
- Make those pages reusable only after the replacement root commits safely.
- Add missing behavioral coverage for left-sibling redistribution.

### Expose a durable KV API

- Add a book-style `KV` type containing the path, file, B+Tree, page state, and
  free list.
- Add `Open(path)` and `Close()` behavior.
- Connect public `Get`, `Set`, `Delete`, and `Range` operations to the
  page-backed B+Tree.
- Maintain pending appended and reused pages until commit.
- Save the pre-update metadata state and restore it after write or sync errors.
- Track a failed update and repair the last known metadata state before a later
  write attempt.
- Reopen the database from the latest valid root metadata.
- Preserve the existing empty-key and range-bound semantics.
- Decide whether the public API remains string-based or changes to `[]byte`.
- Keep the map implementation available only as a test reference or remove it
  after migration.

### Add ordered disk iteration

- Seek to the first leaf entry at or after a starting key.
- Walk subsequent leaf entries in key order.
- Continue across leaf pages without sorting the complete database in memory.
- Support inclusive public range bounds.
- Consider an iterator API so large ranges do not require one result slice.

### Add versioned trees with concurrent readers and writers

The current durable `KV` serializes reads and writes with one mutex. This is a
correctness-first implementation: a read cannot observe a root, page map, or
close operation changing while it traverses the tree. A later concurrency
milestone should replace this coarse locking with explicitly versioned trees.

The intended model is:

1. Every committed version has its own generation and root page ID.
2. A reader pins a version and traverses the immutable pages reachable from
   that root without holding a tree-wide lock.
3. A writer forks a selected version, creates replacement pages with
   copy-on-write, and owns a private root while making changes.
4. Multiple readers and writers may use different roots concurrently. A writer
   never modifies pages belonging to its base version or another writer.
5. Finishing a writer creates a new, distinct committed version. It does not
   merge changes into another writer's version and does not retry against a
   newer root.
6. Selecting which committed version is the canonical current root is a
   separate, serialized metadata operation. Publishing a version created from
   an older branch must be explicit so it cannot silently discard another
   version's changes.
7. Page allocation and metadata publication may use short critical sections,
   but tree traversal and private-version editing should not require a global
   tree lock.
8. Pages are reclaimed only after no retained version, active reader, active
   writer, or fallback metadata slot can reference them.

This is a persistent multiversion tree model, not optimistic conflict
detection. Versions remain separate unless a future explicit merge operation
defines how their changes are combined. Keeping versions independent avoids
ambiguous last-writer-wins behavior, but requires version lifecycle and storage
retention policies.

Follow-up work:

- Define version handles and their retain/release lifetimes.
- Track committed roots, active readers, and private writer roots.
- Allow `Get` and range scans to select an immutable version.
- Allow writers to fork and modify selected versions concurrently.
- Define explicit canonical-root publication without automatic retry or merge.
- Synchronize page allocation, metadata publication, recovery, and `Close`.
- Delay free-list reuse until no protected version can reach a page.
- Define retention, pruning, and explicit version-deletion policies.
- Add race, version-isolation, reclamation, and reader/writer stress tests.
- Benchmark `sync.Mutex`, `sync.RWMutex`, and snapshot-based reads before
  selecting the final implementation.

### Complete recovery validation

- Decode the selected root page when opening the database.
- Traverse every page reachable from the selected root.
- Validate child page IDs, key ordering, parent-child bounds, node occupancy,
  and equal leaf depth.
- Reject corrupt committed trees instead of silently opening an empty database.
- Detect unreachable pages left by interrupted operations.
- Add fault-injection tests for page writes and both synchronization phases.
- Sync the parent directory when creating the database file.

### Integrate and scale free-page management

- Replace the metadata-inline page ID list with the book's unrolled linked
  list of fixed-size free-list pages.
- Store multiple free page IDs plus a next-page pointer in each free-list node.
- Track `headPage`, `headSeq`, `tailPage`, `tailSeq`, and `maxSeq`.
- Implement `PopHead`, `PushTail`, and `SetMaxSeq`.
- Make the free list recycle its consumed head pages and allocate its own tail
  pages before extending the database file.
- Route `BTree.new` through free-list-first page allocation.
- Route `BTree.del` to the free-list tail.
- Keep pending updates for reused pages separate from committed file pages.
- Persist free-list head and tail state atomically with the tree root.
- Use the book's sequence boundary to prevent pages from the current or
  fallback version from being reused too early.
- Reclaim crash-created orphan pages after recovery reachability validation.

### Clarify append-only log responsibilities

- The copy-on-write page file and its checksummed root metadata are the sole
  authoritative source of committed database state.
- The experimental logical Set/Delete log has been removed because it was not
  part of the durable KV commit or recovery path.
- The database does not currently use a write-ahead log. Adding one later must
  be justified by a new requirement such as grouped transactions rather than
  duplicating the existing single-operation durability protocol.
- Any future WAL design requires transaction identifiers, an explicit commit
  record, page-commit ordering, checkpoint boundaries, bounded truncation, and
  documented torn-tail and checksum-corruption handling before implementation.

### Validate the completed storage engine

- Compare disk-backed behavior against a map reference model.
- Run invariant checks after long insert/delete sequences.
- Add reopen tests after root splits, merges, overwrites, and deletes.
- Test crashes before page sync, before metadata write, and before metadata
  sync.
- Run fuzz tests against encoded pages and persistent operation sequences.
- Compare B+Tree and durable-operation benchmarks with the map baseline.
