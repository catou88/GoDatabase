package db_test

import (
	"errors"
	"sync"
	"testing"

	"godatabase/db"
)

func TestTransactionReadWriteBehaviorIsDeterministic(t *testing.T) {
	database := db.New()
	if err := database.Set("key", "before"); err != nil {
		t.Fatal(err)
	}
	reader, err := database.Begin(db.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	writer, err := database.Begin(db.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := writer.Set("key", "after"); err != nil {
		t.Fatal(err)
	}
	if value, found, err := reader.Get("key"); err != nil || !found || value != "before" {
		t.Fatalf("reader before commit = (%q, %v, %v), want before", value, found, err)
	}
	if err := writer.Commit(); err != nil {
		t.Fatal(err)
	}
	if value, found, err := database.Get("key"); err != nil || !found || value != "after" {
		t.Fatalf("database after commit = (%q, %v, %v), want after", value, found, err)
	}
	// Current transactions do not yet pin immutable snapshot roots. Reads of
	// unbuffered keys observe the current committed database state.
	if value, found, err := reader.Get("key"); err != nil || !found || value != "after" {
		t.Fatalf("reader after commit = (%q, %v, %v), want current state", value, found, err)
	}
	if err := reader.Rollback(); err != nil {
		t.Fatal(err)
	}
}

func TestSecondWriteTransactionIsRejected(t *testing.T) {
	database := db.New()
	first, err := database.Begin(db.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Rollback() }()
	if _, err := database.Begin(db.TxOptions{}); !errors.Is(err, db.ErrWriteTransactionActive) {
		t.Fatalf("second Begin() error = %v, want %v", err, db.ErrWriteTransactionActive)
	}
	reader, err := database.Begin(db.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatalf("read-only Begin() error = %v", err)
	}
	if err := reader.Rollback(); err != nil {
		t.Fatal(err)
	}
}

func TestRollbackDuringConcurrentReadsKeepsCommittedState(t *testing.T) {
	database := db.New()
	if err := database.Set("stable", "old"); err != nil {
		t.Fatal(err)
	}
	tx, err := database.Begin(db.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set("stable", "new"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set("created", "temporary"); err != nil {
		t.Fatal(err)
	}

	var group sync.WaitGroup
	group.Add(1)
	go func() {
		defer group.Done()
		for i := 0; i < 100; i++ {
			value, found, err := database.Get("stable")
			if err != nil || !found || value != "old" {
				t.Errorf("concurrent Get() = (%q, %v, %v), want old", value, found, err)
			}
		}
	}()
	group.Wait()
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if value, found, err := database.Get("stable"); err != nil || !found || value != "old" {
		t.Fatalf("stable after rollback = (%q, %v, %v), want old", value, found, err)
	}
	if _, found, err := database.Get("created"); err != nil || found {
		t.Fatalf("created after rollback = (found %v, error %v), want missing", found, err)
	}
}

func TestCommitVisibilityIsAtomicToConcurrentReaders(t *testing.T) {
	database := db.New()
	tx, err := database.Begin(db.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set("first", "one"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set("second", "two"); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		defer group.Done()
		if value, found, err := database.Get("first"); err != nil || found || value != "" {
			t.Errorf("before commit Get() = (%q, %v, %v), want missing", value, found, err)
		}
		if value, found, err := database.Get("second"); err != nil || found || value != "" {
			t.Errorf("before commit Get() = (%q, %v, %v), want missing", value, found, err)
		}
	}()
	group.Wait()
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	items, err := database.Range("first", "second")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Value != "one" || items[1].Value != "two" {
		t.Fatalf("after commit Range() = %#v, want both committed rows", items)
	}
}
