package btree

import (
	"bytes"
	"fmt"
	"sort"
)

// pageTree stores immutable B+Tree nodes behind page-management callbacks.
type pageTree struct {
	root uint64
	get  func(uint64) BNode
	new  func(BNode) uint64
	del  func(uint64)
}

type treeEntry struct {
	key   []byte
	value []byte
}

type pageTreeCursorFrame struct {
	node     BNode
	childIdx uint16
}

func (tree *pageTree) getValue(key []byte) ([]byte, bool, error) {
	if len(key) == 0 {
		return nil, false, fmt.Errorf("key is empty")
	}
	if tree.root == 0 {
		return nil, false, nil
	}
	if err := tree.validateCallbacks(); err != nil {
		return nil, false, err
	}

	node := tree.get(tree.root)
	for {
		if err := validateBNode(node); err != nil {
			return nil, false, fmt.Errorf("read page: %w", err)
		}
		idx, err := nodeLookupLE(node, key)
		if err != nil {
			return nil, false, err
		}
		if node.btype() == nodeTypeLeaf {
			if !bytes.Equal(node.getKey(idx), key) {
				return nil, false, nil
			}
			return append([]byte(nil), node.getVal(idx)...), true, nil
		}
		node = tree.get(node.getPtr(idx))
	}
}

func (tree *pageTree) rangeValues(start, end []byte) ([]treeEntry, error) {
	if len(start) > maxKeySize {
		return nil, fmt.Errorf("start key exceeds max size")
	}
	if len(end) > maxKeySize {
		return nil, fmt.Errorf("end key exceeds max size")
	}
	if bytes.Compare(start, end) > 0 || tree.root == 0 {
		return []treeEntry{}, nil
	}
	if err := tree.validateCallbacks(); err != nil {
		return nil, err
	}

	leaf, entryIdx, path, err := tree.seekRangeStart(start)
	if err != nil {
		return nil, err
	}
	entries := make([]treeEntry, 0)
	for {
		for entryIdx < leaf.nkeys() {
			idx := entryIdx
			key := leaf.getKey(idx)
			entryIdx++
			if len(key) == 0 || bytes.Compare(key, start) < 0 {
				continue
			}
			if bytes.Compare(key, end) > 0 {
				return entries, nil
			}
			entries = append(entries, treeEntry{
				key:   append([]byte(nil), key...),
				value: append([]byte(nil), leaf.getVal(idx)...),
			})
		}

		leaf, path, err = tree.nextLeaf(path)
		if err != nil {
			return nil, err
		}
		if leaf == nil {
			return entries, nil
		}
		entryIdx = 0
	}
}

func (tree *pageTree) seekRangeStart(start []byte) (BNode, uint16, []pageTreeCursorFrame, error) {
	node := tree.get(tree.root)
	path := make([]pageTreeCursorFrame, 0)
	for {
		if err := validateBNode(node); err != nil {
			return nil, 0, nil, fmt.Errorf("read range page: %w", err)
		}
		if node.btype() == nodeTypeLeaf {
			idx := sort.Search(int(node.nkeys()), func(i int) bool {
				return bytes.Compare(node.getKey(uint16(i)), start) >= 0
			})
			return node, uint16(idx), path, nil
		}
		idx, err := nodeLookupLE(node, start)
		if err != nil {
			return nil, 0, nil, err
		}
		path = append(path, pageTreeCursorFrame{node: node, childIdx: idx})
		node = tree.get(node.getPtr(idx))
	}
}

func (tree *pageTree) nextLeaf(path []pageTreeCursorFrame) (BNode, []pageTreeCursorFrame, error) {
	for len(path) > 0 {
		last := len(path) - 1
		frame := path[last]
		if frame.childIdx+1 >= frame.node.nkeys() {
			path = path[:last]
			continue
		}

		frame.childIdx++
		path[last] = frame
		node := tree.get(frame.node.getPtr(frame.childIdx))
		for {
			if err := validateBNode(node); err != nil {
				return nil, nil, fmt.Errorf("read range page: %w", err)
			}
			if node.btype() == nodeTypeLeaf {
				return node, path, nil
			}
			path = append(path, pageTreeCursorFrame{node: node, childIdx: 0})
			node = tree.get(node.getPtr(0))
		}
	}
	return nil, path, nil
}

