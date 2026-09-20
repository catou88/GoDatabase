package structures

import (
	"bytes"
	"sort"
)

type SortedSlice struct{ entries []Entry }

var _ KV = (*SortedSlice)(nil)

func NewSortedSlice() *SortedSlice { return &SortedSlice{} }
func (s *SortedSlice) position(k []byte) int {
	return sort.Search(len(s.entries), func(i int) bool { return bytes.Compare(s.entries[i].Key, k) >= 0 })
}
func (s *SortedSlice) Get(k []byte) ([]byte, bool, error) {
	i := s.position(k)
	if i == len(s.entries) || !bytes.Equal(s.entries[i].Key, k) {
		return nil, false, nil
	}
	return bytes.Clone(s.entries[i].Value), true, nil
}
func (s *SortedSlice) Set(k, v []byte) error {
	i := s.position(k)
	if i < len(s.entries) && bytes.Equal(s.entries[i].Key, k) {
		s.entries[i].Value = bytes.Clone(v)
		return nil
	}
	s.entries = append(s.entries, Entry{})
	copy(s.entries[i+1:], s.entries[i:])
	s.entries[i] = Entry{Key: bytes.Clone(k), Value: bytes.Clone(v)}
	return nil
}
func (s *SortedSlice) Delete(k []byte) (bool, error) {
	i := s.position(k)
	if i == len(s.entries) || !bytes.Equal(s.entries[i].Key, k) {
		return false, nil
	}
	copy(s.entries[i:], s.entries[i+1:])
	s.entries[len(s.entries)-1] = Entry{}
	s.entries = s.entries[:len(s.entries)-1]
	return true, nil
}
func (s *SortedSlice) Range(start, end []byte) ([]Entry, error) {
	result := make([]Entry, 0)
	for i := s.position(start); i < len(s.entries) && bytes.Compare(s.entries[i].Key, end) <= 0; i++ {
		result = append(result, Entry{Key: bytes.Clone(s.entries[i].Key), Value: bytes.Clone(s.entries[i].Value)})
	}
	return result, nil
}
