# Table Layer Design

## Goal

Define how relational table metadata and rows map onto the durable key-value
engine while preserving ordered primary-key scans.

The first table layer should be small, deterministic, and compatible with the
current single-key commit model. It should not require transactions merely to
create a table or insert one row.

## Non-Goals

The initial table layer does not provide:

- SQL parsing or query planning;
- composite primary keys;
- secondary indexes;
- foreign keys or uniqueness constraints beyond the primary key;
- atomic multi-row mutations;
- table renaming or online schema migration;
- locale-aware string collation;
- rows or keys larger than the current KV limits.

## Ownership Of The KV Keyspace

The table layer owns the complete KV keyspace of a database file. Applications
must not mix raw `db.Set` keys with table operations in the same file because a
raw key could collide with an internal table key.

The first byte identifies the logical keyspace:

```text
0x10  table descriptors
0x20  table rows
0x30  reserved for secondary indexes
0x40  reserved for future table-layer metadata
```

The prefixes are format details, not public API values. Unknown keyspace bytes
must be rejected by table-level validation rather than interpreted as rows.

## Ordered Byte Encoding

The B+Tree compares keys lexicographically by bytes. Every key component used
for ordered scans must therefore have an order-preserving encoding.

### Escaped byte strings

Table names and string primary keys use a terminated byte-string encoding:

```text
ordinary byte       -> unchanged
0x00                -> 0x00 0xFF
end of component    -> 0x00 0x00
```

This encoding is unambiguous and preserves raw byte ordering. A shorter string
sorts before a longer string with the same prefix. Go strings are treated as
opaque bytes; UTF-8 is allowed but not required, and no locale collation or
Unicode normalization is applied.

### Signed integers

An `int64` primary key is transformed and written in big-endian order:

```go
ordered := uint64(value) ^ (uint64(1) << 63)
```

Flipping the sign bit maps signed integer order to unsigned byte order. Writing
the result big-endian ensures lexicographic B+Tree order matches numeric order.

## Table Identity

The initial design uses the table name as its persistent identity. Names must be
non-empty, unique, at most 128 bytes, and encoded with the escaped byte-string
format.

Using the name avoids a catalog counter and lets table creation commit one
descriptor key atomically. A numeric table ID would require atomically updating
both an ID allocator and a descriptor, which the current engine cannot do until
multi-key transactions exist.

Table renaming is therefore unsupported in the initial format. A later
transactional catalog may introduce stable numeric IDs through a new table
format version and migration.

## Table Metadata

Each table has one descriptor entry.

Metadata key:

```text
0x10 | escaped table name
```

Metadata value, version 1:

```text
magic          4 bytes   "GDTB"
format_version 2 bytes   unsigned, little-endian
flags          2 bytes   reserved, must be zero
schema_version 4 bytes   unsigned, little-endian
name_length    2 bytes   unsigned, little-endian
name           variable  raw table-name bytes
primary_id     2 bytes   stable primary-key column ID
column_count   2 bytes   number of column descriptors
columns        variable  repeated column descriptors
```

Each column descriptor is encoded as:

```text
column_id      2 bytes   stable, nonzero identifier
column_type    1 byte    known type code
column_flags   1 byte    nullable and primary-key flags
name_length    2 bytes   unsigned, little-endian
name           variable  raw column-name bytes
```

Initial column type codes represent:

```text
1  int64
2  string
3  bytes
4  bool
```

Descriptor rules:

- Format version starts at `1`.
- Schema version starts at `1` and increases after a supported schema change.
- Column IDs are unique and remain stable across schema versions.
- Column names are non-empty, unique, and at most 128 bytes.
- Exactly one column is the primary key.
- The primary key is non-nullable and initially must be `int64` or `string`.
- Columns are encoded in ascending column-ID order.
- Reserved flags and unknown type codes are rejected.
- The complete descriptor must fit within the KV value-size limit.
- The table name stored in the value must match the name encoded in the key.

Storing the name in both places is deliberate. It makes descriptor corruption
and incorrect key construction easier to detect during decoding.

## Primary-Key Representation

The initial table API accepts one schema-typed primary-key value:

- `int64` uses sign-bit-flipped big-endian encoding.
- `string` uses escaped terminated bytes.

The primary-key type comes from the table descriptor and is not repeated in
every row key. A value with the wrong Go or schema type is rejected before KV
access.

Primary keys cannot be null. Duplicate primary keys identify the same KV key,
so insertion policy must be explicit: `Insert` rejects an existing key, while a
future `Upsert` operation may replace it.

Composite primary keys are deferred. A later format can concatenate
self-delimiting components in schema order, but must define mixed-type ordering
and version compatibility first.

## Row-Key Encoding

Every row is stored under:

```text
0x20 | escaped table name | encoded primary key
```

The row prefix is:

```text
0x20 | escaped table name
```

All rows for one table are contiguous because they share that prefix. Within
the prefix, row order is primary-key order. Different tables cannot overlap
because the escaped table name is terminated.

The primary key is not duplicated in the row value. Point lookup constructs the
row key directly, and scans recover the primary key from the key suffix.

The complete encoded row key, including namespace and table name, must fit the
KV key-size limit of 1000 bytes.

## Row-Value Encoding

Row values contain non-primary columns and reference columns by stable ID rather
than position. This allows decoding to detect schema mismatches and leaves room
for limited future schema evolution.

Row value, version 1:

