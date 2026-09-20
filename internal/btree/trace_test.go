package btree

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"godatabase/internal/structures"
	"godatabase/internal/trace"
)

func TestMemoryTraceActualPath(t *testing.T) {
	m := NewMemory()
	for i := 0; i < 100; i++ {
		if err := m.Set([]byte(fmt.Sprintf("k%03d", i)), []byte("v")); err != nil {
			t.Fatal(err)
		}
	}
	key := "k057"
	var path []*node
	for n := m.tree.root; ; n = n.child(key) {
		path = append(path, n)
		if n.leaf {
			break
		}
	}
	r := trace.NewRecorder()
	m.SetTrace(r)
	if _, found, err := m.Get([]byte(key)); err != nil || !found {
		t.Fatalf("get: %v, %v", found, err)
	}
	var visited []uint64
	comparisons := 0
	for _, event := range r.Events() {
		if event.Type == trace.Comparison {
			comparisons++
		}
		if event.Type != trace.Traversal {
			continue
		}
		if len(visited) >= len(path) {
			t.Fatal("extra traversal")
		}
		n := path[len(visited)]
		if event.NodeID != m.tree.observer.ids[n] || event.NodeID == 0 || event.PageID != 0 {
			t.Fatalf("invalid node identity: %+v", event)
		}
		if !reflect.DeepEqual(event.Keys, n.keys) {
			t.Fatalf("snapshot %v, want %v", event.Keys, n.keys)
		}
		for i, id := range event.Children {
			if id != m.tree.observer.ids[n.children[i]] {
				t.Fatal("incorrect child ID")
			}
		}
		visited = append(visited, event.NodeID)
	}
	if len(visited) != len(path) || comparisons == 0 {
		t.Fatalf("visits %d/%d, comparisons %d", len(visited), len(path), comparisons)
	}
	before := r.Events()
	if _, _, err := m.Get([]byte(key)); err != nil {
		t.Fatal(err)
	}
	after := r.Events()[len(before):]
	for i := range after {
		after[i].Sequence = before[i].Sequence
	}
	if !reflect.DeepEqual(before, after) {
		t.Fatal("IDs changed on repeated lookup")
	}
}

func TestDurableTraceActualPages(t *testing.T) {
	kv, err := Open(filepath.Join(t.TempDir(), "tree.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := kv.Close(); err != nil {
			t.Error(err)
		}
	})
	for i := 0; i < 100; i++ {
		if err := kv.Set([]byte(fmt.Sprintf("k%03d", i)), []byte(strings.Repeat("v", 100))); err != nil {
			t.Fatal(err)
		}
	}
	var want []uint64
	key := []byte("k057")
	for id := kv.tree.root; ; {
		want = append(want, id)
		n := kv.pages[id]
		if n.btype() == nodeTypeLeaf {
			break
		}
		idx, err := nodeLookupLE(n, key)
		if err != nil {
			t.Fatal(err)
		}
		id = n.getPtr(idx)
	}
	if len(want) < 2 {
		t.Fatal("test requires multiple levels")
	}
	r := trace.NewRecorder()
	kv.SetTrace(r)
	if _, found, err := kv.Get(key); err != nil || !found {
		t.Fatalf("get: %v, %v", found, err)
	}
	var got []uint64
	for _, event := range r.Events() {
		if event.Type != trace.Traversal {
			continue
		}
		got = append(got, event.PageID)
		page, ok := kv.pages[event.PageID]
		if !ok || event.NodeID != 0 {
			t.Fatalf("unknown page: %+v", event)
		}
		if len(event.Keys) != min(int(page.nkeys()), 16) {
			t.Fatal("incorrect snapshot length")
		}
		for i, key := range event.Keys {
			if key != string(page.getKey(uint16(i))) {
				t.Fatal("incorrect page key")
			}
		}
		for i, child := range event.Children {
			if child != page.getPtr(uint16(i)) {
				t.Fatal("incorrect page child")
			}
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("page visits %v, want %v", got, want)
	}
}

type observedKV interface {
	structures.KV
	SetTrace(trace.Sink)
	Close() error
}

type disabledTrace struct{}

func (disabledTrace) Enabled() bool    { return false }
func (disabledTrace) Emit(trace.Event) { panic("disabled sink received an event") }

func TestTreeTraceDeterministicAndObservational(t *testing.T) {
	for _, durable := range []bool{false, true} {
		t.Run(fmt.Sprintf("durable=%v", durable), func(t *testing.T) {
			run := func(sink trace.Sink) []structures.Entry {
				var store observedKV = NewMemory()
				if durable {
					kv, err := Open(filepath.Join(t.TempDir(), "tree.db"))
					if err != nil {
						t.Fatal(err)
					}
					store = kv
				}
				t.Cleanup(func() {
					if err := store.Close(); err != nil {
						t.Error(err)
					}
				})
				for i := 0; i < 40; i++ {
					if err := store.Set([]byte(fmt.Sprintf("k%03d", i)), []byte(strings.Repeat("v", 180))); err != nil {
						t.Fatal(err)
					}
				}
				store.SetTrace(sink)
				for i := 40; i < 60; i++ {
					if err := store.Set([]byte(fmt.Sprintf("k%03d", i)), []byte(strings.Repeat("x", 180))); err != nil {
						t.Fatal(err)
					}
				}
				for i := 0; i < 50; i++ {
					if found, err := store.Delete([]byte(fmt.Sprintf("k%03d", i))); err != nil || !found {
						t.Fatalf("delete: %v, %v", found, err)
					}
				}
				if _, found, err := store.Get([]byte("missing")); err != nil || found {
					t.Fatalf("missing lookup: %v, %v", found, err)
				}
				entries, err := store.Range([]byte("k"), []byte("z"))
				if err != nil {
					t.Fatal(err)
				}
				store.SetTrace(nil)
				if _, _, err := store.Get([]byte("k055")); err != nil {
					t.Fatal(err)
				}
				return entries
			}
			a, b := trace.NewRecorder(), trace.NewRecorder()
			want := run(nil)
			for _, sink := range []trace.Sink{disabledTrace{}, a, b} {
				if got := run(sink); !reflect.DeepEqual(got, want) {
					t.Fatal("tracing changed logical results")
				}
			}
			if !reflect.DeepEqual(a.Events(), b.Events()) {
				t.Fatal("trace is not deterministic")
			}
			mutations := 0
			for _, event := range a.Events() {
				if len(event.Keys) > 16 || len(event.Children) > 17 {
					t.Fatal("unbounded snapshot")
				}
				if event.Layer != "backing" || (event.NodeID == 0 && event.PageID == 0) {
					t.Fatalf("missing event identity: %+v", event)
				}
				if event.Type == "mutation" {
					mutations++
				}
			}
			if mutations == 0 {
				t.Fatal("no actual mutation events")
			}
		})
	}
}

func TestMemoryTraceSnapshotBounds(t *testing.T) {
	m := NewMemory()
	m.tree.maxKeys = 40
	for i := 0; i < 1000; i++ {
		m.tree.set(fmt.Sprintf("k%04d", i), "v")
	}
	r := trace.NewRecorder()
	m.SetTrace(r)
	if _, _, err := m.Get([]byte("k0500")); err != nil {
		t.Fatal(err)
	}
	if len(r.Events()) == 0 {
		t.Fatal("no events")
	}
	for _, event := range r.Events() {
		if len(event.Keys) > 16 || len(event.Children) > 17 {
			t.Fatal("unbounded memory snapshot")
		}
	}
}
