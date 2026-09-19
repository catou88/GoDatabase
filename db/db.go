package db

import (
	"errors"
	"sort"
	"sync"

	"godatabase/internal/engine"
)

var (
	// ErrEmptyKey is returned when Set receives an empty key.
	ErrEmptyKey = errors.New("key cannot be empty")
	// ErrClosed is returned when an operation uses a closed database.
	ErrClosed = errors.New("database is closed")
	// ErrTransactionClosed is returned after a transaction has finished.
	ErrTransactionClosed = errors.New("transaction is closed")
	// ErrReadOnlyTransaction is returned when a read-only transaction writes.
	ErrReadOnlyTransaction = errors.New("transaction is read-only")
	// ErrWriteTransactionActive is returned when a second writer starts.
	ErrWriteTransactionActive = errors.New("write transaction already active")
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
	mu           sync.RWMutex
	data         map[string]string
	durable      engine.Store
	closed       bool
	writerActive bool
}

// TxOptions controls transaction behavior.
type TxOptions struct{ ReadOnly bool }

// Tx groups database operations into one commit or rollback.
type Tx struct {
	db       *Database
	readOnly bool
	closed   bool
	order    []string
	changes  map[string]transactionChange
}

type transactionChange struct {
	value  string
	delete bool
}

// Begin starts a transaction. Read-write transactions are serialized per
// database handle; read-only transactions may coexist with a writer.
func (d *Database) Begin(options TxOptions) (*Tx, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.closed {
		return nil, ErrClosed
	}
	if !options.ReadOnly && d.writerActive {
		return nil, ErrWriteTransactionActive
	}
	if !options.ReadOnly {
		d.writerActive = true
	}
	return &Tx{db: d, readOnly: options.ReadOnly, changes: make(map[string]transactionChange)}, nil
}

// Get reads from the transaction, including its buffered writes.
func (tx *Tx) Get(key string) (string, bool, error) {
	tx.db.mu.RLock()
	defer tx.db.mu.RUnlock()
	if tx.closed {
		return "", false, ErrTransactionClosed
	}
	if tx.db.closed {
		return "", false, ErrClosed
	}
	if change, ok := tx.changes[key]; ok {
		if change.delete {
			return "", false, nil
		}
		return change.value, true, nil
	}
	return tx.db.getLocked(key)
}

// Set buffers a value until Commit.
func (tx *Tx) Set(key, value string) error {
	return tx.buffer(key, transactionChange{value: value})
}

// Delete buffers a deletion until Commit.
func (tx *Tx) Delete(key string) (bool, error) {
	tx.db.mu.Lock()
	defer tx.db.mu.Unlock()
	if tx.closed {
		return false, ErrTransactionClosed
	}
	if tx.db.closed {
		return false, ErrClosed
	}
	if tx.readOnly {
		return false, ErrReadOnlyTransaction
	}
	if key == "" {
		return false, nil
	}
	_, found, err := tx.valueLocked(key)
	if err != nil {
		return false, err
	}
	tx.recordLocked(key, transactionChange{delete: true})
	return found, nil
}

// Range reads a transaction view, including buffered changes.
func (tx *Tx) Range(start, end string) ([]Item, error) {
	tx.db.mu.RLock()
	defer tx.db.mu.RUnlock()
	if tx.closed {
		return nil, ErrTransactionClosed
	}
	if tx.db.closed {
		return nil, ErrClosed
	}
	if start > end {
		return []Item{}, nil
	}
	items, err := tx.db.rangeLocked(start, end)
	if err != nil {
		return nil, err
	}
	values := make(map[string]string, len(items)+len(tx.changes))
	for _, item := range items {
		values[item.Key] = item.Value
	}
	for key, change := range tx.changes {
		if key < start || key > end {
			continue
		}
		if change.delete {
			delete(values, key)
		} else {
			values[key] = change.value
		}
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]Item, 0, len(keys))
	for _, key := range keys {
		result = append(result, Item{Key: key, Value: values[key]})
	}
	return result, nil
}

// Commit atomically publishes buffered changes.
func (tx *Tx) Commit() error {
	tx.db.mu.Lock()
	defer tx.db.mu.Unlock()
	return tx.commitLocked()
}

func (tx *Tx) commitLocked() error {
	if tx.closed {
		return ErrTransactionClosed
	}
	if tx.db.closed {
		return ErrClosed
	}
	if tx.readOnly {
		tx.closed = true
		return ErrReadOnlyTransaction
	}
	mutations := make([]engine.Mutation, 0, len(tx.order))
	for _, key := range tx.order {
		change := tx.changes[key]
		mutations = append(mutations, engine.Mutation{Key: []byte(key), Value: []byte(change.value), Delete: change.delete})
	}
	if err := tx.db.applyBatchLocked(mutations); err != nil {
		return err
	}
	tx.closed = true
	tx.db.writerActive = false
	return nil
}

// Rollback discards buffered changes. It is safe to call more than once.
func (tx *Tx) Rollback() error {
	tx.db.mu.Lock()
	defer tx.db.mu.Unlock()
	if tx.closed {
		return nil
	}
	tx.closed = true
	if !tx.readOnly {
		tx.db.writerActive = false
	}
	return nil
}

func (tx *Tx) buffer(key string, value transactionChange) error {
	tx.db.mu.Lock()
	defer tx.db.mu.Unlock()
	if tx.closed {
		return ErrTransactionClosed
	}
	if tx.db.closed {
		return ErrClosed
	}
	if tx.readOnly {
		return ErrReadOnlyTransaction
	}
	if key == "" {
		return ErrEmptyKey
	}
	tx.recordLocked(key, value)
	return nil
}

func (tx *Tx) recordLocked(key string, change transactionChange) {
	if _, exists := tx.changes[key]; !exists {
		tx.order = append(tx.order, key)
	}
	tx.changes[key] = change
}

func (tx *Tx) valueLocked(key string) (string, bool, error) {
	if change, ok := tx.changes[key]; ok {
		return change.value, !change.delete, nil
	}
	return tx.db.getLocked(key)
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
	durable, err := engine.Open(path)
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
	if d.writerActive {
		return ErrWriteTransactionActive
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
	if d.writerActive {
		return false, ErrWriteTransactionActive
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

// applyBatchLocked publishes a complete mutation set while the caller holds mu.
func (d *Database) applyBatchLocked(mutations []engine.Mutation) error {
	if d.closed {
		return ErrClosed
	}
	if d.durable != nil {
		return translateError(d.durable.ApplyBatch(mutations))
	}
	if d.data == nil {
		return errors.New("database is not initialized")
	}
	for _, mutation := range mutations {
		if len(mutation.Key) == 0 || len(mutation.Key) > 1000 || (!mutation.Delete && len(mutation.Value) > 3000) {
			return ErrInvalidRow
		}
	}
	for _, mutation := range mutations {
		if mutation.Delete {
			delete(d.data, string(mutation.Key))
		} else {
			d.data[string(mutation.Key)] = string(mutation.Value)
		}
	}
	return nil
}

func translateError(err error) error {
	if errors.Is(err, engine.ErrClosed) {
		return ErrClosed
	}
	return err
}
