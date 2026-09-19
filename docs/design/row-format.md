# Relational format version 2

This document defines the descriptor, row, value, and secondary-index codecs.
All lengths, IDs, counts, and format versions are unsigned little-endian.
Integer value payloads are the exception described below. Names are byte strings.

## Compatibility

Descriptor (`GDTB`), row (`GDRW`), and index metadata (`GDXI`) versions are
all 2. Decoders reject every other version with
`ErrUnsupportedRelationalFormat`, also wrapping `ErrInvalidSchema` or
`ErrInvalidRow`. There is no automatic migration or silent reinterpretation.
Descriptor lookup keys remain unchanged so opening a version-1 table finds its
descriptor and explicitly rejects it. Version-1 row and index metadata records
are also rejected independently when decoded.

Export relational data with the original software and import into a new database
with this version. Old NULL/empty ambiguities cannot be recovered from bytes
alone. Do not change version bytes in place. Indexes must be rebuilt on import.
Raw public KV keys, values, storage pages, and APIs are unchanged; arbitrary
raw KV data is not interpreted as relational data except through table APIs.

## Components and values

`escape(b)` replaces each zero byte with `00 ff`, leaves other bytes unchanged,
and appends `00 00`. Decoding requires exactly one terminal delimiter, valid
escapes, and no trailing data.

Primary keys retain their original representation: strings are raw bytes,
including empty strings and embedded zeros; int64 uses eight big-endian bytes
with the sign bit flipped. This preserves signed numeric ordering. Primary keys
cannot be NULL. The row-key prefix supplies their left boundary and the end of
the key supplies their right boundary.

Non-primary values are self-delimiting:

- NULL: a single `00`, permitted only for nullable columns.
- int64: tag `01`, followed by the eight-byte primary integer representation.
- string: tag `02`, followed by `escape(string bytes)`.
- bytes: tag `03`, followed by `escape(bytes)`.
- bool: tag `04`, followed by exactly `00` or `01`.

An untyped Go `nil` is NULL. A typed `[]byte(nil)` is an empty bytes value, like
`[]byte{}`. Empty strings, empty bytes, integer zero, and false remain non-NULL.
Decoders require the schema's type tag, exact fixed widths, and canonical
variable components. Missing nullable row fields encode as NULL. SQL NULL
support is outside this format change.

Escaping intentionally increases stored string/bytes payload size: one type tag,
two terminator bytes, and one extra byte for each embedded zero. Existing size
limits apply to the encoded representation, so a payload that fit the old format
may exceed the limit in version 2.

## Keys and records

- Descriptor key: `10 | escape(table name)`.
- Row key: `20 | escape(table name) | raw primary key`.
- Index metadata key: `11 | escape(table name) | escape(index name)`.
- Index entry prefix: `30 | escape(table name) | escape(index name)`.
- Unique index key: entry prefix followed by one encoded index value.
- Nonunique index key: unique key followed by the raw primary key.
- Every index entry value contains the raw primary key.

`encodeIndexValue` uses the non-primary value format even if passed a primary
column. `indexValuePrefix` is the shared equality prefix used by entry encoding
and lookup. Termination prevents `(a, bc)` and `(ab, c)` collisions and prevents
an equality lookup for `a` from matching `ab`. Unique indexes allow at most one
NULL, because NULL has a single canonical index key.

Descriptor: `GDTB | u16(2) | u16(0) | u32(1) | u16(name length) | name |
u16(primary column ID) | u16(column count)`, followed by columns in schema order:
`u16(ID) | u8(type) | u8(nullable) | u16(name length) | name`.
IDs are consecutive starting at 1; nullable is 0 or 1 and must be 0 on the
primary column. Schema validation applies and trailing bytes are rejected.

Row: `GDRW | u16(2) | u32(1) | u16(non-primary field count)`, followed by each
non-primary field in schema order: `u16(column ID) | u8(0) | u8(0) |
u32(encoded value length) | encoded value`. The primary value lives only in
the row key. Decoding rejects missing/duplicate/out-of-order IDs, primary fields,
unknown IDs, nonzero reserved bytes, invalid values, truncation, and trailing
bytes. Encoded row values are limited to 3000 bytes and row keys to 1000 bytes.

Index metadata: `GDXI | u16(2) | u8(unique) | u8(0) | u16(column name length) |
column name`. Unique is 0 or 1. Names are nonempty and at most 128 bytes.
Reserved bytes, unsupported flags, bad lengths, and trailing bytes are rejected.
