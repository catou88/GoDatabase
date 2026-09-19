package db

import (
	"encoding/binary"
	"fmt"

	"godatabase/internal/engine"
)

// Index describes a secondary index on one table column.
type Index struct {
	Name   string
	Column string
	Unique bool
}

// FindByIndex returns rows whose indexed column equals value, ordered by
// primary key. The table row remains the authoritative source of values.
func (t *Table) FindByIndex(indexName string, value any) ([]map[string]any, error) {
	t.db.mu.Lock()
	defer t.db.mu.Unlock()
	if t.db.closed {
		return nil, ErrClosed
	}
	if err := t.refreshIndexesLocked(); err != nil {
		return nil, err
	}
	index, ok := t.indexes[indexName]
	if !ok {
		return nil, ErrIndexNotFound
	}
	column := columnByName(t.schema, index.Column)
	encoded, err := encodeIndexValue(column, value)
	if err != nil {
		return nil, err
	}
	prefix := indexValuePrefix(t.schema.Name, index, encoded)
	entries, err := t.db.rangeLocked(string(prefix), string(prefixEnd(prefix)))
	if err != nil {
		return nil, err
	}
	rows := make([]map[string]any, 0, len(entries))
	entryPrefix := string(prefix)
	for _, entry := range entries {
		if len(entry.Key) < len(entryPrefix) || entry.Key[:len(entryPrefix)] != entryPrefix {
			continue
		}
		var primaryKey []byte
		if index.Unique {
			if len(entry.Key) != len(entryPrefix) {
				return nil, ErrInvalidRow
			}
			primaryKey = []byte(entry.Value)
		} else {
			primaryKey = []byte(entry.Key[len(entryPrefix):])
			if string(primaryKey) != entry.Value {
				return nil, ErrInvalidRow
			}
		}
		rowKey := append(rowPrefix(t.schema.Name), primaryKey...)
		rowValue, found, err := t.db.getLocked(string(rowKey))
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, ErrInvalidRow
		}
		row, err := decodeRow(t.schema, []byte(rowValue))
		if err != nil {
			return nil, err
		}
		actual, err := encodeIndexValue(column, row[index.Column])
		if err != nil || string(actual) != string(encoded) {
			return nil, ErrInvalidRow
		}
		row[t.primaryColumn().Name], err = decodePrimaryKey(t.primaryColumn(), primaryKey)
		if err != nil {
			return nil, err
		}
		rows = append(rows, row)
	}
	return rows, nil
}

var (
	ErrIndexExists      = fmt.Errorf("index already exists")
	ErrIndexNotFound    = fmt.Errorf("index not found")
	ErrDuplicateIndexed = fmt.Errorf("duplicate indexed value")
	ErrRowNotFound      = fmt.Errorf("row not found")
)

// CreateIndex creates and backfills an index on column. Existing rows are
// indexed before the operation returns successfully.
func (t *Table) CreateIndex(index Index) error {
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
	if err := validateIndex(index, t.schema); err != nil {
		return err
	}
	if _, exists := t.indexes[index.Name]; exists {
		return ErrIndexExists
	}
	metadataKey := indexMetadataKey(t.schema.Name, index.Name)
	if _, found, err := t.db.getLocked(metadataKey); err != nil {
		return err
	} else if found {
		return ErrIndexExists
	}

	rows, err := tableRowsLocked(t.db, t.schema.Name)
	if err != nil {
		return err
	}
	mutations := make([]engine.Mutation, 0, len(rows)+1)
	seen := make(map[string]string, len(rows))
	column := columnByName(t.schema, index.Column)
	for _, entry := range rows {
		row, err := decodeRow(t.schema, []byte(entry.Value))
		if err != nil {
			return err
		}
		indexedValue, err := encodeIndexValue(column, row[index.Column])
		if err != nil {
			return err
		}
		primaryKey := []byte(entry.Key[len(rowPrefix(t.schema.Name)):])
		key := indexEntryKey(t.schema.Name, index, indexedValue, primaryKey)
		if index.Unique {
			if previous, exists := seen[string(indexedValue)]; exists && previous != string(primaryKey) {
				return ErrDuplicateIndexed
			}
			seen[string(indexedValue)] = string(primaryKey)
		}
		mutations = append(mutations, engine.Mutation{Key: []byte(key), Value: primaryKey})
	}
	mutations = append(mutations, engine.Mutation{Key: []byte(metadataKey), Value: []byte(encodeIndexMetadata(index))})
	if err := t.db.applyBatchLocked(mutations); err != nil {
		return err
	}
	if t.indexes == nil {
		t.indexes = make(map[string]Index)
	}
	t.indexes[index.Name] = index
	return nil
}

// Indexes returns the table's secondary-index definitions.
func (t *Table) Indexes() []Index {
	t.db.mu.Lock()
	defer t.db.mu.Unlock()
	if t.db.closed || t.refreshIndexesLocked() != nil {
		return nil
	}
	indexes := make([]Index, 0, len(t.indexes))
	for _, index := range t.indexes {
		indexes = append(indexes, index)
	}
	return indexes
}