func (tree *pageTree) insert(key, value []byte) error {
	if err := validateTreeEntry(key, value); err != nil {
		return err
	}
	if err := tree.validateCallbacks(); err != nil {
		return err
	}

	if tree.root == 0 {
		root := BNode(make([]byte, pageSize))
		root.setHeader(nodeTypeLeaf, 2)
		if err := nodeAppendKV(root, 0, 0, nil, nil); err != nil {
			return err
		}
		if err := nodeAppendKV(root, 1, 0, key, value); err != nil {
			return err
		}
		tree.root = tree.new(root)
		return nil
	}

	oldRoot := tree.get(tree.root)
	if err := validateBNode(oldRoot); err != nil {
		return fmt.Errorf("read root page: %w", err)
	}

	obsolete := make([]uint64, 0, 4)
	updatedRoot, err := tree.insertNode(oldRoot, key, value, &obsolete)
	if err != nil {
		return err
	}
	obsolete = append(obsolete, tree.root)

	rootParts, err := nodeSplit3(updatedRoot)
	if err != nil {
		return err
	}

	var newRootID uint64
	if len(rootParts) == 1 {
		newRootID = tree.new(rootParts[0])
	} else {
		newRoot := BNode(make([]byte, pageSize))
		newRoot.setHeader(nodeTypeInternal, uint16(len(rootParts)))
		for i, part := range rootParts {
			childPageID := tree.new(part)
			if err := nodeAppendKV(newRoot, uint16(i), childPageID, part.getKey(0), nil); err != nil {
				return err
			}
		}
		newRootID = tree.new(newRoot)
	}

	tree.root = newRootID
	for _, pageID := range obsolete {
		tree.del(pageID)
	}
	return nil
}

func (tree *pageTree) delete(key []byte) (bool, error) {
	if len(key) == 0 {
		return false, fmt.Errorf("key is empty")
	}
	if len(key) > maxKeySize {
		return false, fmt.Errorf("key exceeds max size")
	}
	if tree.root == 0 {
		return false, nil
	}
	if err := tree.validateCallbacks(); err != nil {
		return false, err
	}

	oldRootID := tree.root
	oldRoot := tree.get(oldRootID)
	if err := validateBNode(oldRoot); err != nil {
		return false, fmt.Errorf("read root page: %w", err)
	}

	obsolete := make([]uint64, 0, 4)
	updatedRoot, deleted, err := tree.deleteNode(oldRoot, key, &obsolete)
	if err != nil || !deleted {
		return deleted, err
	}
	obsolete = append(obsolete, oldRootID)

	switch {
	case updatedRoot.btype() == nodeTypeLeaf && updatedRoot.nkeys() == 1:
		tree.root = 0
	case updatedRoot.btype() == nodeTypeInternal && updatedRoot.nkeys() == 0:
		tree.root = 0
	case updatedRoot.btype() == nodeTypeInternal && updatedRoot.nkeys() == 1:
		tree.root = updatedRoot.getPtr(0)
	default:
		tree.root = tree.new(updatedRoot)
	}
	for _, pageID := range obsolete {
		tree.del(pageID)
	}
	return true, nil
}

func (tree *pageTree) deleteNode(
	node BNode,
	key []byte,
	obsolete *[]uint64,
) (BNode, bool, error) {
	idx, err := nodeLookupLE(node, key)
	if err != nil {
		return nil, false, err
	}
	if node.btype() == nodeTypeLeaf {
		if !bytes.Equal(node.getKey(idx), key) {
			return node, false, nil
		}
		updated, err := leafDeleteEncoded(node, idx)
		return updated, true, err
	}

	childPageID := node.getPtr(idx)
	child := tree.get(childPageID)
	if err := validateBNode(child); err != nil {
		return nil, false, fmt.Errorf("read child page %d: %w", childPageID, err)
	}
	updatedChild, deleted, err := tree.deleteNode(child, key, obsolete)
	if err != nil || !deleted {
		return node, deleted, err
	}
	*obsolete = append(*obsolete, childPageID)

	if updatedChild.nkeys() == 0 || int(updatedChild.nbytes()) <= pageSize/4 {
		if idx > 0 {
			leftID := node.getPtr(idx - 1)
			left := tree.get(leftID)
			if merged, ok, err := mergeEncodedNodes(left, updatedChild); err != nil {
				return nil, false, err
			} else if ok {
				*obsolete = append(*obsolete, leftID)
				return tree.replaceChildrenAfterDelete(node, idx-1, 2, merged)
			}
		}
		if idx+1 < node.nkeys() {
			rightID := node.getPtr(idx + 1)
			right := tree.get(rightID)
			if merged, ok, err := mergeEncodedNodes(updatedChild, right); err != nil {
				return nil, false, err
			} else if ok {
				*obsolete = append(*obsolete, rightID)
				return tree.replaceChildrenAfterDelete(node, idx, 2, merged)
			}
		}
	}

	if updatedChild.nkeys() == 0 {
		return removeChildEncoded(node, idx)
	}
	return tree.replaceChildrenAfterDelete(node, idx, 1, updatedChild)
}

