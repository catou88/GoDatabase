# Input Validation Semantics

## Goal

Define how GoDatabase handles empty keys, empty values, missing keys, and range bounds before the storage internals become more complex.

## Decisions

### Keys

Empty keys should not be accepted for write operations.

```go
db.Set("", "value")
```

should return an error.

Reason: empty keys make future storage encoding, debugging, and API behavior harder to reason about. Rejecting them before adding B+Tree and persistence layers keeps the storage model simpler.

### Values

Empty values are allowed.

```go
db.Set("alpha", "")
```

should succeed.

Reason: an empty string is a valid value and is different from a missing key.

### Missing Keys

Getting a missing key returns the zero value and `false`.

```go
value, ok, err := db.Get("missing")
```

Expected behavior:

```go
value == ""
ok == false
err == nil
```

Deleting a missing key returns `false`.

```go
deleted, err := db.Delete("missing")
```

Expected behavior:

```go
deleted == false
err == nil
```

Reason: this follows common Go map-style behavior and keeps read/delete operations simple.

### Range Bounds

Range bounds are inclusive.

```go
db.Range("a", "c")
```

includes keys `"a"` and `"c"` when they exist.

If `start > end`, `Range` returns an empty slice.

```go
db.Range("c", "a")
```

Expected behavior:

```go
[]Item{}
```

Reason: the requested interval is empty, so returning no results is simple and predictable.

### Empty Range Bounds

Empty range bounds are not magic unbounded markers.

Go compares strings in lexicographic byte order. The empty string `""` has zero bytes and sorts before any non-empty string.

```go
db.Range("a", "")
```

does not mean "return everything on or after `a`." Since `"a" > ""`, this follows the `start > end` rule and returns an empty slice.

```go
db.Range("", "c")
```

means "return keys where `"" <= key <= "c"`." For typical non-empty keys, this returns keys less than or equal to `"c"`.

Reason: overloading empty strings to mean "no bound" creates ambiguity. Open-ended scans should be added later with explicit APIs such as `RangeFrom`, `RangeTo`, or a scan options type.

## API Impact

All operations return errors so durable databases can report closed handles,
I/O failures, and corrupt storage without hiding them:

```go
func (d *Database) Set(key, value string) error
func (d *Database) Get(key string) (string, bool, error)
func (d *Database) Delete(key string) (bool, error)
func (d *Database) Range(start, end string) ([]Item, error)
```

Missing keys and empty ranges are normal results and return a `nil` error.
Operations on a closed database return `ErrClosed`.

## Follow-Up Work

- Add tests for empty key behavior.
- Update `Set` to reject empty keys.
- Keep empty values allowed.
- Document validation behavior in package comments or README examples.
- Consider explicit open-ended range APIs later, such as `RangeFrom` and `RangeTo`.
