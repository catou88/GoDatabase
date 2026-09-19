package db

import (
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
)

// ColumnType identifies the supported table column representation.
type ColumnType uint8

const (
	ColumnInt64 ColumnType = iota + 1
	ColumnString
	ColumnBytes
	ColumnBool
)

// Column describes one table column. Exactly one column must have PrimaryKey set.
type Column struct {
	Name       string
	Type       ColumnType
	PrimaryKey bool
	Nullable   bool
}

// TableSchema describes a table created by CreateTable.
type TableSchema struct {
	Name    string
	Columns []Column
}

var (
	ErrTableExists      = errors.New("table already exists")
	ErrTableNotFound    = errors.New("table not found")
	ErrDuplicatePrimary = errors.New("duplicate primary key")
	ErrInvalidSchema    = errors.New("invalid table schema")
	ErrInvalidRow       = errors.New("invalid table row")
)

// Table is a handle for operations on one durable or in-memory table.
type Table struct {
	db      *Database
	schema  TableSchema
	indexes map[string]Index
}

// Scan returns all rows in ascending primary-key order, including empty string
// keys. It scans only this table's row prefix and takes no value bounds.
func (t *Table) Scan() ([]map[string]any, error) {
	t.db.mu.RLock()
	defer t.db.mu.RUnlock()
	if t.db.closed {
		return nil, ErrClosed
	}
	entries, err := tableRowsLocked(t.db, t.schema.Name)
	if err != nil {
		return nil, err
	}
	return t.decodeRows(entries)
}

// Range returns rows whose primary keys are between start and end, inclusive.
// Rows are returned in ascending primary-key order.
func (t *Table) Range(start, end any) ([]map[string]any, error) {
	t.db.mu.RLock()
	defer t.db.mu.RUnlock()
	if t.db.closed {
		return nil, ErrClosed
	}
	startKey, err := t.rowKey(start)
	if err != nil {
		return nil, err
	}
	endKey, err := t.rowKey(end)
	if err != nil {
		return nil, err
	}
	if startKey > endKey {
		return []map[string]any{}, nil
	}
	entries, err := t.db.rangeLocked(startKey, endKey)
	if err != nil {
		return nil, err
	}
	return t.decodeRows(entries)
}

func (t *Table) decodeRows(entries []Item) ([]map[string]any, error) {
	prefix := rowPrefix(t.schema.Name)
	rows := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		if !hasPrefix([]byte(entry.Key), prefix) {
			continue
		}
		row, err := decodeRow(t.schema, []byte(entry.Value))
		if err != nil {
			return nil, err
		}
		primary, err := decodePrimaryKey(t.primaryColumn(), []byte(entry.Key[len(prefix):]))
		if err != nil {
			return nil, err
		}
		row[t.primaryColumn().Name] = primary
		rows = append(rows, row)
	}
	return rows, nil
}

// Get returns the row identified by primaryKey and whether it exists.
// Values are returned using the Go type implied by each column.
func (t *Table) Get(primaryKey any) (map[string]any, bool, error) {
	t.db.mu.RLock()
	defer t.db.mu.RUnlock()
	if t.db.closed {
		return nil, false, ErrClosed
	}
	key, err := t.rowKey(primaryKey)
	if err != nil {
		return nil, false, err
	}
	raw, found, err := t.db.getLocked(key)
	if err != nil || !found {
		return nil, found, err
	}
	row, err := decodeRow(t.schema, []byte(raw))
	if err != nil {
		return nil, false, err
	}
	row[t.primaryColumn().Name] = primaryKey
	return row, true, nil
}

// CreateTable creates a table and returns its handle. Table names are unique.
func (d *Database) CreateTable(schema TableSchema) (*Table, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, ErrClosed
	}
	if d.writerActive {
		return nil, ErrWriteTransactionActive
	}
	if err := validateSchema(schema); err != nil {
		return nil, err
	}
	key := descriptorKey(schema.Name)
	if _, found, err := d.getLocked(key); err != nil {
		return nil, err
	} else if found {
		return nil, ErrTableExists
	}
	descriptor := encodeDescriptor(schema)
	if len(descriptor) > 3000 {
		return nil, fmt.Errorf("%w: descriptor exceeds 3000 bytes", ErrInvalidSchema)
	}
	if err := d.setLocked(key, descriptor); err != nil {
		return nil, err
	}
	return &Table{db: d, schema: cloneSchema(schema), indexes: make(map[string]Index)}, nil
}

