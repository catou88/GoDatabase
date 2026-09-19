package engine

import "godatabase/internal/btree"

// Durable coordinates the durable key-value lifecycle. The current
// implementation delegates page commit and recovery to the existing B+Tree
// coordinator; this is the migration boundary for extracting that coordination
// without changing the file format or commit ordering.
type Durable struct{ store *btree.KV }

// Open opens or creates a durable engine at path.
func Open(path string) (*Durable, error) {
	store, err := btree.Open(path)
	if err != nil {
		return nil, err
	}
	return &Durable{store: store}, nil
}

func (d *Durable) Get(key []byte) ([]byte, bool, error) { return d.store.Get(key) }

func (d *Durable) Range(start, end []byte) ([]Entry, error) {
	entries, err := d.store.Range(start, end)
	if err != nil {
		return nil, err
	}
	result := make([]Entry, len(entries))
	for i, entry := range entries {
		result[i] = Entry(entry)
	}
	return result, nil
}

func (d *Durable) Set(key, value []byte) error { return d.store.Set(key, value) }

func (d *Durable) Delete(key []byte) (bool, error) { return d.store.Delete(key) }

func (d *Durable) ApplyBatch(mutations []Mutation) error {
	changes := make([]btree.Mutation, len(mutations))
	for i, mutation := range mutations {
		changes[i] = btree.Mutation{Key: mutation.Key, Value: mutation.Value, Delete: mutation.Delete}
	}
	return d.store.ApplyBatch(changes)
}

func (d *Durable) Close() error { return d.store.Close() }
