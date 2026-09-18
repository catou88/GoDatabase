package db

import (
	"encoding/binary"
	"fmt"
)

// Index describes a secondary index on one table column.
type Index struct {
	Name   string
	Column string
	Unique bool
}

var (
	ErrIndexExists      = fmt.Errorf("index already exists")
	ErrIndexNotFound    = fmt.Errorf("index not found")
	ErrDuplicateIndexed = fmt.Errorf("duplicate indexed value")
)

// CreateIndex creates and backfills an index on column. Existing rows are
// indexed before the operation returns successfully.
func (t *Table) CreateIndex(index Index) error {
	t.db.mu.Lock()
	defer t.db.mu.Unlock()
	if t.db.closed {
		return ErrClosed
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
	entries := make([]struct{ key, value string }, 0, len(rows))
	seen := make(map[string]string, len(rows))
	column := columnByName(t.schema, index.Column)
	for _, entry := range rows {
		row, err := decodeRow(t.schema, []byte(entry.Value))
		if err != nil {
			return err
		}
		indexedValue, err := encodeValue(column, row[index.Column])
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
		entries = append(entries, struct{ key, value string }{key: key, value: string(primaryKey)})
	}
	if err := t.db.setLocked(metadataKey, encodeIndexMetadata(index)); err != nil {
		return err
	}
	for _, entry := range entries {
		if err := t.db.setLocked(entry.key, entry.value); err != nil {
			return err
		}
	}
	if t.indexes == nil {
		t.indexes = make(map[string]Index)
	}
	t.indexes[index.Name] = index
	return nil
}

// Indexes returns the table's secondary-index definitions.
func (t *Table) Indexes() []Index {
	t.db.mu.RLock()
	defer t.db.mu.RUnlock()
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

func indexEntryKey(tableName string, index Index, indexedValue, primaryKey []byte) string {
	key := indexEntryPrefix(tableName, index.Name)
	key = append(key, indexedValue...)
	if !index.Unique {
		key = append(key, primaryKey...)
	}
	return string(key)
}

func encodeIndexMetadata(index Index) string {
	data := make([]byte, 0, 12+len(index.Column))
	data = append(data, "GDXI"...)
	var buf [4]byte
	binary.LittleEndian.PutUint16(buf[:2], 1)
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
	if len(data) < 10 || string(data[:4]) != "GDXI" || binary.LittleEndian.Uint16(data[4:6]) != 1 || data[7] != 0 {
		return Index{}, ErrInvalidSchema
	}
	nameLength := int(binary.LittleEndian.Uint16(data[8:10]))
	if 10+nameLength != len(data) || data[6]&^byte(1) != 0 || nameLength == 0 {
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
