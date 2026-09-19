package engine

import "testing"

func TestDurableStoreRoundTrip(t *testing.T) {
	path := t.TempDir() + "/database"
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set([]byte("key"), []byte("value")); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := store.Close(); err != nil {
			t.Errorf("close durable store: %v", err)
		}
	}()
	value, found, err := store.Get([]byte("key"))
	if err != nil || !found || string(value) != "value" {
		t.Fatalf("Get() = (%q, %v, %v)", value, found, err)
	}
}
