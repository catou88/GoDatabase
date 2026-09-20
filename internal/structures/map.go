package structures

import "sort"

type Map struct{ values map[string]string }

var _ KV = (*Map)(nil)

func NewMap() *Map                   { return &Map{values: make(map[string]string)} }
func (m *Map) Set(k, v []byte) error { m.values[string(k)] = string(v); return nil }
func (m *Map) Get(k []byte) ([]byte, bool, error) {
	v, ok := m.values[string(k)]
	return []byte(v), ok, nil
}
func (m *Map) Delete(k []byte) (bool, error) {
	_, ok := m.values[string(k)]
	delete(m.values, string(k))
	return ok, nil
}
func (m *Map) Range(start, end []byte) ([]Entry, error) {
	keys := make([]string, 0)
	for k := range m.values {
		if k >= string(start) && k <= string(end) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	result := make([]Entry, 0, len(keys))
	for _, k := range keys {
		result = append(result, Entry{Key: []byte(k), Value: []byte(m.values[k])})
	}
	return result, nil
}