```text
magic          4 bytes   "GDRW"
format_version 2 bytes   unsigned, little-endian
schema_version 4 bytes   descriptor version used to encode the row
field_count    2 bytes   number of encoded non-primary fields
fields         variable  repeated fields sorted by column ID
```

Each field is encoded as:

```text
column_id      2 bytes   stable column identifier
field_flags    1 byte    bit 0 means NULL; other bits must be zero
value_length   4 bytes   unsigned, little-endian
value          variable  encoded field bytes
```

Field values use these encodings:

- `int64`: 8-byte two's-complement value in little-endian order.
- `string`: raw string bytes without a terminator.
- `bytes`: raw bytes.
- `bool`: one byte, `0` or `1`.
- `NULL`: null flag set, zero value length, and no value bytes.

An empty string or byte slice has a zero length with the null flag clear, so it
is distinct from `NULL`.

Decoder rules:

- Field IDs are strictly increasing and cannot repeat.
- The primary-key field must not appear in the value.
- Unknown fields, unknown flags, malformed lengths, and invalid fixed-width
  values return errors.
- Every non-primary column must be present exactly once.
- Nullable columns use an explicit `NULL` field when they have no value.
- The row schema version must be supported by the loaded descriptor.
- The complete row value must fit the KV value-size limit of 3000 bytes.

## Operation Mapping

### Create table

1. Validate the descriptor and encode its metadata key and value.
2. Check whether the descriptor key already exists.
3. Store the one descriptor entry with durable `Set`.

Because creation writes one KV entry, it is atomic under the current
single-operation durability guarantee. However, the existence check and `Set`
are separate operations, so concurrent attempts to create the same table must
be serialized until conditional writes or transactions exist.

### Insert row

1. Load and validate the table descriptor.
2. Validate the primary key and column values.
3. Encode the row key and row value.
4. Check whether the row key already exists.
5. Store the row with durable `Set` only when it is absent.

The current KV API has no atomic compare-and-set operation. Concurrent inserts
of the same primary key therefore require table-layer serialization until
transactions or conditional writes exist.

### Get by primary key

1. Encode the row key from the table name and typed primary key.
2. Call KV `Get`.
3. Decode and validate the row against the descriptor.
4. Return the primary key from the requested key plus decoded non-primary
   fields.

### Delete by primary key

Encode the row key and call KV `Delete`. Missing rows return the same normal
not-found result as the underlying KV engine.

## Primary-Key Range Scans

For an inclusive range `[start, end]`, construct:

```text
start key = row prefix | encode(start primary key)
end key   = row prefix | encode(end primary key)
```

Passing those keys to the ordered B+Tree range operation returns rows in
ascending primary-key order. The table layer must verify both bounds match the
descriptor's primary-key type and return an empty result when `start > end`.

For a full-table scan, calculate the exclusive lexicographic successor of the
row prefix. The current KV `Range` API has inclusive bounds, so the initial
implementation may scan through that successor and discard any result that does
not retain the exact row prefix. A future iterator should expose half-open
`[start, end)` bounds and avoid materializing the complete result slice.

String ranges use raw byte order, not human-language collation. Integer ranges
use numeric order because of the sign-bit-flipped big-endian encoding.

## Durability And Recovery

Each descriptor or row mutation inherits the durable KV guarantees: new pages
are written copy-on-write and root metadata is synchronized before success is
returned. Reopening recovers one complete committed tree generation.

The initial table layer does not make a sequence of row operations atomic. A
crash may preserve any operations that individually returned success. Atomic
multi-row changes, table deletion, and index maintenance require transactions.

## Compatibility

Metadata and row values carry independent format versions. Decoders must reject
unsupported versions instead of guessing. Changes to key encoding are
especially sensitive because they alter ordering and lookup identity; an
incompatible key change requires a new namespace or an explicit migration.

Column IDs and schema versions are reserved now so future schema work can make
compatibility decisions without relying on mutable column positions.

## Limitations

- Table names are persistent identities and cannot be renamed.
- Only one `int64` or string primary-key column is supported initially.
- Raw byte ordering is the only string collation.
- Rows must fit the 3000-byte KV value limit; overflow storage is unavailable.
- Encoded row keys must fit the 1000-byte KV key limit.
- Descriptor size limits the number and length of columns.
- `Insert` cannot atomically check absence and write under concurrent writers.
- Multi-row operations and DDL are not transactional.
- Dropping a table requires scanning and deleting rows and is not atomic.
- Schema migration, default values, and generated columns are not defined.
- Secondary indexes and constraints are deferred.
- Mixing raw KV operations with table operations is unsupported.
- The table layer inherits the durable engine's single-handle and corruption
  limitations.

## Follow-Up Implementation Tasks

1. Add ordered encoders and decoders for escaped strings and signed integers,
   with boundary and malformed-input tests.
2. Add versioned table-descriptor encoding, decoding, and validation tests.
3. Add versioned row-value encoding, decoding, and schema-validation tests.
4. Define public table, column, row, and typed-value APIs without exposing
   internal key encodings.
5. Implement durable `CreateTable` and descriptor lookup.
6. Implement insert with duplicate-primary-key detection.
7. Implement point Get and Delete by primary key.
8. Implement bounded and full-table primary-key range scans.
9. Add restart and black-box public integration tests.
10. Add table benchmarks for insert, point lookup, Delete, and range scans;
    capture a committed `before` result before optimizing the table layer.
11. Add transactions or conditional writes before claiming concurrent insert
    uniqueness or atomic index maintenance.
12. Design secondary-index keys only after the primary table encoding is stable.
