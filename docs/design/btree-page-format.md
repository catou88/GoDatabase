# B+Tree Page Format

## Goal

Define the future on-disk encoding for B+Tree nodes so the storage layer can
serialize nodes into fixed-size pages without changing the public database API.

This design is for the disk-backed implementation. The current B+Tree remains
in memory until search, insert, delete, range scan, and invariant checks are
stable.

## Page Size

Each B+Tree node is stored in one fixed-size page.

```go
const pageSize = 4096
```

Rationale:

- 4 KiB is a common operating system page size.
- Fixed-size pages make allocation and reuse simpler.
- Deleted pages can later be managed with a free list.
- Page numbers can be used as child references instead of byte offsets.

Large keys or values that cannot fit in one page are out of scope for the first
disk-backed implementation. The initial implementation should reject oversized
keys or values before writing a page.

## Byte Order

All integer fields are encoded using little-endian byte order.

Rationale:

- Go's `encoding/binary` package supports it directly.
- The format is explicit and portable across machines.
- Most development and CI machines for this project use little-endian CPUs.

## Page Header

Every page starts with a fixed-size header.

```text
| field     | size | description                         |
|-----------|------|-------------------------------------|
| magic     | 4B   | identifies a GoDatabase B+Tree page |
| version   | 2B   | page format version                 |
| node_type | 2B   | leaf or internal                    |
| key_count | 2B   | number of keys in the node          |
| reserved  | 6B   | reserved for future metadata        |
```

Header size: 16 bytes.

Node types:

```go
const (
	nodeTypeInternal = 1
	nodeTypeLeaf     = 2
)
```

Rationale:

- `magic` prevents decoding random bytes as a valid page.
- `version` gives future migrations a clear compatibility point.
- `node_type` keeps leaf and internal page behavior explicit.
- `key_count` allows decoders to find the pointer and offset arrays.
- `reserved` keeps room for future flags, checksums, or sibling page numbers
  without immediately changing the header size.

## Page Layout

After the header, the page stores child pointers, key/value offsets, and encoded
key/value records.

```text
| header | child pointers | offsets | key/value records | unused |
```

The unused region must be zero-filled when a new page is written.

## Child Pointer Encoding

Child pointers are encoded as page numbers.

```text
| child_0 | child_1 | ... |
|   8B    |   8B    | ... |
```

Rules:

- Internal pages store `key_count + 1` child page numbers.
- Leaf pages store no child page numbers.
- Page number `0` is reserved as invalid or empty.
- A valid root page number must be nonzero.

Rationale:

- Page numbers are stable across process restarts.
- Page numbers avoid tying the format to in-memory pointers.
- Reserving page `0` makes missing pointers easier to detect in tests.

## Offset Encoding

Offsets locate each encoded key/value record relative to the start of the
record area.

```text
| offset_0 | offset_1 | ... | offset_key_count |
|    2B    |    2B    | ... |        2B        |
```

Rules:

- The offsets array has `key_count + 1` entries.
- `offset_0` must be `0`.
- `offset_key_count` is the total byte length of all encoded records.
- Offsets must be non-decreasing.
- Every offset must stay within the page.

Rationale:

- Offsets allow direct access to the nth key/value record.
- Storing the final end offset makes page-size validation simple.
- Two-byte offsets are enough for a 4 KiB page.

## Key/Value Record Encoding

Each key/value record stores lengths followed by raw bytes.

```text
| key_size | value_size | key bytes | value bytes |
|    2B    |     2B     |    ...    |     ...     |
```

Rules:

- `key_size` must be greater than `0`.
- Leaf pages store both key bytes and value bytes.
- Internal pages store key bytes and use `value_size = 0`.
- Keys in a page must be sorted ascending by byte comparison.
- Duplicate keys are not allowed within a page.
- A page is invalid if any record points outside the page.

Rationale:

- Length-prefixed records avoid delimiter escaping.
- Raw bytes allow the future storage layer to encode strings without assuming a
  specific character set inside the page format.
- Internal pages do not store user values because values live only in leaves.

## Size Limits

The first disk-backed implementation should enforce conservative per-record
limits.

```go
const maxKeySize = 1000
const maxValueSize = 3000
```

A single maximum-sized leaf record must fit into one page with the header,
offsets, and record length fields.

Tests should verify that:

- maximum-sized valid records are accepted;
- records that exceed the limit are rejected;
- encoded pages never exceed `pageSize`;
- internal nodes can still store at least two separator keys.

## Compatibility Assumptions

Version `1` of the page format assumes:

- The database file is read and written by GoDatabase.
- Integers are always encoded little-endian.
- Page size is fixed at 4096 bytes for the database file.
- Page numbers identify fixed-size pages, not byte offsets.
- Keys are compared lexicographically by encoded bytes.
- Leaf values are opaque bytes to the page layer.
- Internal values are always empty.
- Page compression and encryption are not part of this format.

Any future incompatible change must increment the page format version.

## Derived Tests

Implementation tests should be derived directly from this format:

- Encode and decode an empty leaf page.
- Encode and decode a leaf page with multiple sorted key/value records.
- Encode and decode an internal page with child page numbers.
- Reject pages with an invalid magic value.
- Reject unsupported page versions.
- Reject unknown node types.
- Reject zero-length keys.
- Reject unsorted keys.
- Reject duplicate keys.
- Reject offsets that move backward.
- Reject offsets or records outside the page.
- Reject leaf pages with missing values only when the public API forbids them.
- Reject internal pages with non-empty values.
- Reject internal pages with the wrong number of child pointers.
- Verify encoded pages are exactly `pageSize` bytes.

## Non-Goals

- Implementing page serialization in this issue.
- Implementing page allocation or free lists.
- Implementing crash recovery or write-ahead logging.
- Supporting overflow pages for very large keys or values.
- Migrating the current public `db.Database` API.
