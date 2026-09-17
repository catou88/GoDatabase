package btree

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestPageTreeGetEmpty(t *testing.T) {
	tree, _ := newFakePageTree()

	value, found, err := tree.getValue([]byte("missing"))
	if err != nil {
		t.Fatalf("getValue() error = %v", err)
	}
	if found || value != nil {
		t.Fatalf("getValue() = (%q, %v), want (nil, false)", value, found)
	}
}

func TestPageTreeInsertGetAndOverwriteAreCopyOnWrite(t *testing.T) {
	tree, store := newFakePageTree()
	if err := tree.insert([]byte("alpha"), []byte("one")); err != nil {
		t.Fatalf("insert(first) error = %v", err)
	}

	oldRootID := tree.root
	oldRoot := cloneBytes(store.pages[oldRootID])
	if err := tree.insert([]byte("alpha"), []byte("two")); err != nil {
		t.Fatalf("insert(overwrite) error = %v", err)
	}
	if tree.root == oldRootID {
		t.Fatalf("overwrite retained root page %d, want a replacement page", oldRootID)
	}
	if !bytes.Equal(store.pages[oldRootID], oldRoot) {
		t.Fatal("overwrite modified the committed root page")
	}
	if !store.released[oldRootID] {
		t.Fatalf("old root page %d was not released", oldRootID)
	}

	value, found, err := tree.getValue([]byte("alpha"))
	if err != nil {
		t.Fatalf("getValue(alpha) error = %v", err)
	}
	if !found || string(value) != "two" {
		t.Fatalf("getValue(alpha) = (%q, %v), want (%q, true)", value, found, "two")
	}
	if _, found, err := tree.getValue([]byte("missing")); err != nil || found {
		t.Fatalf("getValue(missing) found = %v, error = %v; want false, nil", found, err)
	}
}

func TestPageTreeInsertPropagatesRootAndNonRootSplits(t *testing.T) {
	tree, store := newFakePageTree()
	reference := make(map[string]string)

	for i := 0; i < 220; i++ {
		key := fmt.Sprintf("key-%04d", i)
		value := strings.Repeat(string(rune('a'+i%26)), maxValueSize)
		if err := tree.insert([]byte(key), []byte(value)); err != nil {
			t.Fatalf("insert(%q) error = %v", key, err)
		}
		reference[key] = value
	}

	if height := pageTreeHeight(store, tree.root); height < 3 {
		t.Fatalf("tree height = %d, want at least 3 after propagated splits", height)
	}
	for key, want := range reference {
		got, found, err := tree.getValue([]byte(key))
		if err != nil {
			t.Fatalf("getValue(%q) error = %v", key, err)
		}
		if !found || string(got) != want {
			t.Fatalf("getValue(%q) = (%q, %v), want value and true", key, got, found)
		}
	}
	stored := make(map[string]string)
	collectPageTreeEntries(t, store, tree.root, stored)
	if len(stored) != len(reference) {
		t.Fatalf("stored key count = %d, want %d", len(stored), len(reference))
	}
	for key, want := range reference {
		if got := stored[key]; got != want {
			t.Fatalf("stored[%q] = %q, want %q", key, got, want)
		}
	}
	for pageID, page := range store.pages {
		if err := validateBNode(page); err != nil {
			t.Fatalf("page %d validation error = %v", pageID, err)
		}
	}
	if len(store.released) == 0 {
		t.Fatal("split workload did not release any replaced pages")
	}
}

func TestNodeSplit3UsesEncodedByteSize(t *testing.T) {
	tests := []struct {
		name       string
		valueSizes []int
		wantParts  int
	}{
		{name: "one page", valueSizes: []int{100, 100}, wantParts: 1},
		{name: "two pages", valueSizes: []int{1500, 1500, 1500}, wantParts: 2},
		{name: "three pages", valueSizes: []int{1900, 2900, 1900}, wantParts: 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			node := encodedLeafForSplit(t, tt.valueSizes)
			parts, err := nodeSplit3(node)
			if err != nil {
				t.Fatalf("nodeSplit3() error = %v", err)
			}
			if len(parts) != tt.wantParts {
				t.Fatalf("nodeSplit3() returned %d parts, want %d", len(parts), tt.wantParts)
			}
			for i, part := range parts {
				if len(part) != pageSize {
					t.Fatalf("part %d size = %d, want %d", i, len(part), pageSize)
				}
				if err := validateBNode(part); err != nil {
					t.Fatalf("part %d validation error = %v", i, err)
				}
			}
		})
	}
}

