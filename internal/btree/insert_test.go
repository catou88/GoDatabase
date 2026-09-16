package btree

import (
	"fmt"
	"testing"
)

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

	if err := validateTree(tree); err != nil {
		t.Fatal(err)
	}
}

func validateTree(tree *tree) error {
	if tree.root == nil {
		return nil
	}

	leafDepth := -1
	return validateNode(tree.root, tree.maxKeys, tree.minKeys(), 0, &leafDepth, true, "", "")
}

func validateNode(
	n *node,
	maxKeys int,
	minKeys int,
	depth int,
	leafDepth *int,
	root bool,
	lower string,
	upper string,
) error {
	if len(n.keys) > maxKeys {
		return fmt.Errorf("node has %d keys, max is %d: %#v", len(n.keys), maxKeys, n.keys)
	}
	if !root && len(n.keys) < minKeys {
		return fmt.Errorf("non-root node has %d keys, min is %d: %#v", len(n.keys), minKeys, n.keys)
	}
	for i := 1; i < len(n.keys); i++ {
		if n.keys[i-1] >= n.keys[i] {
			return fmt.Errorf("keys are not sorted: %#v", n.keys)
		}
	}
	for _, key := range n.keys {
		if lower != "" && key < lower {
			return fmt.Errorf("key %q is below lower bound %q in node %#v", key, lower, n.keys)
		}
		if upper != "" && key >= upper {
			return fmt.Errorf("key %q is at or above upper bound %q in node %#v", key, upper, n.keys)
		}
	}

	if n.leaf {
		if len(n.values) != len(n.keys) {
			return fmt.Errorf("leaf has %d keys and %d values", len(n.keys), len(n.values))
		}
		if len(n.children) != 0 {
			return fmt.Errorf("leaf has %d children", len(n.children))
		}
		if *leafDepth == -1 {
			*leafDepth = depth
		}
		if depth != *leafDepth {
			return fmt.Errorf("leaf depth = %d, want %d", depth, *leafDepth)
		}
		return nil
	}

	if len(n.values) != 0 {
		return fmt.Errorf("internal node has %d values", len(n.values))
	}
	if len(n.children) != len(n.keys)+1 {
		return fmt.Errorf("internal node has %d children and %d keys", len(n.children), len(n.keys))
	}

	for i, child := range n.children {
		childLower := lower
		if i > 0 {
			childLower = n.keys[i-1]
		}
		childUpper := upper
		if i < len(n.keys) {
			childUpper = n.keys[i]
		}
		if err := validateNode(child, maxKeys, minKeys, depth+1, leafDepth, false, childLower, childUpper); err != nil {
			return err
		}
	}
	return nil
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
