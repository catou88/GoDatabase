package btree

import "godatabase/internal/structures"

// Memory adapts the in-memory B+Tree to the byte-oriented structure contract.
// Its zero value is ready to use. Callers must serialize concurrent access.
type Memory struct {
	tree tree
}

var _ structures.KV = (*Memory)(nil)

// NewMemory returns an empty in-memory B+Tree.
func NewMemory() *Memory { return &Memory{} }

// Get returns a copy of the value and whether key exists.
func (m *Memory) Get(key []byte) ([]byte, bool, error) {
	value, found := m.tree.get(string(key))
	if !found {
		return nil, false, nil
	}
	return []byte(value), true, nil
}

// Set stores independent copies of key and value.
func (m *Memory) Set(key, value []byte) error {
	m.tree.set(string(key), string(value))
	return nil
}

// Delete removes key and reports whether it existed.
func (m *Memory) Delete(key []byte) (bool, error) {
	return m.tree.delete(string(key)), nil
}

// Range returns independent copies in ascending byte order, with inclusive
// bounds. Empty bounds are ordinary keys; reversed bounds return no entries.
func (m *Memory) Range(start, end []byte) ([]structures.Entry, error) {
	lower, upper := string(start), string(end)
	var entries []structures.Entry
	n := m.tree.root
	if n == nil || lower > upper {
		return entries, nil
	}
	for !n.leaf {
		n = n.child(lower)
	}
	idx, _ := n.search(lower)
	for ; n != nil; n = n.next {
		for ; idx < len(n.keys); idx++ {
			if n.keys[idx] > upper {
				return entries, nil
			}
			entries = append(entries, structures.Entry{
				Key: []byte(n.keys[idx]), Value: []byte(n.values[idx]),
			})
		}
		idx = 0
	}
	return entries, nil
}

// Close is a no-op; Memory owns no external resources and remains usable.
func (m *Memory) Close() error { return nil }
