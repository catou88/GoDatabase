package btree

import "testing"

func TestTreeDeleteMissingKey(t *testing.T) {
	tree := newTestTree("a", "b", "c")

	if tree.delete("missing") {
		t.Fatal("expected deleting missing key to return false")
	}
	assertTreeInvariants(t, tree)
	assertGet(t, tree, "a", "a")
	assertGet(t, tree, "b", "b")
	assertGet(t, tree, "c", "c")
}

func TestTreeDeleteRemovesExistingKey(t *testing.T) {
	tree := newTestTree("a", "b", "c")

	if !tree.delete("b") {
		t.Fatal("expected delete to remove existing key")
	}
	assertTreeInvariants(t, tree)
	assertMissing(t, tree, "b")
	assertGet(t, tree, "a", "a")
	assertGet(t, tree, "c", "c")
}

func TestTreeDeleteMergesUnderfullLeafAndShrinksRoot(t *testing.T) {
	tree := newTestTree("a", "b", "c", "d")

	if !tree.delete("a") {
		t.Fatal("expected delete to remove existing key")
	}
	if !tree.delete("b") {
		t.Fatal("expected delete to remove existing key")
	}
	if !tree.delete("c") {
		t.Fatal("expected delete to remove existing key")
	}

	assertTreeInvariants(t, tree)
	if tree.root == nil {
		t.Fatal("expected non-empty tree")
	}
	if !tree.root.leaf {
		t.Fatal("expected root to shrink back to leaf")
	}
	assertMissing(t, tree, "a")
	assertMissing(t, tree, "b")
	assertMissing(t, tree, "c")
	assertGet(t, tree, "d", "d")
}

func TestTreeDeleteRedistributesFromSibling(t *testing.T) {
	tree := newTestTree("a", "b", "c", "d", "e")

	if !tree.delete("a") {
		t.Fatal("expected delete to remove existing key")
	}

	assertTreeInvariants(t, tree)
	assertMissing(t, tree, "a")
	assertGet(t, tree, "b", "b")
	assertGet(t, tree, "c", "c")
	assertGet(t, tree, "d", "d")
	assertGet(t, tree, "e", "e")
}

func TestTreeDeleteAfterNonRootSplits(t *testing.T) {
	tree := newTestTree("a", "b", "c", "d", "e", "f", "g", "h", "i", "j")

	for _, key := range []string{"a", "b", "c", "d", "e"} {
		if !tree.delete(key) {
			t.Fatalf("expected delete to remove %q", key)
		}
		assertTreeInvariants(t, tree)
		assertMissing(t, tree, key)
	}

	assertGet(t, tree, "f", "f")
	assertGet(t, tree, "g", "g")
	assertGet(t, tree, "h", "h")
	assertGet(t, tree, "i", "i")
	assertGet(t, tree, "j", "j")
}

func TestTreeDeleteLastKeyEmptiesTree(t *testing.T) {
	tree := newTestTree("a")

	if !tree.delete("a") {
		t.Fatal("expected delete to remove existing key")
	}
	assertTreeInvariants(t, tree)
	if tree.root != nil {
		t.Fatalf("expected empty tree, got root %#v", tree.root)
	}
}

func newTestTree(keys ...string) *tree {
	tree := &tree{maxKeys: 3}
	for _, key := range keys {
		tree.set(key, key)
	}
	return tree
}

func assertMissing(t *testing.T, tree *tree, key string) {
	t.Helper()

	value, ok := tree.get(key)
	if ok {
		t.Fatalf("expected key %q to be missing, got %q", key, value)
	}
}
