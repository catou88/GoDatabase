package btree

import (
	"sort"

	"godatabase/internal/trace"
)

const snapshotKeys = 16

type memoryTrace struct {
	sink trace.Sink
	ids  map[*node]uint64
	op   trace.Operation
}

// SetTrace starts a new run-local ID namespace. Attach after setup and serialize
// calls with operations, just as for the other Memory methods.
func (m *Memory) SetTrace(sink trace.Sink) {
	m.tree.observer = memoryTrace{sink: sink}
}

func (t *tree) nodeID(n *node) uint64 {
	if t.observer.ids == nil {
		t.observer.ids = make(map[*node]uint64)
	}
	if id := t.observer.ids[n]; id != 0 {
		return id
	}
	id := uint64(len(t.observer.ids) + 1)
	t.observer.ids[n] = id
	return id
}

func (t *tree) emitNode(n *node, kind trace.EventType, key, detail string) {
	sink := t.observer.sink
	if sink == nil || !sink.Enabled() || n == nil {
		return
	}
	event := trace.Event{Type: kind, Operation: t.observer.op,
		Structure: "btree-memory", Layer: "backing", NodeID: t.nodeID(n), Key: key, Detail: detail}
	event.Keys = append([]string(nil), n.keys[:min(len(n.keys), snapshotKeys)]...)
	for _, child := range n.children[:min(len(n.children), snapshotKeys+1)] {
		event.Children = append(event.Children, t.nodeID(child))
	}
	sink.Emit(event)
}

func (t *tree) search(n *node, key string) (int, bool) {
	if t.observer.sink == nil || !t.observer.sink.Enabled() {
		return n.search(key)
	}
	idx := sort.Search(len(n.keys), func(i int) bool {
		t.emitNode(n, trace.Comparison, key, "lower bound: key <= "+n.keys[i])
		return n.keys[i] >= key
	})
	return idx, idx < len(n.keys) && n.keys[idx] == key
}

func (t *tree) childIndex(n *node, key string) int {
	if t.observer.sink == nil || !t.observer.sink.Enabled() {
		return n.childIndex(key)
	}
	return sort.Search(len(n.keys), func(i int) bool {
		t.emitNode(n, trace.Comparison, key, "child selection: key < "+n.keys[i])
		return key < n.keys[i]
	})
}

func (t *tree) minimumKey(n *node) string {
	for !n.leaf {
		t.emitNode(n, trace.Traversal, "", "seek minimum for separator")
		n = n.children[0]
	}
	t.emitNode(n, trace.Traversal, "", "read minimum for separator")
	return n.keys[0]
}

// SetTrace attaches an optional observer. Sinks execute under the store lock
// and must not call back into this KV. nil detaches the observer.
func (kv *KV) SetTrace(sink trace.Sink) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	kv.traceSink = sink
}

func (kv *KV) emitPage(id uint64, page BNode, kind trace.EventType, detail string) {
	if kv.traceSink == nil || !kv.traceSink.Enabled() {
		return
	}
	event := trace.Event{Type: kind, Operation: kv.traceOperation,
		Structure: "btree-durable", Layer: "backing", PageID: id, Detail: detail}
	// Preserve the normal validation/error path for invalid or missing pages.
	if validateBNode(page) == nil {
		for i := uint16(0); i < page.nkeys() && i < snapshotKeys; i++ {
			event.Keys = append(event.Keys, string(page.getKey(i)))
		}
		if page.btype() == nodeTypeInternal {
			for i := uint16(0); i < page.nkeys() && i < snapshotKeys+1; i++ {
				event.Children = append(event.Children, page.getPtr(i))
			}
		}
	}
	kv.traceSink.Emit(event)
}
