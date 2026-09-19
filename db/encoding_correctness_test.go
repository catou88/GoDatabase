package db

import (
	"bytes"
	"encoding/binary"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func codecSchema() TableSchema {
	return TableSchema{Name: "codec", Columns: []Column{{Name: "id", Type: ColumnString, PrimaryKey: true}, {Name: "v", Type: ColumnBytes, Nullable: true}}}
}

func TestCodecValueRoundTrips(t *testing.T) {
	for _, tc := range []struct {
		typ    ColumnType
		values []any
	}{
		{ColumnInt64, []any{nil, int64(0), int64(-1), int64(-1 << 63), int64(1<<63 - 1)}},
		{ColumnString, []any{nil, "", "a", "a\x00\xff"}},
		{ColumnBytes, []any{nil, []byte{}, []byte{0, 255}, []byte(nil)}},
		{ColumnBool, []any{nil, false, true}},
	} {
		c := Column{Type: tc.typ, Nullable: true}
		for _, v := range tc.values {
			encoded, err := encodeValue(c, v)
			if err != nil {
				t.Fatal(err)
			}
			got, err := decodeValue(c, encoded)
			if err != nil {
				t.Fatal(err)
			}
			if b, ok := v.([]byte); ok {
				if got == nil || !bytes.Equal(got.([]byte), b) {
					t.Fatalf("bytes round trip: %#v", got)
				}
			} else if !reflect.DeepEqual(got, v) {
				t.Fatalf("round trip %#v: %#v", v, got)
			}
			if _, err := decodeValue(c, append(encoded, 0)); err == nil {
				t.Fatalf("accepted trailing data %x", encoded)
			}
		}
	}
}

func TestCodecCompositeBoundaries(t *testing.T) {
	i := Index{Name: "v", Column: "v"}
	for _, typ := range []ColumnType{ColumnString, ColumnBytes} {
		var values []any
		if typ == ColumnString {
			values = []any{nil, "", "a", "ab", "a\x00"}
		} else {
			values = []any{nil, []byte{}, []byte("a"), []byte("ab"), []byte{'a', 0}}
		}
		for x, value := range values {
			encoded, err := encodeIndexValue(Column{Type: typ, Nullable: true}, value)
			if err != nil {
				t.Fatal(err)
			}
			prefix := indexValuePrefix("t", i, encoded)
			for y, other := range values {
				e, _ := encodeIndexValue(Column{Type: typ, Nullable: true}, other)
				key := indexEntryKey("t", i, e, []byte("bc\x00"))
				if bytes.HasPrefix([]byte(key), prefix) != (x == y) {
					t.Fatalf("ambiguous index %d/%d", x, y)
				}
			}
		}
	}
}

func TestCodecPrimaryKeysRemainRaw(t *testing.T) {
	for _, value := range []string{"", "a", "a\x00\xff"} {
		c := Column{Type: ColumnString, PrimaryKey: true}
		encoded, _ := encodeValue(c, value)
		got, err := decodePrimaryKey(c, encoded)
		if string(encoded) != value || got != value || err != nil {
			t.Fatal("string primary changed")
		}
	}
	c := Column{Type: ColumnInt64, PrimaryKey: true}
	var previous []byte
	for _, value := range []int64{-1 << 63, -1, 0, 1, 1<<63 - 1} {
		encoded, _ := encodeValue(c, value)
		got, err := decodePrimaryKey(c, encoded)
		if len(encoded) != 8 || got != value || err != nil || (previous != nil && bytes.Compare(previous, encoded) >= 0) {
			t.Fatal("integer primary changed")
		}
		previous = encoded
	}
}

func TestCodecStrictRecords(t *testing.T) {
	s := codecSchema()
	descriptor := []byte(encodeDescriptor(s))
	_, row, err := encodeRow(s, map[string]any{"id": "", "v": []byte{}})
	if err != nil {
		t.Fatal(err)
	}
	metadata := []byte(encodeIndexMetadata(Index{Name: "i", Column: "v"}))
	for _, tc := range []struct {
		data   []byte
		decode func([]byte) error
	}{
		{descriptor, func(b []byte) error { _, e := decodeDescriptor(string(b)); return e }},
		{[]byte(row), func(b []byte) error { _, e := decodeRow(s, b); return e }},
		{metadata, func(b []byte) error { _, e := decodeIndexMetadata("i", b); return e }},
	} {
		if e := tc.decode(tc.data); e != nil {
			t.Fatal(e)
		}
		for n := 0; n < len(tc.data); n++ {
			if tc.decode(tc.data[:n]) == nil {
				t.Fatalf("accepted truncation %d", n)
			}
		}
		if tc.decode(append(append([]byte{}, tc.data...), 0)) == nil {
			t.Fatal("accepted trailing byte")
		}
		old := append([]byte{}, tc.data...)
		binary.LittleEndian.PutUint16(old[4:6], 1)
		if !errors.Is(tc.decode(old), ErrUnsupportedRelationalFormat) {
			t.Fatal("legacy format not explicitly rejected")
		}
	}
	for _, offset := range []int{6, 7, 8} {
		bad := append([]byte{}, descriptor...)
		bad[offset] ^= 2
		if _, err := decodeDescriptor(string(bad)); err == nil {
			t.Fatal("accepted descriptor reserved/version field")
		}
	}
	bad := []byte(row)
	bad[15] = 1
	if _, err := decodeRow(s, bad); err == nil {
		t.Fatal("accepted row reserved field")
	}
	for _, b := range [][]byte{{}, {2}, {2, 0}, {2, 0, 1, 0, 0}, {2, 0, 0, 0, 0}} {
		if _, err := decodeValue(Column{Type: ColumnString}, b); err == nil {
			t.Fatalf("accepted malformed string %x", b)
		}
	}
}

func TestLegacyTableFileRequiresExplicitMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	schema := codecSchema()
	legacy := []byte(encodeDescriptor(schema))
	binary.LittleEndian.PutUint16(legacy[4:6], 1)
	if err := d.Set(descriptorKey(schema.Name), string(legacy)); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	if _, err := reopened.OpenTable(schema.Name); !errors.Is(err, ErrUnsupportedRelationalFormat) {
		t.Fatalf("legacy OpenTable = %v", err)
	}
	stored, found, err := reopened.Get(descriptorKey(schema.Name))
	if err != nil || !found || stored != string(legacy) {
		t.Fatalf("legacy bytes changed: found=%v, err=%v", found, err)
	}
}

