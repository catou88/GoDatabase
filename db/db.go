package db

import "sort"

// Item represents a key-value record stored in the in-memory database.
type Item struct {
	Key   string
	Value string
}

// Database is a minimal in-memory key-value store.
type Database struct {
	data map[string]string
}

// New creates a new in-memory database.
func New() *Database {
	return &Database{data: make(map[string]string)}
}

// Set stores a value for a key.
func (d *Database) Set(key, value string) error {
	d.data[key] = value
	return nil
}

// Get returns a value and whether it exists.
func (d *Database) Get(key string) (string, bool) {
	value, ok := d.data[key]
	return value, ok
}

// Delete removes a key from the database.
func (d *Database) Delete(key string) bool {
	if _, exists := d.data[key]; !exists {
		return false
	}
	delete(d.data, key)
	return true
}

// Range returns key-value pairs between start and end, inclusive.
func (d *Database) Range(start, end string) []Item {
	keys := make([]string, 0, len(d.data))
	for key := range d.data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	items := make([]Item, 0)
	for _, key := range keys {
		if key < start || key > end {
			continue
		}
		items = append(items, Item{Key: key, Value: d.data[key]})
	}

	return items
}
