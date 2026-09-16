package btree

type tree struct {
	root *node
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
