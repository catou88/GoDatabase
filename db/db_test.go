package db

import "testing"

func TestDatabaseSetAndGet(t *testing.T) {
	db := New()

	if err := db.Set("alpha", "one"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	value, ok := db.Get("alpha")
	if !ok {
		t.Fatal("expected alpha to exist")
	}
	if value != "one" {
		t.Fatalf("expected alpha to be one, got %q", value)
	}
}

func TestDatabaseOverwrite(t *testing.T) {
	db := New()

	if err := db.Set("alpha", "one"); err != nil {
		t.Fatalf("first Set returned error: %v", err)
	}
	if err := db.Set("alpha", "two"); err != nil {
		t.Fatalf("overwrite Set returned error: %v", err)
	}

	value, ok := db.Get("alpha")
	if !ok {
		t.Fatal("expected alpha to exist after overwrite")
	}
	if value != "two" {
		t.Fatalf("expected alpha to be two, got %q", value)
	}
}

func TestDatabaseDelete(t *testing.T) {
	db := New()

	if err := db.Set("alpha", "one"); err != nil {
		t.Fatalf("Set returned error: %v", err)
	}

	if !db.Delete("alpha") {
		t.Fatal("expected alpha to be deleted")
	}

	if _, ok := db.Get("alpha"); ok {
		t.Fatal("alpha should not exist after delete")
	}
}

func TestDatabaseRange(t *testing.T) {
	db := New()
	for _, pair := range []struct{ key, value string }{
		{"b", "two"},
		{"a", "one"},
		{"c", "three"},
	} {
		if err := db.Set(pair.key, pair.value); err != nil {
			t.Fatalf("Set returned error: %v", err)
		}
	}

	values := db.Range("a", "c")
	if len(values) != 3 {
		t.Fatalf("expected 3 values in range, got %d", len(values))
	}

	if values[0].Key != "a" || values[0].Value != "one" {
		t.Fatalf("first range item mismatch: %#v", values[0])
	}
	if values[1].Key != "b" || values[1].Value != "two" {
		t.Fatalf("second range item mismatch: %#v", values[1])
	}
	if values[2].Key != "c" || values[2].Value != "three" {
		t.Fatalf("third range item mismatch: %#v", values[2])
	}
}
