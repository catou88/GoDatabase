package structures

import (
	"errors"
	"testing"
)

func TestCacheHitsInvalidationAndEviction(t *testing.T) {
	m := NewMap()
	for _, key := range []string{"a", "b"} {
		if err := m.Set([]byte(key), []byte(key)); err != nil {
			t.Fatal(err)
		}
	}
	c, err := NewCache(m, 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"a", "a", "b"} {
		if _, found, err := c.Get([]byte(key)); err != nil || !found {
			t.Fatal("lookup failed")
		}
	}
	if s := c.Stats(); s.Hits != 1 || s.Misses != 2 || s.Evictions != 1 {
		t.Fatalf("%+v", s)
	}
	if err := c.Set([]byte("b"), []byte("new")); err != nil {
		t.Fatal(err)
	}
	got, _, err := c.Get([]byte("b"))
	if err != nil || string(got) != "new" {
		t.Fatal("stale update")
	}
	got[0] = 'X'
	again, _, _ := c.Get([]byte("b"))
	if string(again) != "new" {
		t.Fatal("aliased value")
	}
	if _, err := c.Delete([]byte("b")); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := c.Get([]byte("b")); err != nil || ok {
		t.Fatal("stale delete")
	}
}

type failingWrites struct{ KV }

func (f failingWrites) Set([]byte, []byte) error    { return errors.New("write failed") }
func (f failingWrites) Delete([]byte) (bool, error) { return false, errors.New("delete failed") }
func TestFailedCacheMutationPreservesValue(t *testing.T) {
	m := NewMap()
	if err := m.Set([]byte("a"), []byte("old")); err != nil {
		t.Fatal(err)
	}
	c, err := NewCache(failingWrites{m}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := c.Get([]byte("a")); err != nil {
		t.Fatal(err)
	}
	if err := c.Set([]byte("a"), []byte("new")); err == nil {
		t.Fatal("expected failure")
	}
	if _, err := c.Delete([]byte("a")); err == nil {
		t.Fatal("expected failure")
	}
	v, ok, err := c.Get([]byte("a"))
	if err != nil || !ok || string(v) != "old" {
		t.Fatal("failed write changed cache")
	}
}
