package btree_test

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"

	"godatabase/internal/btree"
)

func TestMemoryMatchesModel(t *testing.T) {
	m := btree.NewMemory()
	model := map[string]string{}
	rng := rand.New(rand.NewSource(188))
	checkRange := func(start, end string) {
		t.Helper()
		got, err := m.Range([]byte(start), []byte(end))
		if err != nil {
			t.Fatal(err)
		}
		var keys []string
		for key := range model {
			if key >= start && key <= end {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		if len(got) != len(keys) {
			t.Fatalf("Range(%q, %q) length = %d, want %d", start, end, len(got), len(keys))
		}
		for i, key := range keys {
			if string(got[i].Key) != key || string(got[i].Value) != model[key] {
				t.Fatalf("Range entry %d = %q:%q, want %q:%q", i, got[i].Key, got[i].Value, key, model[key])
			}
		}
	}
	checkRange("", "\xff")
	for i := 0; i < 2000; i++ {
		key := fmt.Sprintf("k%03d", rng.Intn(100))
		switch rng.Intn(3) {
		case 0:
			value := fmt.Sprintf("v%d", i)
			if err := m.Set([]byte(key), []byte(value)); err != nil {
				t.Fatal(err)
			}
			model[key] = value
		case 1:
			_, want := model[key]
			if got, err := m.Delete([]byte(key)); err != nil || got != want {
				t.Fatalf("Delete(%q) = %v, %v; want %v", key, got, err, want)
			}
			delete(model, key)
		case 2:
			want, exists := model[key]
			if got, found, err := m.Get([]byte(key)); err != nil || found != exists || string(got) != want {
				t.Fatalf("Get(%q) = %q, %v, %v; want %q, %v", key, got, found, err, want, exists)
			}
		}
		checkRange("", "\xff")
		checkRange(key, "k075")
		checkRange(key, key)
	}
	for key := range model {
		if found, err := m.Delete([]byte(key)); err != nil || !found {
			t.Fatalf("final Delete(%q) = %v, %v", key, found, err)
		}
		delete(model, key)
		checkRange("", "\xff")
	}
}

func TestMemoryByteOwnershipAndBounds(t *testing.T) {
	var m btree.Memory
	key, value := []byte{0, 255}, []byte{255, 0}
	if err := m.Set(key, value); err != nil {
		t.Fatal(err)
	}
	key[0], value[0] = 42, 42
	assertValue := func() {
		t.Helper()
		got, found, err := m.Get([]byte{0, 255})
		if err != nil || !found || string(got) != "\xff\x00" {
			t.Fatalf("Get = %v, %v, %v", got, found, err)
		}
		got[0] = 42
	}
	assertValue()
	assertValue()
	entries, err := m.Range(nil, []byte{255})
	if err != nil || len(entries) != 1 {
		t.Fatalf("Range = %v, %v", entries, err)
	}
	entries[0].Key[0], entries[0].Value[0] = 42, 42
	assertValue()
	if err := m.Set(nil, nil); err != nil {
		t.Fatal(err)
	}
	entries, err = m.Range(nil, nil)
	if err != nil || len(entries) != 1 || len(entries[0].Key) != 0 || len(entries[0].Value) != 0 {
		t.Fatalf("empty bounds = %v, %v", entries, err)
	}
	if got, found, err := m.Get(nil); err != nil || !found || len(got) != 0 {
		t.Fatalf("empty key Get = %v, %v, %v", got, found, err)
	}
	for i := 0; i < 2; i++ {
		if err := m.Close(); err != nil {
			t.Fatal(err)
		}
	}
	assertValue()
}
