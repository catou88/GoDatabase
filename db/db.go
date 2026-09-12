package db

import (
	"errors"
	"sort"
)

// ErrEmptyKey is returned when Set receives an empty key.
var ErrEmptyKey = errors.New("key cannot be empty")

// Item is a key-value pair returned by range queries.
type Item struct {
	Key   string
	Value string
}

// Database is an in-memory key-value store.
//
// Database is not safe for concurrent use.
type Database struct {
	data map[string]string
}

// New creates a new in-memory database.
func New() *Database {
	return &Database{data: make(map[string]string)}
}

// Set stores value for key.
//
// Set overwrites any existing value for key. It returns ErrEmptyKey when key is
// empty. Empty values are allowed.
func (d *Database) Set(key, value string) error {
	if key == "" {
		return ErrEmptyKey
	}
	d.data[key] = value
	return nil
}

// Get returns the value for key and whether key exists.
//
// When key does not exist, Get returns "", false.
func (d *Database) Get(key string) (string, bool) {
	value, ok := d.data[key]
	return value, ok
}

// Delete removes key from the database.
//
// Delete returns true when key existed and was removed. It returns false when
// key does not exist.
func (d *Database) Delete(key string) bool {
	if _, exists := d.data[key]; !exists {
		return false
	}
	delete(d.data, key)
	return true
}

// Range returns key-value pairs with keys between start and end, inclusive.
//
// Results are sorted by key in ascending order. Range returns an empty slice
// when no keys match or when start is greater than end. Empty bounds are
// compared as ordinary strings; they are not unbounded range markers.
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
