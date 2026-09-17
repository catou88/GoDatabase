package db

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDurableDatabasePersistsPublicOperations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	database, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	for key, value := range map[string]string{
		"alpha":   "one",
		"bravo":   "two",
		"charlie": "three",
		"delta":   "four",
	} {
		if err := database.Set(key, value); err != nil {
			t.Fatalf("Set(%q) error = %v", key, err)
		}
	}
	if err := database.Set("bravo", "updated"); err != nil {
		t.Fatalf("Set(overwrite) error = %v", err)
	}
	deleted, err := database.Delete("delta")
	if err != nil || !deleted {
		t.Fatalf("Delete(delta) = %v, %v; want true, nil", deleted, err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	database, err = Open(path)
	if err != nil {
		t.Fatalf("Open(reopen) error = %v", err)
	}
	defer func() {
		if err := database.Close(); err != nil {
			t.Errorf("Close(reopened) error = %v", err)
		}
	}()

	value, found, err := database.Get("bravo")
	if err != nil || !found || value != "updated" {
		t.Fatalf("Get(bravo) = (%q, %v, %v), want (%q, true, nil)", value, found, err, "updated")
	}
	if _, found, err := database.Get("delta"); err != nil || found {
		t.Fatalf("Get(delta) = found %v, error %v; want false, nil", found, err)
	}

	got, err := database.Range("alpha", "charlie")
	if err != nil {
		t.Fatalf("Range() error = %v", err)
	}
	want := []Item{
		{Key: "alpha", Value: "one"},
		{Key: "bravo", Value: "updated"},
		{Key: "charlie", Value: "three"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Range() = %#v, want %#v", got, want)
	}
}

func TestPublicDatabasePreservesInputValidation(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "database.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer func() { _ = database.Close() }()

	if err := database.Set("", "value"); !errors.Is(err, ErrEmptyKey) {
		t.Fatalf("Set(empty key) error = %v, want %v", err, ErrEmptyKey)
	}
	if value, found, err := database.Get(""); err != nil || found || value != "" {
		t.Fatalf("Get(empty key) = (%q, %v, %v), want (\"\", false, nil)", value, found, err)
	}
	if deleted, err := database.Delete(""); err != nil || deleted {
		t.Fatalf("Delete(empty key) = (%v, %v), want (false, nil)", deleted, err)
	}
	if err := database.Set("alpha", "one"); err != nil {
		t.Fatalf("Set(alpha) error = %v", err)
	}
	if items, err := database.Range("alpha", ""); err != nil || len(items) != 0 {
		t.Fatalf("Range(alpha, empty) = (%v, %v), want empty, nil", items, err)
	}
}

func TestDatabaseOperationsAfterCloseReturnErrClosed(t *testing.T) {
	database, err := Open(filepath.Join(t.TempDir(), "database.db"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("second Close() error = %v", err)
	}

	if err := database.Set("key", "value"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Set() error = %v, want %v", err, ErrClosed)
	}
	if _, _, err := database.Get("key"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Get() error = %v, want %v", err, ErrClosed)
	}
	if _, err := database.Delete("key"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Delete() error = %v, want %v", err, ErrClosed)
	}
	if _, err := database.Range("a", "z"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Range() error = %v, want %v", err, ErrClosed)
	}
}

func TestNewRemainsInMemory(t *testing.T) {
	database := New()
	if database.durable != nil || database.data == nil {
		t.Fatal("New() did not create an in-memory database")
	}
	if err := database.Set("key", "value"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	value, found, err := database.Get("key")
	if err != nil || !found || value != "value" {
		t.Fatalf("Get() = (%q, %v, %v), want (%q, true, nil)", value, found, err, "value")
	}
}
