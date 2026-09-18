# Secondary Index Design

## Goal

Define how secondary indexes map indexed column values to primary keys in the
table key-value namespace.

Secondary indexes are maintained structures, not an independent source of row
data. The table row remains authoritative; an index entry only identifies a
row that must be fetched from the primary-key row key.

## Scope

The initial design supports indexes over one non-primary column whose type is
`int64`, `string`, `bytes`, or `bool`. Composite indexes, expression indexes,
partial indexes, and index-only row storage are deferred.

An index may be either:

- **unique:** at most one row may contain a given indexed value;
- **non-unique:** many rows may contain the same indexed value.

The index definition has a stable name, the table name, the indexed column ID,
and a uniqueness flag. Index names are non-empty, unique within a table, and
limited to 128 bytes.

## Key Namespace

Secondary index entries use the reserved index namespace:

```text
0x30 | escaped table name | escaped index name | index payload
```

The escaped components use the table-layer byte-string encoding: ordinary bytes
are unchanged, `0x00` becomes `0x00 0xFF`, and the component terminator is
`0x00 0x00`.

The index prefix is:

```text
0x30 | escaped table name | escaped index name
```

Index names are part of the persistent key so two indexes over the same column
cannot collide. A future numeric index ID may replace the name only in a new
format version or through an explicit migration.

## Indexed-Value Encoding

The indexed value uses the same order-preserving encoding as the table primary
key:

- `int64`: flip the sign bit and encode as big-endian;
- `string` and `bytes`: use a self-delimiting escaped byte encoding;
- `bool`: encode `false` as `0x00` and `true` as `0x01`.

The type is taken from the index definition. Values with another Go type are
rejected before any KV mutation. A nullable indexed value is encoded with a
reserved null marker before ordinary values. Null policy is explicit:

- unique indexes allow at most one `NULL` value;
- non-unique indexes may contain many `NULL` values.

This policy can be changed only in a new index format version because it affects
duplicate detection and scan results.

## Non-Unique Index Entries

Non-unique indexes include the primary key in the key so duplicate indexed
values have distinct entries:

```text
index prefix | encoded indexed value | encoded primary key
```

The value is empty. The primary key is already available in the key and the row
is retrieved from the table row key:

```text
0x20 | escaped table name | encoded primary key
```

Entries with the same indexed value are ordered by primary key. An index scan
can therefore return deterministic primary-key order for one indexed value.

## Unique Index Entries

Unique indexes omit the primary key from the index key and store the encoded
primary key as the value:

```text
index prefix | encoded indexed value  ->  encoded primary key
```

An existing key means the indexed value is already owned. The insert or update
must reject the operation unless the existing owner is the same primary key.
The index value lets point lookup and maintenance find the owning row without
scanning duplicate entries.

## Index Metadata

Index definitions are stored in the table descriptor rather than inferred by
scanning index entries. A descriptor extension contains:

```text
index_count       2 bytes
index_name        escaped bytes
column_id         2 bytes
index_flags       1 byte, bit 0 means unique
index_format      1 byte
```

Index descriptors are sorted by index name and validated when the table is
opened. Unknown flags or formats are errors. The table descriptor remains the
source of truth for which index keys are valid.

## Maintenance Rules

### Insert

1. Validate and encode the complete row.
2. Check every unique index for an existing owner of each indexed value.
3. Write the primary row entry.
4. Write one entry to each configured index.

The row and all index entries must become visible in one transaction before
multi-key transactions are available. Until then, the table layer must either
serialize the operation and use a recovery-safe mutation sequence or document
that index maintenance is not enabled. A partially applied insert must not be
reported as successful.

### Update

The initial public API has no update operation; replacing a row is deferred
until multi-key transactions are implemented. The required future sequence is:

1. Read the old row and derive old indexed values.
2. Validate the new row and check unique-index conflicts, ignoring its own
   primary key.
3. Remove old index entries whose values changed.
4. Write the new primary row.
5. Add new index entries.

The operation must be atomic. If an indexed value is unchanged, its index entry
may be retained, but the resulting entry must still identify the current row.

### Delete

1. Read the row by primary key.
2. Derive its indexed values.
3. Delete the primary row.
4. Delete the corresponding index entries.

Deletion of a missing row is a no-op. Deleting an index entry that is already
absent is safe. The primary row and index entries must be committed together;
otherwise a later index scan could reference a missing row.

## Lookup and Range Scans

An index lookup encodes the requested value and scans the corresponding index
prefix. Non-unique results decode primary keys from the key suffix; unique
results decode them from the value. Each primary key is then fetched from the
authoritative table row namespace.

Because the indexed value precedes the primary key, a range scan over an index
returns rows ordered first by indexed value and then by primary key. The scan
must stop at the exact index prefix and must ignore malformed or foreign keys.

The initial range API materializes results. A future iterator should provide
half-open bounds and avoid allocating every decoded row at once.

## Consistency and Recovery

Index entries are derived state. A committed tree is valid only when every
reachable index entry points to an existing row with the expected indexed
value, and every row requiring an index has the corresponding entry.

Recovery must validate index ownership and either:

1. select a previous complete committed generation; or
2. fail open with a corruption error.

It must never silently expose a partially maintained index as correct data.
Index rebuild from primary rows is a future repair operation and must publish a
new committed generation only after all entries are written and validated.

## Limitations and Follow-Up Work

- Multi-key transactions are required for atomic row and index maintenance.
- Concurrent unique inserts require conditional writes or transaction conflict
  handling; a check followed by `Set` is not sufficient by itself.
- Schema changes must define index backfill, removal, and compatibility rules.
- Composite, partial, and expression indexes are deferred.
- Index entries increase write amplification and durable synchronization cost.
- Benchmarks must be captured before and after index maintenance is added.
- Add descriptor encoding tests, duplicate-value tests, update/delete tests,
  restart tests, corruption tests, and index-rebuild tests before exposing the
  public index API.
