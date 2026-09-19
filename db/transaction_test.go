package db_test

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"godatabase/db"
)

func TestTransactionCommitPublishesAllWrites(t *testing.T) {
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
	if _, found, err := database.Get("first"); err != nil || found {
		t.Fatalf("uncommitted Get() = (%v, %v), want missing", found, err)
	}
	if value, found, err := tx.Get("first"); err != nil || !found || value != "one" {
		t.Fatalf("transaction Get() = (%q, %v, %v), want buffered value", value, found, err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	for key, want := range map[string]string{"first": "one", "second": "two"} {
		if value, found, err := database.Get(key); err != nil || !found || value != want {
			t.Errorf("Get(%q) = (%q, %v, %v), want %q", key, value, found, err, want)
		}
	}
}

func TestTransactionRollbackDiscardsWrites(t *testing.T) {
	database := db.New()
	if err := database.Set("existing", "old"); err != nil {
		t.Fatal(err)
	}
	tx, err := database.Begin(db.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set("existing", "new"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set("created", "value"); err != nil {
		t.Fatal(err)
	}
	if deleted, err := tx.Delete("existing"); err != nil || !deleted {
		t.Fatalf("Delete() = (%v, %v)", deleted, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if value, found, err := database.Get("existing"); err != nil || !found || value != "old" {
		t.Fatalf("existing after rollback = (%q, %v, %v), want old", value, found, err)
	}
	if _, found, err := database.Get("created"); err != nil || found {
		t.Fatalf("created after rollback = (found %v, error %v), want missing", found, err)
	}
}

func TestDurableTransactionPersistsAsOneCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := database.Begin(db.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set("alpha", "one"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set("beta", "two"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	database, err = db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	for key, want := range map[string]string{"alpha": "one", "beta": "two"} {
		if value, found, err := database.Get(key); err != nil || !found || value != want {
			t.Errorf("reopened Get(%q) = (%q, %v, %v), want %q", key, value, found, err, want)
		}
	}
}

func TestReadOnlyTransactionRejectsWrites(t *testing.T) {
	database := db.New()
	tx, err := database.Begin(db.TxOptions{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set("key", "value"); !errors.Is(err, db.ErrReadOnlyTransaction) {
		t.Fatalf("read-only Set() error = %v, want %v", err, db.ErrReadOnlyTransaction)
	}
	if deleted, err := tx.Delete("key"); !errors.Is(err, db.ErrReadOnlyTransaction) || deleted {
		t.Fatalf("read-only Delete() = (%v, %v)", deleted, err)
	}
	if err := tx.Commit(); !errors.Is(err, db.ErrReadOnlyTransaction) {
		t.Fatalf("read-only Commit() error = %v, want %v", err, db.ErrReadOnlyTransaction)
	}
}

func TestFailedTransactionCommitPreservesCommittedData(t *testing.T) {
	database, err := db.Open(filepath.Join(t.TempDir(), "database.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	if err := database.Set("stable", "value"); err != nil {
		t.Fatal(err)
	}
	tx, err := database.Begin(db.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set("new", "value"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(strings.Repeat("k", 1001), "too large"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err == nil {
		t.Fatal("Commit() error = nil, want oversized-key error")
	}
	if value, found, err := database.Get("stable"); err != nil || !found || value != "value" {
		t.Fatalf("stable after failed commit = (%q, %v, %v), want value", value, found, err)
	}
	if _, found, err := database.Get("new"); err != nil || found {
		t.Fatalf("new after failed commit = (found %v, error %v), want missing", found, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
}

func TestTransactionOperationsAfterCommitAreClosed(t *testing.T) {
	database := db.New()
	tx, err := database.Begin(db.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set("key", "value"); !errors.Is(err, db.ErrTransactionClosed) {
		t.Fatalf("Set after commit error = %v, want %v", err, db.ErrTransactionClosed)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
}

func TestTransactionMaintainsTableAndIndexTogether(t *testing.T) {
	database := db.New()
	table, err := database.CreateTable(testSchema())
	if err != nil {
		t.Fatal(err)
	}
	if err := table.CreateIndex(db.Index{Name: "name_idx", Column: "name"}); err != nil {
		t.Fatal(err)
	}
	tx, err := database.Begin(db.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []map[string]any{
		{"id": int64(1), "name": "Ada", "active": true},
		{"id": int64(2), "name": "Grace", "active": true},
	} {
		if err := tx.Insert(table, row); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	rows, err := table.FindByIndex("name_idx", "Ada")
	if err != nil || len(rows) != 1 || rows[0]["id"] != int64(1) {
		t.Fatalf("indexed rows after commit = (%v, %v), want Ada row", rows, err)
	}

	tx, err = database.Begin(db.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Update(table, map[string]any{"id": int64(1), "name": "Grace", "active": false}); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.DeleteRow(table, int64(2)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	assertIndexedRowsMatchScan(t, table, "name_idx", "Ada")
	assertIndexedRowsMatchScan(t, table, "name_idx", "Grace")
}

func TestTransactionalTableChangesSurviveReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	table, err := database.CreateTable(testSchema())
	if err != nil {
		t.Fatal(err)
	}
	if err := table.CreateIndex(db.Index{Name: "name_idx", Column: "name"}); err != nil {
		t.Fatal(err)
	}
	tx, err := database.Begin(db.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Insert(table, map[string]any{"id": int64(4), "name": "Lin", "active": true}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	database, err = db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	table, err = database.OpenTable("users")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := table.FindByIndex("name_idx", "Lin")
	if err != nil || len(rows) != 1 || rows[0]["id"] != int64(4) {
		t.Fatalf("reopened indexed rows = (%v, %v), want Lin row", rows, err)
	}
}

func TestFailedTransactionalTableCommitLeavesRowsAndIndexesUnchanged(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = database.Close() }()
	table, err := database.CreateTable(testSchema())
	if err != nil {
		t.Fatal(err)
	}
	if err := table.CreateIndex(db.Index{Name: "name_idx", Column: "name"}); err != nil {
		t.Fatal(err)
	}
	tx, err := database.Begin(db.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Insert(table, map[string]any{"id": int64(5), "name": "temporary", "active": true}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(strings.Repeat("k", 1001), "invalid"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(); err == nil {
		t.Fatal("Commit() error = nil, want oversized-key error")
	}
	if _, found, err := table.Get(int64(5)); err != nil || found {
		t.Fatalf("row after failed commit = (found %v, error %v), want missing", found, err)
	}
	rows, err := table.FindByIndex("name_idx", "temporary")
	if err != nil || len(rows) != 0 {
		t.Fatalf("index after failed commit = (%v, %v), want empty", rows, err)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
}

func TestTransactionalUpdateRejectsUniqueIndexConflict(t *testing.T) {
	database := db.New()
	table, err := database.CreateTable(testSchema())
	if err != nil {
		t.Fatal(err)
	}
	if err := table.CreateIndex(db.Index{Name: "active_idx", Column: "active", Unique: true}); err != nil {
		t.Fatal(err)
	}
	if err := table.Insert(map[string]any{"id": int64(1), "name": "Ada", "active": true}); err != nil {
		t.Fatal(err)
	}
	if err := table.Insert(map[string]any{"id": int64(2), "name": "Grace", "active": false}); err != nil {
		t.Fatal(err)
	}
	tx, err := database.Begin(db.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Update(table, map[string]any{"id": int64(2), "name": "Grace", "active": true}); !errors.Is(err, db.ErrDuplicateIndexed) {
		t.Fatalf("Update() error = %v, want %v", err, db.ErrDuplicateIndexed)
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
}
