package structures

import (
	"bytes"
	"container/list"
	"errors"
	"time"
)

type CacheStats struct {
	Hits         int   `json:"hits"`
	Misses       int   `json:"misses"`
	Evictions    int   `json:"evictions"`
	BackingCalls int   `json:"backing_calls"`
	BackingNS    int64 `json:"backing_ns"`
}
type cacheEntry struct {
	key   string
	value []byte
}

// Cache is a per-experiment LRU with write-through invalidation.
type Cache struct {
	backing  KV
	capacity int
	order    *list.List
	entries  map[string]*list.Element
	stats    CacheStats
}

var _ KV = (*Cache)(nil)

func NewCache(backing KV, capacity int) (*Cache, error) {
	if backing == nil || capacity < 1 || capacity > 256 {
		return nil, errors.New("invalid cache configuration")
	}
	return &Cache{backing: backing, capacity: capacity, order: list.New(), entries: make(map[string]*list.Element)}, nil
}
func (c *Cache) Stats() CacheStats { return c.stats }
func (c *Cache) invalidate(key string) {
	if e := c.entries[key]; e != nil {
		c.order.Remove(e)
		delete(c.entries, key)
	}
}
func (c *Cache) record(start time.Time) {
	c.stats.BackingCalls++
	c.stats.BackingNS += time.Since(start).Nanoseconds()
}
func (c *Cache) Get(key []byte) ([]byte, bool, error) {
	if e := c.entries[string(key)]; e != nil {
		c.stats.Hits++
		c.order.MoveToFront(e)
		return bytes.Clone(e.Value.(cacheEntry).value), true, nil
	}
	c.stats.Misses++
	start := time.Now()
	value, found, err := c.backing.Get(key)
	c.record(start)
	if err != nil || !found {
		return value, found, err
	}
	if len(c.entries) == c.capacity {
		last := c.order.Back()
		delete(c.entries, last.Value.(cacheEntry).key)
		c.order.Remove(last)
		c.stats.Evictions++
	}
	c.entries[string(key)] = c.order.PushFront(cacheEntry{key: string(key), value: bytes.Clone(value)})
	return value, true, nil
}
func (c *Cache) Set(key, value []byte) error {
	start := time.Now()
	err := c.backing.Set(key, value)
	c.record(start)
	if err == nil {
		c.invalidate(string(key))
	}
	return err
}
func (c *Cache) Delete(key []byte) (bool, error) {
	start := time.Now()
	found, err := c.backing.Delete(key)
	c.record(start)
	if err == nil {
		c.invalidate(string(key))
	}
	return found, err
}
func (c *Cache) Range(start, end []byte) ([]Entry, error) {
	at := time.Now()
	entries, err := c.backing.Range(start, end)
	c.record(at)
	return entries, err
}
