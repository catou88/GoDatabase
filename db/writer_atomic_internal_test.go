package db

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func atomicTestTable(t *testing.T, d *Database) *Table {
	t.Helper()
	table, err := d.CreateTable(TableSchema{Name: "atomic", Columns: []Column{
		{Name: "id", Type: ColumnInt64, PrimaryKey: true},
		{Name: "value", Type: ColumnString},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return table
}

func atomicTestRow(id int64, value string) map[string]any {
	return map[string]any{"id": id, "value": value}
}

func TestWriterAdmissionAllDirectPaths(t *testing.T) {
	d := New()
	table := atomicTestTable(t, d)
	tx, err := d.Begin(TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	operations := map[string]func() error{
		"set":        func() error { return d.Set("key", "value") },
		"delete":     func() error { _, err := d.Delete(""); return err },
		"table":      func() error { _, err := d.CreateTable(TableSchema{}); return err },
		"insert":     func() error { return table.Insert(atomicTestRow(1, "a")) },
		"update":     func() error { return table.Update(atomicTestRow(1, "a")) },
		"delete row": func() error { _, err := table.Delete(int64(1)); return err },
		"index":      func() error { return table.CreateIndex(Index{Name: "value", Column: "value"}) },
	}
	for name, operation := range operations {
		t.Run(name, func(t *testing.T) {
			result := make(chan error, 1)
			go func() { result <- operation() }()
			if err := <-result; !errors.Is(err, ErrWriteTransactionActive) {
				t.Fatalf("error = %v", err)
			}
		})
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := table.Insert(atomicTestRow(1, "a")); err != nil {
		t.Fatal(err)
	}
}

func TestAtomicStaleHandleAndUniqueOwner(t *testing.T) {
	d := New()
	old := atomicTestTable(t, d)
	current, err := d.OpenTable("atomic")
	if err != nil {
		t.Fatal(err)
	}
	if err := current.CreateIndex(Index{Name: "value", Column: "value", Unique: true}); err != nil {
		t.Fatal(err)
	}
	for id, value := range []string{"a", "b"} {
		if err := old.Insert(atomicTestRow(int64(id), value)); err != nil {
			t.Fatal(err)
		}
	}
	if err := old.Update(atomicTestRow(0, "a")); err != nil {
		t.Fatalf("unchanged owner: %v", err)
	}
	if err := old.Update(atomicTestRow(0, "b")); !errors.Is(err, ErrDuplicateIndexed) {
		t.Fatalf("conflicting owner: %v", err)
	}
	if len(old.Indexes()) != 1 {
		t.Fatal("stale index definitions")
	}
	rows, err := old.FindByIndex("value", "a")
	if err != nil || len(rows) != 1 || rows[0]["id"] != int64(0) {
		t.Fatalf("lookup = %v, %v", rows, err)
	}
	if deleted, err := old.Delete(int64(0)); err != nil || !deleted {
		t.Fatalf("delete = %v, %v", deleted, err)
	}
	rows, err = current.FindByIndex("value", "a")
	if err != nil || len(rows) != 0 {
		t.Fatalf("deleted index = %v, %v", rows, err)
	}
}

func TestAtomicFailedBackfillAndMutation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atomic.db")
	d, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = d.Close() }()
	table := atomicTestTable(t, d)
	large := strings.Repeat("x", 1100)
	for id, value := range []string{"small", large} {
		if err := table.Insert(atomicTestRow(int64(id), value)); err != nil {
			t.Fatal(err)
		}
	}
	index := Index{Name: "value", Column: "value", Unique: true}
	if err := table.CreateIndex(index); err == nil {
		t.Fatal("oversized backfill succeeded")
	}
	if len(table.Indexes()) != 0 {
		t.Fatal("failed index published")
	}
	prefix := indexEntryPrefix("atomic", "value")
	entries, err := d.Range(string(prefix), string(prefixEnd(prefix)))
	if err != nil || len(entries) != 0 {
		t.Fatalf("partial backfill = %v, %v", entries, err)
	}
	if _, err := table.Delete(int64(1)); err != nil {
		t.Fatal(err)
	}
	if err := table.CreateIndex(index); err != nil {
		t.Fatal(err)
	}
	if err := table.Update(atomicTestRow(0, large)); err == nil {
		t.Fatal("oversized index update succeeded")
	}
	if err := table.Insert(atomicTestRow(2, large)); err == nil {
		t.Fatal("oversized index insert succeeded")
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = reopened.Close() }()
	table, err = reopened.OpenTable("atomic")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := table.FindByIndex("value", "small")
	if err != nil || len(rows) != 1 {
		t.Fatalf("original index = %v, %v", rows, err)
	}
	row, found, err := table.Get(int64(0))
	if err != nil || !found || row["value"] != "small" {
		t.Fatalf("original row = %v, %v, %v", row, found, err)
	}
	if _, found, err := table.Get(int64(2)); err != nil || found {
		t.Fatalf("partial insert = %v, %v", found, err)
	}
}

func TestTransactionRejectsClosedDatabaseIncludingBufferedReads(t *testing.T) {
	d := New()
	table := atomicTestTable(t, d)
	tx, err := d.Begin(TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set("buffered", "value"); err != nil {
		t.Fatal(err)
	}
	if err := d.Close(); err != nil {
		t.Fatal(err)
	}
	operations := []func() error{
		func() error { _, _, err := tx.Get("buffered"); return err },
		func() error { _, err := tx.Range("z", "a"); return err },
		func() error { return tx.Set("x", "y") },
		func() error { _, err := tx.Delete(""); return err },
		func() error { return tx.Insert(table, atomicTestRow(1, "a")) },
		tx.Commit,
	}
	for _, operation := range operations {
		if err := operation(); !errors.Is(err, ErrClosed) {
			t.Fatalf("error = %v, want ErrClosed", err)
		}
	}
	if err := tx.Rollback(); err != nil {
		t.Fatal(err)
	}
	if d.writerActive {
		t.Fatal("rollback did not release admission")
	}
}

func TestStandaloneTableWritesSerialize(t *testing.T) {
	d := New()
	table := atomicTestTable(t, d)
	if err := table.CreateIndex(Index{Name: "value", Column: "value", Unique: true}); err != nil {
		t.Fatal(err)
	}
	const count = 32
	errors := make(chan error, count)
	start := make(chan struct{})
	var workers sync.WaitGroup
	for id := 0; id < count; id++ {
		workers.Add(1)
		go func(id int) {
			defer workers.Done()
			<-start
			errors <- table.Insert(atomicTestRow(int64(id), strings.Repeat("x", id)))
		}(id)
	}
	close(start)
	workers.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, err := table.Scan()
	if err != nil || len(rows) != count {
		t.Fatalf("Scan = %d rows, %v", len(rows), err)
	}
	for id := 0; id < count; id++ {
		rows, err := table.FindByIndex("value", strings.Repeat("x", id))
		if err != nil || len(rows) != 1 || rows[0]["id"] != int64(id) {
			t.Fatalf("index %d = %v, %v", id, rows, err)
		}
	}
}
