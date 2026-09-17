package btree

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestPageTreeRangeValues(t *testing.T) {
	tree, _ := newFakePageTree()
	for _, entry := range []struct {
		key   string
		value string
	}{
		{key: "alpha", value: "one"},
		{key: "bravo", value: "two"},
		{key: "charlie", value: "three"},
		{key: "delta", value: "four"},
	} {
		if err := tree.insert([]byte(entry.key), []byte(entry.value)); err != nil {
			t.Fatalf("insert(%q) error = %v", entry.key, err)
		}
	}

	tests := []struct {
		name  string
		start string
		end   string
		want  []string
	}{
		{name: "inclusive bounds", start: "bravo", end: "charlie", want: []string{"bravo", "charlie"}},
		{name: "single matching key", start: "bravo", end: "bravo", want: []string{"bravo"}},
		{name: "positions between keys", start: "b", end: "c", want: []string{"bravo"}},
		{name: "no matching keys", start: "echo", end: "foxtrot", want: nil},
		{name: "start greater than end", start: "delta", end: "alpha", want: nil},
		{name: "empty start", start: "", end: "bravo", want: []string{"alpha", "bravo"}},
		{name: "empty end", start: "alpha", end: "", want: nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tree.rangeValues([]byte(tt.start), []byte(tt.end))
			if err != nil {
				t.Fatalf("rangeValues(%q, %q) error = %v", tt.start, tt.end, err)
			}
			assertTreeEntryKeys(t, got, tt.want)
		})
	}
}

func TestPageTreeRangeValuesEmptyTree(t *testing.T) {
	tree, _ := newFakePageTree()
	got, err := tree.rangeValues(nil, []byte("z"))
	if err != nil {
		t.Fatalf("rangeValues() error = %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("rangeValues() returned %d entries, want 0", len(got))
	}
}

func TestPageTreeRangeValuesAcrossLeafPages(t *testing.T) {
	tree, store := newFakePageTree()
	const keyCount = 30
	for i := 0; i < keyCount; i++ {
		key := fmt.Sprintf("key-%04d", i)
		value := strings.Repeat(string(rune('a'+i%26)), maxValueSize)
		if err := tree.insert([]byte(key), []byte(value)); err != nil {
			t.Fatalf("insert(%q) error = %v", key, err)
		}
	}
	if height := pageTreeHeight(store, tree.root); height < 2 {
		t.Fatalf("tree height = %d, want multiple leaf pages", height)
	}

	got, err := tree.rangeValues([]byte("key-0007"), []byte("key-0023"))
	if err != nil {
		t.Fatalf("rangeValues() error = %v", err)
	}
	want := make([]string, 0, 17)
	for i := 7; i <= 23; i++ {
		want = append(want, fmt.Sprintf("key-%04d", i))
	}
	assertTreeEntryKeys(t, got, want)
}

func TestPageTreeRangeValuesRejectsOversizedBounds(t *testing.T) {
	tree, _ := newFakePageTree()
	tooLarge := bytes.Repeat([]byte{'k'}, maxKeySize+1)
	if _, err := tree.rangeValues(tooLarge, []byte("z")); err == nil {
		t.Fatal("rangeValues(oversized start) error = nil, want error")
	}
	if _, err := tree.rangeValues(nil, tooLarge); err == nil {
		t.Fatal("rangeValues(oversized end) error = nil, want error")
	}
}

func TestKVRangePersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	kv := mustOpenKV(t, path)
	keys := make([]string, 24)
	for i := range keys {
		keys[i] = largeTestKey(i)
		if err := kv.Set([]byte(keys[i]), []byte(fmt.Sprintf("value-%02d", i))); err != nil {
			t.Fatalf("Set(%d) error = %v", i, err)
		}
	}
	if pageTreeHeightFromPages(kv.pages, kv.tree.root) < 2 {
		t.Fatal("test data did not create multiple leaf pages")
	}
	if err := kv.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	kv = mustOpenKV(t, path)
	defer closeKV(t, kv)
	got, err := kv.Range([]byte(keys[5]), []byte(keys[18]))
	if err != nil {
		t.Fatalf("Range() error = %v", err)
	}
	if len(got) != 14 {
		t.Fatalf("Range() returned %d entries, want 14", len(got))
	}
	for i, entry := range got {
		wantIndex := i + 5
		if string(entry.Key) != keys[wantIndex] || string(entry.Value) != fmt.Sprintf("value-%02d", wantIndex) {
			t.Fatalf("Range()[%d] = (%q, %q), want (%q, %q)", i, entry.Key, entry.Value, keys[wantIndex], fmt.Sprintf("value-%02d", wantIndex))
		}
	}
}

func TestKVRangeRejectsClosedDatabase(t *testing.T) {
	kv := mustOpenKV(t, filepath.Join(t.TempDir(), "database.db"))
	if err := kv.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, err := kv.Range(nil, []byte("z")); !errors.Is(err, ErrClosed) {
		t.Fatalf("Range() error = %v, want %v", err, ErrClosed)
	}
}

func assertTreeEntryKeys(t *testing.T, entries []treeEntry, want []string) {
	t.Helper()
	if len(entries) != len(want) {
		t.Fatalf("range returned %d entries, want %d", len(entries), len(want))
	}
	for i, entry := range entries {
		if string(entry.key) != want[i] {
			t.Fatalf("range key %d = %q, want %q", i, entry.key, want[i])
		}
		if i > 0 && bytes.Compare(entries[i-1].key, entry.key) >= 0 {
			t.Fatalf("range keys are not strictly ascending at index %d", i)
		}
	}
}