func TestNodeLookupLE(t *testing.T) {
	page, err := encodePageNode(&pageNode{
		keys:       []string{"", "m", "t"},
		childPages: []uint64{1, 2, 3},
	})
	if err != nil {
		t.Fatalf("encodePageNode() error = %v", err)
	}

	tests := []struct {
		key  string
		want uint16
	}{
		{key: "a", want: 0},
		{key: "m", want: 1},
		{key: "s", want: 1},
		{key: "t", want: 2},
		{key: "z", want: 2},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			got, err := nodeLookupLE(BNode(page), []byte(tt.key))
			if err != nil {
				t.Fatalf("nodeLookupLE(%q) error = %v", tt.key, err)
			}
			if got != tt.want {
				t.Fatalf("nodeLookupLE(%q) = %d, want %d", tt.key, got, tt.want)
			}
		})
	}
}

type fakePageStore struct {
	pages    map[uint64]BNode
	released map[uint64]bool
	nextID   uint64
}

func newFakePageTree() (*pageTree, *fakePageStore) {
	store := &fakePageStore{
		pages:    make(map[uint64]BNode),
		released: make(map[uint64]bool),
		nextID:   1,
	}
	tree := &pageTree{
		get: func(pageID uint64) BNode {
			page, ok := store.pages[pageID]
			if !ok {
				panic(fmt.Sprintf("page %d does not exist", pageID))
			}
			return page
		},
		new: func(node BNode) uint64 {
			if err := validateBNode(node); err != nil {
				panic(fmt.Sprintf("new page is invalid: %v", err))
			}
			pageID := store.nextID
			store.nextID++
			store.pages[pageID] = BNode(cloneBytes(node))
			return pageID
		},
		del: func(pageID uint64) {
			if _, ok := store.pages[pageID]; !ok {
				panic(fmt.Sprintf("released page %d does not exist", pageID))
			}
			if store.released[pageID] {
				panic(fmt.Sprintf("page %d released twice", pageID))
			}
			store.released[pageID] = true
		},
	}
	return tree, store
}

func pageTreeHeight(store *fakePageStore, rootPageID uint64) int {
	height := 0
	pageID := rootPageID
	for pageID != 0 {
		height++
		node := store.pages[pageID]
		if node.btype() == nodeTypeLeaf {
			break
		}
		pageID = node.getPtr(0)
	}
	return height
}

func collectPageTreeEntries(t *testing.T, store *fakePageStore, pageID uint64, entries map[string]string) {
	t.Helper()

	node := store.pages[pageID]
	if node.btype() == nodeTypeLeaf {
		for i := uint16(0); i < node.nkeys(); i++ {
			key := string(node.getKey(i))
			if key == "" {
				continue
			}
			if _, duplicate := entries[key]; duplicate {
				t.Fatalf("duplicate key %q in leaf pages", key)
			}
			entries[key] = string(node.getVal(i))
		}
		return
	}

	for i := uint16(0); i < node.nkeys(); i++ {
		child := store.pages[node.getPtr(i)]
		if !bytes.Equal(node.getKey(i), child.getKey(0)) {
			t.Fatalf(
				"parent lower bound %q does not match child first key %q",
				node.getKey(i),
				child.getKey(0),
			)
		}
		collectPageTreeEntries(t, store, node.getPtr(i), entries)
	}
}

func encodedLeafForSplit(t *testing.T, valueSizes []int) BNode {
	t.Helper()

	node := BNode(make([]byte, 3*pageSize))
	node.setHeader(nodeTypeLeaf, uint16(len(valueSizes)))
	for i, valueSize := range valueSizes {
		key := []byte(fmt.Sprintf("key-%02d", i))
		value := bytes.Repeat([]byte{'v'}, valueSize)
		if err := nodeAppendKV(node, uint16(i), 0, key, value); err != nil {
			t.Fatalf("nodeAppendKV(%d) error = %v", i, err)
		}
	}
	return node
}
