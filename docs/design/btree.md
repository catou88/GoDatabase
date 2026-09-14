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

## Future Disk-Backed Design

The first B+Tree implementation should stay in memory. Disk pages, byte-level
encoding, page numbers, free lists, and copy-on-write persistence should be
designed after the in-memory tree is correct and well-tested.

A later disk-backed design may represent nodes as fixed-size byte pages:

```go
type BNode []byte
```

That later design can add:

- 4 KiB pages
- page-number child references
- encoded node headers
- key/value offsets
- little-endian integer encoding
- maximum key/value size limits
- page allocation and reuse

Those details are intentionally out of scope for the initial in-memory B+Tree.

## Search Behavior

Search starts at the root.

For each internal node:

1. Find the first separator key greater than the search key.
2. Follow the child pointer at that position.
3. Repeat until reaching a leaf.

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

## Follow-Up Implementation Issues

- Implement B+Tree node search.
- Implement B+Tree `Get`.
- Implement B+Tree `Insert`.
- Implement split propagation into parent nodes.
- Add B+Tree invariant tests.
- Add B+Tree range scan support.
- Implement B+Tree `Delete`.
- Add B+Tree fuzz tests against a map reference model.
- Design page-backed B+Tree node format as a later persistence milestone.
