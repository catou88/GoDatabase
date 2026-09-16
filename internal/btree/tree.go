package btree

type tree struct {
	root    *node
	maxKeys int
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
