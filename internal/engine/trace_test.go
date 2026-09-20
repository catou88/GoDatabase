package engine

import (
	"path/filepath"
	"testing"

	"godatabase/internal/trace"
)

func TestDurableForwardsTrace(t *testing.T) {
	d, err := Open(filepath.Join(t.TempDir(), "tree.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := d.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := d.Set([]byte("key"), []byte("value")); err != nil {
		t.Fatal(err)
	}
	r := trace.NewRecorder()
	d.SetTrace(r)
	if _, found, err := d.Get([]byte("key")); err != nil || !found {
		t.Fatalf("get: %v, %v", found, err)
	}
	if events := r.Events(); len(events) != 1 || events[0].PageID == 0 || events[0].Type != trace.Traversal {
		t.Fatalf("expected actual page read through engine: %+v", events)
	}
	d.SetTrace(nil)
	if _, _, err := d.Get([]byte("key")); err != nil {
		t.Fatal(err)
	}
	if len(r.Events()) != 1 {
		t.Fatal("detach was not forwarded")
	}
}