// OpenTable opens an existing table by name.
func (d *Database) OpenTable(name string) (*Table, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return nil, ErrClosed
	}
	value, found, err := d.getLocked(descriptorKey(name))
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrTableNotFound
	}
	schema, err := decodeDescriptor(value)
	if err != nil {
		return nil, err
	}
	if schema.Name != name {
		return nil, fmt.Errorf("%w: descriptor name does not match catalog key", ErrInvalidSchema)
	}
	indexes, err := loadIndexesLocked(d, schema)
	if err != nil {
		return nil, err
	}
	return &Table{db: d, schema: schema, indexes: indexes}, nil
}

// Schema returns a copy of the table schema.
func (t *Table) Schema() TableSchema { return cloneSchema(t.schema) }

// Insert adds row. Existing primary keys are rejected.
func (t *Table) Insert(row map[string]any) error {
	return t.mutate(func(tx *Tx) error { return tx.insertRowLocked(t, row) })
}

// Update replaces an existing row and its secondary-index entries atomically.
func (t *Table) Update(row map[string]any) error {
	return t.mutate(func(tx *Tx) error { return tx.updateRowLocked(t, row) })
}

// Delete removes a row and its secondary-index entries atomically.
func (t *Table) Delete(primaryKey any) (bool, error) {
	var deleted bool
	err := t.mutate(func(tx *Tx) error {
		var err error
		deleted, err = tx.deleteRowLocked(t, primaryKey)
		return err
	})
	if err != nil {
		return false, err
	}
	return deleted, nil
}

// Standalone mutations retain the mutex through validation and publication.
// An explicit transaction owns writer admission across multiple API calls.
func (t *Table) mutate(change func(*Tx) error) error {
	t.db.mu.Lock()
	defer t.db.mu.Unlock()
	if t.db.closed {
		return ErrClosed
	}
	if t.db.writerActive {
		return ErrWriteTransactionActive
	}
	if err := t.refreshIndexesLocked(); err != nil {
		return err
	}
	tx := &Tx{db: t.db, changes: make(map[string]transactionChange)}
	if err := change(tx); err != nil {
		return err
	}
	return tx.commitLocked()
}

type indexEntry struct{ key, value string }

// refreshIndexesLocked loads only committed definitions. The caller holds mu.
func (t *Table) refreshIndexesLocked() error {
	indexes, err := loadIndexesLocked(t.db, t.schema)
	if err != nil {
		return err
	}
	t.indexes = indexes
	return nil
}

func (t *Table) rowKey(primaryKey any) (string, error) {
	encoded, err := encodeValue(t.primaryColumn(), primaryKey)
	if err != nil {
		return "", err
	}
	key := append(rowPrefix(t.schema.Name), encoded...)
	if len(key) > 1000 {
		return "", ErrInvalidRow
	}
	return string(key), nil
}

func (t *Table) primaryColumn() Column { return findPrimary(t.schema) }

func (d *Database) getLocked(key string) (string, bool, error) {
	if d.durable != nil {
		v, ok, err := d.durable.Get([]byte(key))
		return string(v), ok, translateError(err)
	}
	v, ok := d.data[key]
	return v, ok, nil
}

func (d *Database) setLocked(key, value string) error {
	if d.durable != nil {
		return translateError(d.durable.Set([]byte(key), []byte(value)))
	}
	d.data[key] = value
	return nil
}