func validateIndex(index Index, schema TableSchema) error {
	if index.Name == "" || len(index.Name) > 128 || index.Column == "" {
		return ErrInvalidSchema
	}
	column, ok := findColumn(schema, index.Column)
	if !ok || column.PrimaryKey {
		return ErrInvalidSchema
	}
	return nil
}

func findColumn(schema TableSchema, name string) (Column, bool) {
	for _, column := range schema.Columns {
		if column.Name == name {
			return column, true
		}
	}
	return Column{}, false
}

func columnByName(schema TableSchema, name string) Column {
	column, _ := findColumn(schema, name)
	return column
}

func indexMetadataKey(tableName, indexName string) string {
	key := []byte{0x11}
	key = append(key, escape([]byte(tableName))...)
	key = append(key, escape([]byte(indexName))...)
	return string(key)
}

func indexEntryPrefix(tableName, indexName string) []byte {
	key := []byte{0x30}
	key = append(key, escape([]byte(tableName))...)
	key = append(key, escape([]byte(indexName))...)
	return key
}

// encodeIndexValue always encodes a tagged, self-delimiting component.
func encodeIndexValue(column Column, value any) ([]byte, error) {
	column.PrimaryKey = false
	return encodeValue(column, value)
}

func indexValuePrefix(tableName string, index Index, encoded []byte) []byte {
	key := indexEntryPrefix(tableName, index.Name)
	return append(key, encoded...)
}

func indexEntryKey(tableName string, index Index, indexedValue, primaryKey []byte) string {
	key := indexValuePrefix(tableName, index, indexedValue)
	if !index.Unique {
		key = append(key, primaryKey...)
	}
	return string(key)
}

func encodeIndexMetadata(index Index) string {
	data := make([]byte, 0, 12+len(index.Column))
	data = append(data, "GDXI"...)
	var buf [4]byte
	binary.LittleEndian.PutUint16(buf[:2], 2)
	data = append(data, buf[:2]...)
	if index.Unique {
		data = append(data, 1)
	} else {
		data = append(data, 0)
	}
	data = append(data, 0)
	binary.LittleEndian.PutUint16(buf[:2], uint16(len(index.Column)))
	data = append(data, buf[:2]...)
	data = append(data, index.Column...)
	return string(data)
}

func decodeIndexMetadata(indexName string, data []byte) (Index, error) {
	if len(data) < 6 || string(data[:4]) != "GDXI" {
		return Index{}, ErrInvalidSchema
	}
	if binary.LittleEndian.Uint16(data[4:6]) != 2 {
		return Index{}, fmt.Errorf("%w: %w", ErrInvalidSchema, ErrUnsupportedRelationalFormat)
	}
	if len(data) < 10 || data[7] != 0 || len(indexName) == 0 || len(indexName) > 128 {
		return Index{}, ErrInvalidSchema
	}
	nameLength := int(binary.LittleEndian.Uint16(data[8:10]))
	if 10+nameLength != len(data) || data[6]&^byte(1) != 0 || nameLength == 0 || nameLength > 128 {
		return Index{}, ErrInvalidSchema
	}
	return Index{Name: indexName, Column: string(data[10:]), Unique: data[6]&1 != 0}, nil
}

func loadIndexesLocked(d *Database, schema TableSchema) (map[string]Index, error) {
	indexes := make(map[string]Index)
	start := string([]byte{0x11}) + string(escape([]byte(schema.Name)))
	end := prefixEnd([]byte(start))
	entries, err := d.rangeLocked(start, string(end))
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		key := []byte(entry.Key)
		if len(key) <= len(start) || string(key[:len(start)]) != start {
			continue
		}
		name, ok := decodeEscapedComponent(key[len(start):])
		if !ok {
			return nil, ErrInvalidSchema
		}
		index, err := decodeIndexMetadata(name, []byte(entry.Value))
		if err != nil {
			return nil, err
		}
		if err := validateIndex(index, schema); err != nil {
			return nil, err
		}
		indexes[name] = index
	}
	return indexes, nil
}

func tableRowsLocked(d *Database, tableName string) ([]Item, error) {
	prefix := rowPrefix(tableName)
	end := prefixEnd(prefix)
	entries, err := d.rangeLocked(string(prefix), string(end))
	if err != nil {
		return nil, err
	}
	rows := make([]Item, 0, len(entries))
	for _, entry := range entries {
		if hasPrefix([]byte(entry.Key), prefix) {
			rows = append(rows, entry)
		}
	}
	return rows, nil
}

func prefixEnd(prefix []byte) []byte {
	end := append([]byte(nil), prefix...)
	for i := len(end) - 1; i >= 0; i-- {
		if end[i] != 0xff {
			end[i]++
			return end[:i+1]
		}
	}
	return append(end, 0)
}

func decodeEscapedComponent(data []byte) (string, bool) {
	if len(data) < 2 || data[len(data)-2] != 0 || data[len(data)-1] != 0 {
		return "", false
	}
	decoded := make([]byte, 0, len(data)-2)
	for i := 0; i < len(data)-2; i++ {
		if data[i] != 0 {
			decoded = append(decoded, data[i])
			continue
		}
		if i+1 >= len(data)-2 || data[i+1] != 0xff {
			return "", false
		}
		decoded = append(decoded, 0)
		i++
	}
	return string(decoded), true
}