func (tree *pageTree) replaceChildrenAfterDelete(
	parent BNode,
	idx, removeCount uint16,
	child BNode,
) (BNode, bool, error) {
	updatedCount := parent.nkeys() - removeCount + 1
	updated := BNode(make([]byte, pageSize))
	updated.setHeader(nodeTypeInternal, updatedCount)
	if err := nodeAppendRange(updated, parent, 0, 0, idx); err != nil {
		return nil, false, err
	}
	childPageID := tree.new(child)
	if err := nodeAppendKV(updated, idx, childPageID, child.getKey(0), nil); err != nil {
		return nil, false, err
	}
	tailStart := idx + removeCount
	if err := nodeAppendRange(
		updated,
		parent,
		idx+1,
		tailStart,
		parent.nkeys()-tailStart,
	); err != nil {
		return nil, false, err
	}
	return updated, true, nil
}

func removeChildEncoded(parent BNode, idx uint16) (BNode, bool, error) {
	updated := BNode(make([]byte, pageSize))
	updated.setHeader(nodeTypeInternal, parent.nkeys()-1)
	if err := nodeAppendRange(updated, parent, 0, 0, idx); err != nil {
		return nil, false, err
	}
	if err := nodeAppendRange(
		updated,
		parent,
		idx,
		idx+1,
		parent.nkeys()-idx-1,
	); err != nil {
		return nil, false, err
	}
	return updated, true, nil
}

func leafDeleteEncoded(old BNode, idx uint16) (BNode, error) {
	updated := BNode(make([]byte, pageSize))
	updated.setHeader(nodeTypeLeaf, old.nkeys()-1)
	if err := nodeAppendRange(updated, old, 0, 0, idx); err != nil {
		return nil, err
	}
	if err := nodeAppendRange(updated, old, idx, idx+1, old.nkeys()-idx-1); err != nil {
		return nil, err
	}
	return updated, nil
}

func mergeEncodedNodes(left, right BNode) (BNode, bool, error) {
	if left.btype() != right.btype() {
		return nil, false, fmt.Errorf("cannot merge different node types")
	}
	keyCount := left.nkeys() + right.nkeys()
	recordBytes := int(left.getOffset(left.nkeys())) + int(right.getOffset(right.nkeys()))
	mergedSize := pageHeaderSize + int(keyCount)*(pagePtrSize+pageOffsetSize) + recordBytes
	if mergedSize > pageSize {
		return nil, false, nil
	}

	merged := BNode(make([]byte, pageSize))
	merged.setHeader(left.btype(), keyCount)
	if err := nodeAppendRange(merged, left, 0, 0, left.nkeys()); err != nil {
		return nil, false, err
	}
	if err := nodeAppendRange(merged, right, left.nkeys(), 0, right.nkeys()); err != nil {
		return nil, false, err
	}
	return merged, true, nil
}

func (tree *pageTree) insertNode(node BNode, key, value []byte, obsolete *[]uint64) (BNode, error) {
	idx, err := nodeLookupLE(node, key)
	if err != nil {
		return nil, err
	}

	if node.btype() == nodeTypeLeaf {
		if bytes.Equal(node.getKey(idx), key) {
			return leafUpdateEncoded(node, idx, key, value)
		}
		return leafInsertEncoded(node, idx+1, key, value)
	}

	childPageID := node.getPtr(idx)
	child := tree.get(childPageID)
	if err := validateBNode(child); err != nil {
		return nil, fmt.Errorf("read child page %d: %w", childPageID, err)
	}
	updatedChild, err := tree.insertNode(child, key, value, obsolete)
	if err != nil {
		return nil, err
	}
	childParts, err := nodeSplit3(updatedChild)
	if err != nil {
		return nil, err
	}
	*obsolete = append(*obsolete, childPageID)
	return tree.replaceChild(node, idx, childParts)
}