func TestUniqueIndexNullAndEmptyAreDistinct(t *testing.T) {
	d := New()
	table, err := d.CreateTable(codecSchema())
	if err != nil {
		t.Fatal(err)
	}
	if err := table.CreateIndex(Index{Name: "bytes", Column: "v", Unique: true}); err != nil {
		t.Fatal(err)
	}
	for _, row := range []map[string]any{{"id": "null", "v": nil}, {"id": "empty", "v": []byte{}}} {
		if err := table.Insert(row); err != nil {
			t.Fatal(err)
		}
	}
	if err := table.Insert(map[string]any{"id": "other", "v": nil}); !errors.Is(err, ErrDuplicateIndexed) {
		t.Fatalf("duplicate NULL = %v", err)
	}
	if err := table.Insert(map[string]any{"id": "other", "v": []byte(nil)}); !errors.Is(err, ErrDuplicateIndexed) {
		t.Fatalf("duplicate empty bytes = %v", err)
	}
	if err := table.Insert(map[string]any{"id": "other", "v": []byte("ok"), "extra": true}); !errors.Is(err, ErrInvalidRow) {
		t.Fatalf("unknown column = %v", err)
	}
	for _, tc := range []struct {
		value any
		id    string
	}{{nil, "null"}, {[]byte{}, "empty"}} {
		rows, err := table.FindByIndex("bytes", tc.value)
		if err != nil || len(rows) != 1 || rows[0]["id"] != tc.id {
			t.Fatalf("index %v = %v, %v", tc.value, rows, err)
		}
	}
}
