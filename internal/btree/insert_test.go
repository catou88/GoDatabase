package btree

import "testing"

func TestTreeSetInsertsIntoLeafSorted(t *testing.T) {
	tree := &tree{maxKeys: 3}

	tree.set("c", "three")
	tree.set("a", "one")
	tree.set("b", "two")

	assertTreeInvariants(t, tree)
	assertGet(t, tree, "a", "one")
	assertGet(t, tree, "b", "two")
	assertGet(t, tree, "c", "three")

	wantKeys := []string{"a", "b", "c"}
	assertStrings(t, tree.root.keys, wantKeys)
}

func TestTreeSetOverwritesExistingKey(t *testing.T) {
	tree := &tree{maxKeys: 3}

	tree.set("a", "one")
	tree.set("a", "updated")

	assertTreeInvariants(t, tree)
	assertGet(t, tree, "a", "updated")

	if len(tree.root.keys) != 1 {
		t.Fatalf("expected one key after overwrite, got %d", len(tree.root.keys))
	}
}

func TestTreeSetSplitsRootLeaf(t *testing.T) {
	tree := &tree{maxKeys: 3}

	for _, pair := range []struct{ key, value string }{
		{"a", "one"},
		{"b", "two"},
		{"c", "three"},
		{"d", "four"},
	} {
		tree.set(pair.key, pair.value)
	}

	assertTreeInvariants(t, tree)
	if tree.root.leaf {
		t.Fatal("expected root to become internal after split")
	}
	assertStrings(t, tree.root.keys, []string{"c"})
	if len(tree.root.children) != 2 {
		t.Fatalf("expected root to have 2 children, got %d", len(tree.root.children))
	}

	assertGet(t, tree, "a", "one")
	assertGet(t, tree, "b", "two")
	assertGet(t, tree, "c", "three")
	assertGet(t, tree, "d", "four")
}

func TestTreeSetPropagatesNonRootSplit(t *testing.T) {
	tree := &tree{maxKeys: 3}

	for _, pair := range []struct{ key, value string }{
		{"a", "one"},
		{"b", "two"},
		{"c", "three"},
		{"d", "four"},
		{"e", "five"},
		{"f", "six"},
		{"g", "seven"},
		{"h", "eight"},
		{"i", "nine"},
		{"j", "ten"},
	} {
		tree.set(pair.key, pair.value)
	}

	assertTreeInvariants(t, tree)
	if tree.root.leaf {
		t.Fatal("expected internal root")
	}
	if len(tree.root.children) < 2 {
		t.Fatalf("expected multiple root children, got %d", len(tree.root.children))
	}

	assertGet(t, tree, "a", "one")
	assertGet(t, tree, "e", "five")
	assertGet(t, tree, "j", "ten")
}

func assertGet(t *testing.T, tree *tree, key, want string) {
	t.Helper()

	got, ok := tree.get(key)
	if !ok {
		t.Fatalf("expected key %q to exist", key)
	}
	if got != want {
		t.Fatalf("get(%q) = %q, want %q", key, got, want)
	}
}

func assertTreeInvariants(t *testing.T, tree *tree) {
	t.Helper()

	if tree.root == nil {
		return
	}

	leafDepth := -1
	assertNodeInvariants(t, tree.root, tree.maxKeys, 0, &leafDepth, true)
}

func assertNodeInvariants(t *testing.T, n *node, maxKeys int, depth int, leafDepth *int, root bool) {
	t.Helper()

	if len(n.keys) > maxKeys {
		t.Fatalf("node has %d keys, max is %d: %#v", len(n.keys), maxKeys, n.keys)
	}
	for i := 1; i < len(n.keys); i++ {
		if n.keys[i-1] >= n.keys[i] {
			t.Fatalf("keys are not sorted: %#v", n.keys)
		}
	}

	if n.leaf {
		if len(n.values) != len(n.keys) {
			t.Fatalf("leaf has %d keys and %d values", len(n.keys), len(n.values))
		}
		if len(n.children) != 0 {
			t.Fatalf("leaf has %d children", len(n.children))
		}
		if *leafDepth == -1 {
			*leafDepth = depth
		}
		if depth != *leafDepth {
			t.Fatalf("leaf depth = %d, want %d", depth, *leafDepth)
		}
		return
	}

	if len(n.values) != 0 {
		t.Fatalf("internal node has %d values", len(n.values))
	}
	if len(n.children) != len(n.keys)+1 {
		t.Fatalf("internal node has %d children and %d keys", len(n.children), len(n.keys))
	}
	if !root && len(n.keys) == 0 {
		t.Fatal("non-root internal node has no keys")
	}

	for _, child := range n.children {
		assertNodeInvariants(t, child, maxKeys, depth+1, leafDepth, false)
	}
}

func assertStrings(t *testing.T, got []string, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("got keys %#v, want %#v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("got keys %#v, want %#v", got, want)
		}
	}
}