func (tree *pageTree) replaceChild(parent BNode, idx uint16, children []BNode) (BNode, error) {
	keyCount := int(parent.nkeys()) + len(children) - 1
	updated := BNode(make([]byte, 2*pageSize))
	updated.setHeader(nodeTypeInternal, uint16(keyCount))

	if err := nodeAppendRange(updated, parent, 0, 0, idx); err != nil {
		return nil, err
	}
	for i, child := range children {
		pageID := tree.new(child)
		if err := nodeAppendKV(updated, idx+uint16(i), pageID, child.getKey(0), nil); err != nil {
			return nil, err
		}
	}
	tailCount := parent.nkeys() - idx - 1
	if err := nodeAppendRange(
		updated,
		parent,
		idx+uint16(len(children)),
		idx+1,
		tailCount,
	); err != nil {
		return nil, err
	}
	return updated, nil
}

func (tree *pageTree) validateCallbacks() error {
	if tree.get == nil || tree.new == nil || tree.del == nil {
		return fmt.Errorf("page callbacks are not configured")
	}
	return nil
}

func validateTreeEntry(key, value []byte) error {
	if len(key) == 0 {
		return fmt.Errorf("key is empty")
	}
	if len(key) > maxKeySize {
		return fmt.Errorf("key exceeds max size")
	}
	if len(value) > maxValueSize {
		return fmt.Errorf("value exceeds max size")
	}
	return nil
}

func nodeLookupLE(node BNode, key []byte) (uint16, error) {
	keyCount := int(node.nkeys())
	idx := sort.Search(keyCount, func(i int) bool {
		return bytes.Compare(node.getKey(uint16(i)), key) > 0
	}) - 1
	if idx < 0 {
		return 0, fmt.Errorf("node does not contain lower bound for key")
	}
	return uint16(idx), nil
}

func leafInsertEncoded(old BNode, idx uint16, key, value []byte) (BNode, error) {
	updated := BNode(make([]byte, 2*pageSize))
	updated.setHeader(nodeTypeLeaf, old.nkeys()+1)
	if err := nodeAppendRange(updated, old, 0, 0, idx); err != nil {
		return nil, err
	}
	if err := nodeAppendKV(updated, idx, 0, key, value); err != nil {
		return nil, err
	}
	if err := nodeAppendRange(updated, old, idx+1, idx, old.nkeys()-idx); err != nil {
		return nil, err
	}
	return updated, nil
}

func leafUpdateEncoded(old BNode, idx uint16, key, value []byte) (BNode, error) {
	updated := BNode(make([]byte, 2*pageSize))
	updated.setHeader(nodeTypeLeaf, old.nkeys())
	if err := nodeAppendRange(updated, old, 0, 0, idx); err != nil {
		return nil, err
	}
	if err := nodeAppendKV(updated, idx, 0, key, value); err != nil {
		return nil, err
	}
	if err := nodeAppendRange(updated, old, idx+1, idx+1, old.nkeys()-idx-1); err != nil {
		return nil, err
	}
	return updated, nil
}

func nodeSplit3(node BNode) ([]BNode, error) {
	if int(node.nbytes()) <= pageSize {
		fixed, err := copyNodeRange(node, 0, node.nkeys())
		if err != nil {
			return nil, err
		}
		return []BNode{fixed}, nil
	}

	parts := make([]BNode, 0, 3)
	for start := uint16(0); start < node.nkeys(); {
		end := start + 1
		for end < node.nkeys() && nodeRangeSize(node, start, end-start+1) <= pageSize {
			end++
		}
		count := end - start
		if nodeRangeSize(node, start, count) > pageSize {
			return nil, fmt.Errorf("encoded entry does not fit in page")
		}
		part, err := copyNodeRange(node, start, count)
		if err != nil {
			return nil, err
		}
		parts = append(parts, part)
		if len(parts) > 3 {
			return nil, fmt.Errorf("oversized node requires more than three pages")
		}
		start = end
	}
	return parts, nil
}

func nodeRangeSize(node BNode, start, count uint16) int {
	recordBytes := int(node.getOffset(start+count) - node.getOffset(start))
	return pageHeaderSize + int(count)*(pagePtrSize+pageOffsetSize) + recordBytes
}

func copyNodeRange(node BNode, start, count uint16) (BNode, error) {
	copyNode := BNode(make([]byte, pageSize))
	copyNode.setHeader(node.btype(), count)
	if err := nodeAppendRange(copyNode, node, 0, start, count); err != nil {
		return nil, err
	}
	if err := validateBNode(copyNode); err != nil {
		return nil, err
	}
	return copyNode, nil
}
