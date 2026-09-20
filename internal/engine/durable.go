package engine

import (
	"godatabase/internal/btree"
	"godatabase/internal/trace"
)

// Durable coordinates the durable key-value lifecycle. The current
// implementation delegates page commit and recovery to the existing B+Tree
// coordinator; this is the migration boundary for extracting that coordination
// without changing the file format or commit ordering.
type Durable struct{ store Store }

var _ Store = (*Durable)(nil)
var _ CommitCoordinator = (*Durable)(nil)

// New constructs a durable engine around an injected store. The constructor
// does not take ownership of the store's lifecycle beyond Close delegation.
func New(store Store) *Durable {
	return &Durable{store: store}
}

// Open opens or creates a durable engine at path.
func Open(path string) (*Durable, error) {
	store, err := btree.Open(path)
	if err != nil {
		return nil, err
	}
	return New(&btreeStore{store: store}), nil
}

func (d *Durable) Get(key []byte) ([]byte, bool, error) { return d.store.Get(key) }

func (d *Durable) Range(start, end []byte) ([]Entry, error) {
	return d.store.Range(start, end)
}

func (d *Durable) Set(key, value []byte) error { return d.store.Set(key, value) }

func (d *Durable) Delete(key []byte) (bool, error) { return d.store.Delete(key) }

func (d *Durable) ApplyBatch(mutations []Mutation) error {
	return d.store.ApplyBatch(mutations)
}

func (d *Durable) Close() error { return d.store.Close() }

// SetTrace forwards the observer when the injected store supports tracing.
func (d *Durable) SetTrace(sink trace.Sink) {
	if store, ok := d.store.(interface{ SetTrace(trace.Sink) }); ok {
		store.SetTrace(sink)
	}
}

type btreeStore struct{ store *btree.KV }

func (s *btreeStore) SetTrace(sink trace.Sink) { s.store.SetTrace(sink) }

func (s *btreeStore) Get(key []byte) ([]byte, bool, error) { return s.store.Get(key) }

func (s *btreeStore) Range(start, end []byte) ([]Entry, error) {
	entries, err := s.store.Range(start, end)
	if err != nil {
		return nil, err
	}
	result := make([]Entry, len(entries))
	for i, entry := range entries {
		result[i] = Entry(entry)
	}
	return result, nil
}

func (s *btreeStore) Set(key, value []byte) error { return s.store.Set(key, value) }

func (s *btreeStore) Delete(key []byte) (bool, error) { return s.store.Delete(key) }

func (s *btreeStore) ApplyBatch(mutations []Mutation) error {
	changes := make([]btree.Mutation, len(mutations))
	for i, mutation := range mutations {
		changes[i] = btree.Mutation{Key: mutation.Key, Value: mutation.Value, Delete: mutation.Delete}
	}
	return s.store.ApplyBatch(changes)
}

func (s *btreeStore) Close() error { return s.store.Close() }
