package btree

type tree struct {
	root    *node
	maxKeys int
}

func (t *tree) minKeys() int {
	if t.maxKeys == 0 {
		t.maxKeys = 3
	}
	return t.maxKeys / 2
}

func (t *tree) get(key string) (string, bool) {
	if t.root == nil {
		return "", false
	}

	n := t.root
	for !n.leaf {
		n = n.child(key)
	}

	idx, found := n.search(key)
	if !found {
		return "", false
	}

	return n.valueAt(idx), true
}

func (t *tree) set(key, value string) {
	if t.maxKeys == 0 {
		t.maxKeys = 3
	}
	if t.root == nil {
		t.root = &node{
			leaf:   true,
			keys:   []string{key},
			values: []string{value},
		}
		return
	}

	split, ok := t.insert(t.root, key, value)
	if !ok {
		return
	}

	t.root = &node{
		keys:     []string{split.key},
		children: []*node{split.left, split.right},
	}
}

type nodeSplit struct {
	key   string
	left  *node
	right *node
}

func (t *tree) insert(n *node, key, value string) (nodeSplit, bool) {
	if n.leaf {
		insertIntoLeaf(n, key, value)
		if len(n.keys) <= t.maxKeys {
			return nodeSplit{}, false
		}
		return splitLeaf(n), true
	}

	childIdx := n.childIndex(key)
	childSplit, ok := t.insert(n.children[childIdx], key, value)
	if !ok {
		return nodeSplit{}, false
	}

	insertIntoInternal(n, childIdx, childSplit)
	if len(n.keys) <= t.maxKeys {
		return nodeSplit{}, false
	}
	return splitInternal(n), true
}

func insertIntoLeaf(n *node, key, value string) {
	idx, found := n.search(key)
	if found {
		n.values[idx] = value
		return
	}

	n.keys = insertString(n.keys, idx, key)
	n.values = insertString(n.values, idx, value)
}

func insertIntoInternal(n *node, childIdx int, split nodeSplit) {
	n.keys = insertString(n.keys, childIdx, split.key)
	n.children[childIdx] = split.left
	n.children = insertNode(n.children, childIdx+1, split.right)
}

func splitLeaf(n *node) nodeSplit {
	mid := len(n.keys) / 2
	right := &node{
		leaf:   true,
		keys:   append([]string(nil), n.keys[mid:]...),
		values: append([]string(nil), n.values[mid:]...),
		next:   n.next,
	}

	n.keys = n.keys[:mid]
	n.values = n.values[:mid]
	n.next = right

	return nodeSplit{key: right.keys[0], left: n, right: right}
}

func splitInternal(n *node) nodeSplit {
	mid := len(n.keys) / 2
	promoted := n.keys[mid]
	right := &node{
		keys:     append([]string(nil), n.keys[mid+1:]...),
		children: append([]*node(nil), n.children[mid+1:]...),
	}

	n.keys = n.keys[:mid]
	n.children = n.children[:mid+1]

	return nodeSplit{key: promoted, left: n, right: right}
}

func insertString(values []string, idx int, value string) []string {
	values = append(values, "")
	copy(values[idx+1:], values[idx:])
	values[idx] = value
	return values
}

func insertNode(nodes []*node, idx int, value *node) []*node {
	nodes = append(nodes, nil)
	copy(nodes[idx+1:], nodes[idx:])
	nodes[idx] = value
	return nodes
}

func (t *tree) delete(key string) bool {
	if t.root == nil {
		return false
	}

	deleted := t.deleteFromNode(t.root, key)
	if !deleted {
		return false
	}

	if len(t.root.keys) == 0 {
		if t.root.leaf {
			t.root = nil
		} else {
			t.root = t.root.children[0]
		}
	}

	return true
}

func (t *tree) deleteFromNode(n *node, key string) bool {
	if n.leaf {
		idx, found := n.search(key)
		if !found {
			return false
		}
		n.keys = deleteString(n.keys, idx)
		n.values = deleteString(n.values, idx)
		return true
	}

	childIdx := n.childIndex(key)
	deleted := t.deleteFromNode(n.children[childIdx], key)
	if !deleted {
		return false
	}

	if childIdx > 0 && len(n.children[childIdx].keys) > 0 {
		n.keys[childIdx-1] = minKey(n.children[childIdx])
	}
	if len(n.children[childIdx].keys) < t.minKeys() {
		t.rebalanceChild(n, childIdx)
	}

	return true
}

func (t *tree) rebalanceChild(parent *node, childIdx int) {
	if childIdx > 0 && len(parent.children[childIdx-1].keys) > t.minKeys() {
		rotateFromLeft(parent, childIdx)
		return
	}
	if childIdx+1 < len(parent.children) && len(parent.children[childIdx+1].keys) > t.minKeys() {
		rotateFromRight(parent, childIdx)
		return
	}
	if childIdx > 0 {
		mergeChildren(parent, childIdx-1)
		return
	}
	mergeChildren(parent, childIdx)
}

func rotateFromLeft(parent *node, childIdx int) {
	left := parent.children[childIdx-1]
	child := parent.children[childIdx]

	if child.leaf {
		last := len(left.keys) - 1
		child.keys = insertString(child.keys, 0, left.keys[last])
		child.values = insertString(child.values, 0, left.values[last])
		left.keys = left.keys[:last]
		left.values = left.values[:last]
		parent.keys[childIdx-1] = child.keys[0]
		return
	}

	last := len(left.keys) - 1
	child.keys = insertString(child.keys, 0, parent.keys[childIdx-1])
	child.children = insertNode(child.children, 0, left.children[len(left.children)-1])
	parent.keys[childIdx-1] = left.keys[last]
	left.keys = left.keys[:last]
	left.children = left.children[:len(left.children)-1]
}

func rotateFromRight(parent *node, childIdx int) {
	child := parent.children[childIdx]
	right := parent.children[childIdx+1]

	if child.leaf {
		child.keys = append(child.keys, right.keys[0])
		child.values = append(child.values, right.values[0])
		right.keys = deleteString(right.keys, 0)
		right.values = deleteString(right.values, 0)
		parent.keys[childIdx] = right.keys[0]
		return
	}

	child.keys = append(child.keys, parent.keys[childIdx])
	child.children = append(child.children, right.children[0])
	parent.keys[childIdx] = right.keys[0]
	right.keys = deleteString(right.keys, 0)
	right.children = deleteNode(right.children, 0)
}

func mergeChildren(parent *node, leftIdx int) {
	left := parent.children[leftIdx]
	right := parent.children[leftIdx+1]

	if left.leaf {
		left.keys = append(left.keys, right.keys...)
		left.values = append(left.values, right.values...)
		left.next = right.next
	} else {
		left.keys = append(left.keys, parent.keys[leftIdx])
		left.keys = append(left.keys, right.keys...)
		left.children = append(left.children, right.children...)
	}

	parent.keys = deleteString(parent.keys, leftIdx)
	parent.children = deleteNode(parent.children, leftIdx+1)
}

func minKey(n *node) string {
	for !n.leaf {
		n = n.children[0]
	}
	return n.keys[0]
}

func deleteString(values []string, idx int) []string {
	copy(values[idx:], values[idx+1:])
	return values[:len(values)-1]
}

func deleteNode(nodes []*node, idx int) []*node {
	copy(nodes[idx:], nodes[idx+1:])
	return nodes[:len(nodes)-1]
}