func (d *Database) rangeLocked(start, end string) ([]Item, error) {
	if d.durable != nil {
		entries, err := d.durable.Range([]byte(start), []byte(end))
		if err != nil {
			return nil, translateError(err)
		}
		items := make([]Item, len(entries))
		for i, entry := range entries {
			items[i] = Item{Key: string(entry.Key), Value: string(entry.Value)}
		}
		return items, nil
	}
	keys := make([]string, 0, len(d.data))
	for key := range d.data {
		if key >= start && key <= end {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	items := make([]Item, len(keys))
	for i, key := range keys {
		items[i] = Item{Key: key, Value: d.data[key]}
	}
	return items, nil
}

func hasPrefix(value, prefix []byte) bool {
	return len(value) >= len(prefix) && string(value[:len(prefix)]) == string(prefix)
}

func validateSchema(s TableSchema) error {
	if len(s.Name) == 0 || len(s.Name) > 128 || len(s.Columns) == 0 || len(s.Columns) > 65535 {
		return ErrInvalidSchema
	}
	seen := map[string]bool{}
	primary := 0
	for _, c := range s.Columns {
		if c.Name == "" || len(c.Name) > 128 || seen[c.Name] || c.Type < ColumnInt64 || c.Type > ColumnBool {
			return ErrInvalidSchema
		}
		seen[c.Name] = true
		if c.PrimaryKey {
			primary++
			if c.Nullable || (c.Type != ColumnInt64 && c.Type != ColumnString) {
				return ErrInvalidSchema
			}
		}
	}
	if primary != 1 {
		return ErrInvalidSchema
	}
	return nil
}
func cloneSchema(s TableSchema) TableSchema {
	s.Columns = append([]Column(nil), s.Columns...)
	return s
}
func descriptorKey(name string) string { return string(append([]byte{0x10}, escape([]byte(name))...)) }
func rowPrefix(name string) []byte     { return append([]byte{0x20}, escape([]byte(name))...) }
func escape(v []byte) []byte {
	out := make([]byte, 0, len(v)+1)
	for _, b := range v {
		if b == 0 {
			out = append(out, 0, 0xff)
		} else {
			out = append(out, b)
		}
	}
	return append(out, 0, 0)
}

func encodeDescriptor(s TableSchema) string {
	b := []byte("GDTB")
	var x [4]byte
	binary.LittleEndian.PutUint16(x[:2], 2)
	b = append(b, x[:2]...)
	b = append(b, 0, 0)
	binary.LittleEndian.PutUint32(x[:], 1)
	b = append(b, x[:]...)
	binary.LittleEndian.PutUint16(x[:2], uint16(len(s.Name)))
	b = append(b, x[:2]...)
	b = append(b, s.Name...)
	pk := uint16(0)
	for i, c := range s.Columns {
		if c.PrimaryKey {
			pk = uint16(i + 1)
		}
	}
	binary.LittleEndian.PutUint16(x[:2], pk)
	b = append(b, x[:2]...)
	binary.LittleEndian.PutUint16(x[:2], uint16(len(s.Columns)))
	b = append(b, x[:2]...)
	for i, c := range s.Columns {
		binary.LittleEndian.PutUint16(x[:2], uint16(i+1))
		b = append(b, x[:2]...)
		b = append(b, byte(c.Type), boolByte(c.Nullable))
		binary.LittleEndian.PutUint16(x[:2], uint16(len(c.Name)))
		b = append(b, x[:2]...)
		b = append(b, c.Name...)
	}
	return string(b)
}
func boolByte(v bool) byte {
	if v {
		return 1
	}
	return 0
}

// ErrUnsupportedRelationalFormat rejects relational data requiring migration.
var ErrUnsupportedRelationalFormat = errors.New("unsupported relational format: export with the original version and import into a new database")

func decodeDescriptor(v string) (TableSchema, error) {
	b := []byte(v)
	if len(b) < 6 || string(b[:4]) != "GDTB" {
		return TableSchema{}, ErrInvalidSchema
	}
	if binary.LittleEndian.Uint16(b[4:6]) != 2 {
		return TableSchema{}, fmt.Errorf("%w: %w", ErrInvalidSchema, ErrUnsupportedRelationalFormat)
	}
	if len(b) < 18 || len(b) > 3000 || b[6] != 0 || b[7] != 0 || binary.LittleEndian.Uint32(b[8:12]) != 1 {
		return TableSchema{}, ErrInvalidSchema
	}
	p := 6 + 2 + 4
	nl := int(binary.LittleEndian.Uint16(b[p : p+2]))
	p += 2
	if p+nl+4 > len(b) {
		return TableSchema{}, ErrInvalidSchema
	}
	s := TableSchema{Name: string(b[p : p+nl])}
	p += nl
	pk := binary.LittleEndian.Uint16(b[p : p+2])
	p += 2
	n := int(binary.LittleEndian.Uint16(b[p : p+2]))
	p += 2
	s.Columns = make([]Column, 0, n)
	for i := 0; i < n; i++ {
		if p+6 > len(b) {
			return TableSchema{}, ErrInvalidSchema
		}
		id, typ, flags := binary.LittleEndian.Uint16(b[p:p+2]), ColumnType(b[p+2]), b[p+3]
		l := int(binary.LittleEndian.Uint16(b[p+4 : p+6]))
		p += 6
		if int(id) != i+1 || flags > 1 || (id == pk && flags != 0) || p+l > len(b) {
			return TableSchema{}, ErrInvalidSchema
		}
		s.Columns = append(s.Columns, Column{Name: string(b[p : p+l]), Type: typ, PrimaryKey: id == pk, Nullable: flags&1 != 0 && id != pk})
		p += l
	}
	if p != len(b) {
		return TableSchema{}, ErrInvalidSchema
	}
	if err := validateSchema(s); err != nil {
		return TableSchema{}, err
	}
	return s, nil
}

func encodeRow(s TableSchema, row map[string]any) (string, string, error) {
	if len(s.Columns) > 65535 || validateSchema(s) != nil {
		return "", "", ErrInvalidSchema
	}
	for name := range row {
		if _, found := findColumn(s, name); !found {
			return "", "", fmt.Errorf("%w: unknown column %s", ErrInvalidRow, name)
		}
	}
	var pk any
	for _, c := range s.Columns {
		v, ok := row[c.Name]
		if !ok && !c.Nullable {
			return "", "", fmt.Errorf("%w: missing column %s", ErrInvalidRow, c.Name)
		}
		if c.PrimaryKey {
			pk = v
		}
	}
	enc, err := encodeValue(findPrimary(s), pk)
	if err != nil {
		return "", "", err
	}
	key := append(rowPrefix(s.Name), enc...)
	if len(key) > 1000 {
		return "", "", ErrInvalidRow
	}
	out := []byte("GDRW")
	var x [4]byte
	binary.LittleEndian.PutUint16(x[:2], 2)
	out = append(out, x[:2]...)
	binary.LittleEndian.PutUint32(x[:], 1)
	out = append(out, x[:]...)
	binary.LittleEndian.PutUint16(x[:2], uint16(len(s.Columns)-1))
	out = append(out, x[:2]...)
	for i, c := range s.Columns {
		if c.PrimaryKey {
			continue
		}
		ev, e := encodeValue(c, row[c.Name])
		if e != nil {
			return "", "", e
		}
		binary.LittleEndian.PutUint16(x[:2], uint16(i+1))
		out = append(out, x[:2]...)
		out = append(out, 0, 0, 0, 0, 0, 0)
		binary.LittleEndian.PutUint32(x[:], uint32(len(ev)))
		copy(out[len(out)-4:], x[:])
		out = append(out, ev...)
	}
	if len(out) > 3000 {
		return "", "", ErrInvalidRow
	}
	return string(key), string(out), nil
}

func decodeRow(s TableSchema, data []byte) (map[string]any, error) {
	if len(data) < 6 || string(data[:4]) != "GDRW" {
		return nil, ErrInvalidRow
	}
	if binary.LittleEndian.Uint16(data[4:6]) != 2 {
		return nil, fmt.Errorf("%w: %w", ErrInvalidRow, ErrUnsupportedRelationalFormat)
	}
	if len(data) < 12 || len(data) > 3000 || len(s.Columns) > 65535 || validateSchema(s) != nil {
		return nil, ErrInvalidRow
	}
	if binary.LittleEndian.Uint32(data[6:10]) != 1 {
		return nil, ErrInvalidRow
	}
	count := int(binary.LittleEndian.Uint16(data[10:12]))
	if count != len(s.Columns)-1 {
		return nil, ErrInvalidRow
	}
	row := make(map[string]any, len(s.Columns)-1)
	pos := 12
	lastID := uint16(0)
	for i := 0; i < count; i++ {
		if pos+8 > len(data) {
			return nil, ErrInvalidRow
		}
		id := binary.LittleEndian.Uint16(data[pos : pos+2])
		flags := data[pos+2]
		reserved := data[pos+3]
		length := int(binary.LittleEndian.Uint32(data[pos+4 : pos+8]))
		pos += 8
		if id <= lastID || flags != 0 || reserved != 0 || length < 0 || length > len(data)-pos {
			return nil, ErrInvalidRow
		}
		if id > uint16(len(s.Columns)) || s.Columns[id-1].PrimaryKey {
			return nil, ErrInvalidRow
		}
		column := s.Columns[id-1]
		value, err := decodeValue(column, data[pos:pos+length])
		if err != nil {
			return nil, err
		}
		row[column.Name] = value
		lastID = id
		pos += length
	}
	if pos != len(data) {
		return nil, ErrInvalidRow
	}
	return row, nil
}

func decodeValue(c Column, data []byte) (any, error) {
	if c.PrimaryKey {
		return decodePrimaryKey(c, data)
	}
	if c.Type < ColumnInt64 || c.Type > ColumnBool || len(data) == 0 {
		return nil, ErrInvalidRow
	}
	if data[0] == 0 {
		if len(data) != 1 || !c.Nullable || c.PrimaryKey {
			return nil, ErrInvalidRow
		}
		return nil, nil
	}
	if data[0] != byte(c.Type) {
		return nil, ErrInvalidRow
	}
	data = data[1:]
	switch c.Type {
	case ColumnInt64:
		if len(data) != 8 {
			return nil, ErrInvalidRow
		}
		return int64(binary.BigEndian.Uint64(data) ^ (1 << 63)), nil
	case ColumnString:
		value, ok := decodeEscapedComponent(data)
		if !ok {
			return nil, ErrInvalidRow
		}
		return value, nil
	case ColumnBytes:
		value, ok := decodeEscapedComponent(data)
		if !ok {
			return nil, ErrInvalidRow
		}
		return []byte(value), nil
	case ColumnBool:
		if len(data) != 1 || data[0] > 1 {
			return nil, ErrInvalidRow
		}
		return data[0] == 1, nil
	default:
		return nil, ErrInvalidRow
	}
}

func decodePrimaryKey(c Column, data []byte) (any, error) {
	if c.Type == ColumnInt64 && len(data) == 8 {
		return int64(binary.BigEndian.Uint64(data) ^ (1 << 63)), nil
	}
	if c.Type == ColumnString {
		return string(data), nil
	}
	return nil, ErrInvalidRow
}
func findPrimary(s TableSchema) Column {
	for _, c := range s.Columns {
		if c.PrimaryKey {
			return c
		}
	}
	return Column{}
}
func encodeValue(c Column, v any) ([]byte, error) {
	if c.Type < ColumnInt64 || c.Type > ColumnBool {
		return nil, ErrInvalidRow
	}
	if v == nil {
		if c.Nullable && !c.PrimaryKey {
			return []byte{0}, nil
		}
		return nil, fmt.Errorf("%w: null column %s", ErrInvalidRow, c.Name)
	}
	switch c.Type {
	case ColumnInt64:
		x, ok := v.(int64)
		if !ok {
			return nil, ErrInvalidRow
		}
		var b [8]byte
		binary.BigEndian.PutUint64(b[:], uint64(x)^(1<<63))
		if c.PrimaryKey {
			return b[:], nil
		}
		return append([]byte{byte(c.Type)}, b[:]...), nil
	case ColumnString:
		x, ok := v.(string)
		if !ok {
			return nil, ErrInvalidRow
		}
		if c.PrimaryKey {
			return []byte(x), nil
		}
		return append([]byte{byte(c.Type)}, escape([]byte(x))...), nil
	case ColumnBytes:
		x, ok := v.([]byte)
		if !ok {
			return nil, ErrInvalidRow
		}
		return append([]byte{byte(c.Type)}, escape(x)...), nil
	case ColumnBool:
		x, ok := v.(bool)
		if !ok {
			return nil, ErrInvalidRow
		}
		if x {
			return []byte{byte(c.Type), 1}, nil
		}
		return []byte{byte(c.Type), 0}, nil
	}
	return nil, ErrInvalidRow
}
