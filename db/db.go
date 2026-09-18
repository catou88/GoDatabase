package db

import (
	"errors"
	"sort"
	"sync"

	"godatabase/internal/btree"
)

var (
	// ErrEmptyKey is returned when Set receives an empty key.
	ErrEmptyKey = errors.New("key cannot be empty")
	// ErrClosed is returned when an operation uses a closed database.
	ErrClosed = errors.New("database is closed")
)

// Item is a key-value pair returned by range queries.
type Item struct {
	Key   string
	Value string
}

// Database is a string-based key-value store.
//
// A Database returned by Open owns its underlying file and must be closed when
// it is no longer needed. Close is idempotent. Operations after Close return
// ErrClosed. A Database is safe for concurrent use by multiple goroutines, but
// multiple Database handles must not access the same durable file concurrently.
type Database struct {
	mu      sync.RWMutex
	data    map[string]string
	durable *btree.KV
	closed  bool
}

// New creates an in-memory database.
//
// New is useful for tests and temporary data. Use Open when data must survive
// process restarts.
func New() *Database {
	return &Database{data: make(map[string]string)}
}

// Open opens or creates a durable database at path.
//
// The caller owns the returned Database and should call Close when finished.
func Open(path string) (*Database, error) {
	durable, err := btree.Open(path)
	if err != nil {
		return nil, err
	}
	return &Database{durable: durable}, nil
}

// Close releases resources owned by the database. It is safe to call Close
// more than once.
func (d *Database) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil
	}
	d.closed = true
	if d.durable != nil {
		return d.durable.Close()
	}
	return nil
}

// Set stores value for key.
//
// Set overwrites any existing value for key. It returns ErrEmptyKey when key is
// empty. Empty values are allowed.
func (d *Database) Set(key, value string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return ErrClosed
	}
	if key == "" {
		return ErrEmptyKey
	}
	if d.durable != nil {
		return translateError(d.durable.Set([]byte(key), []byte(value)))
	}
	d.data[key] = value
	return nil
}

// Get returns the value for key and whether key exists.
//
// When key does not exist, Get returns "", false, nil.
func (d *Database) Get(key string) (string, bool, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return "", false, ErrClosed
	}
	if key == "" {
		return "", false, nil
	}
	if d.durable != nil {
		value, found, err := d.durable.Get([]byte(key))
		return string(value), found, translateError(err)
	}
	value, ok := d.data[key]
	return value, ok, nil
}

// Delete removes key from the database.
//
// Delete returns true when key existed and was removed. It returns false when
// key does not exist.
func (d *Database) Delete(key string) (bool, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return false, ErrClosed
	}
	if key == "" {
		return false, nil
	}
	if d.durable != nil {
		deleted, err := d.durable.Delete([]byte(key))
		return deleted, translateError(err)
	}
	if _, exists := d.data[key]; !exists {
		return false, nil
	}
	delete(d.data, key)
	return true, nil
}

// Range returns key-value pairs with keys between start and end, inclusive.
//
// Results are sorted by key in ascending order. Range returns an empty slice
// when no keys match or when start is greater than end. Empty bounds are
// compared as ordinary strings; they are not unbounded range markers.
func (d *Database) Range(start, end string) ([]Item, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.closed {
		return nil, ErrClosed
	}
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
	return items, nil
}

func translateError(err error) error {
	if errors.Is(err, btree.ErrClosed) {
		return ErrClosed
	}
	return err
}
