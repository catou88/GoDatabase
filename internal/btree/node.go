package btree

import "sort"

type node struct {
	leaf     bool
	keys     []string
	values   []string
	children []*node
	next     *node
}

func (n *node) search(key string) (int, bool) {
	idx := sort.SearchStrings(n.keys, key)
	if idx < len(n.keys) && n.keys[idx] == key {
		return idx, true
	}
	return idx, false
}

func (n *node) insertPosition(key string) int {
	idx, _ := n.search(key)
	return idx
}

func (n *node) childIndex(key string) int {
	return sort.Search(len(n.keys), func(i int) bool {
		return key < n.keys[i]
	})
}

func (n *node) child(key string) *node {
	return n.children[n.childIndex(key)]
}
