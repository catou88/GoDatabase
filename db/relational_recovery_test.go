package db_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"godatabase/db"
)

// Exercise the encoding and mutation guarantees together across a restart.
func TestDurableNullableIndexesAndTransactions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "relational.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if database == nil {
			return
		}
		if err := database.Close(); err != nil {
			t.Error(err)
		}
	})
	table, err := database.CreateTable(db.TableSchema{Name: "records", Columns: []db.Column{
		{Name: "id", Type: db.ColumnString, PrimaryKey: true},
		{Name: "label", Type: db.ColumnString, Nullable: true},
		{Name: "payload", Type: db.ColumnBytes, Nullable: true},
		{Name: "count", Type: db.ColumnInt64, Nullable: true},
		{Name: "active", Type: db.ColumnBool, Nullable: true},
	}})
	if err != nil {
		t.Fatal(err)
	}
	oldHandle, err := database.OpenTable("records")
	if err != nil {
		t.Fatal(err)
	}
	if err := table.CreateIndex(db.Index{Name: "labels", Column: "label"}); err != nil {
		t.Fatal(err)
	}
	rows := []map[string]any{
		{"id": "null", "label": nil, "payload": nil, "count": nil, "active": nil},
		{"id": "empty", "label": "", "payload": []byte{}, "count": int64(0), "active": false},
		{"id": "bc", "label": "a", "payload": []byte{0, 255}, "count": int64(-1), "active": true},
		{"id": "c", "label": "ab", "payload": []byte("data"), "count": int64(1), "active": false},
	}
	for _, row := range rows {
		if err := oldHandle.Insert(row); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := database.Begin(db.TxOptions{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(); err != nil {
			t.Error(err)
		}
	})
	if err := database.Set("outside", "blocked"); !errors.Is(err, db.ErrWriteTransactionActive) {
		t.Fatalf("direct write during transaction: %v", err)
	}
	updated := map[string]any{"id": "bc", "label": "a\x00b", "payload": []byte{}, "count": int64(0), "active": false}
	if err := tx.Update(oldHandle, updated); err != nil {
		t.Fatal(err)
	}
	if removed, err := tx.DeleteRow(oldHandle, "c"); err != nil || !removed {
		t.Fatalf("DeleteRow = %v, %v", removed, err)
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
	table, err = database.OpenTable("records")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []map[string]any{rows[0], rows[1], updated} {
		got, found, err := table.Get(want["id"])
		if err != nil || !found || !reflect.DeepEqual(got, want) {
			t.Fatalf("Get(%q) = %#v, %v, %v; want %#v", want["id"], got, found, err, want)
		}
		matches, err := table.FindByIndex("labels", want["label"])
		if err != nil || len(matches) != 1 || !reflect.DeepEqual(matches[0], want) {
			t.Fatalf("FindByIndex(%v) = %#v, %v; want %#v", want["label"], matches, err, want)
		}
	}
	for _, removed := range []string{"a", "ab"} {
		matches, err := table.FindByIndex("labels", removed)
		if err != nil || len(matches) != 0 {
			t.Fatalf("old index value %q = %#v, %v", removed, matches, err)
		}
	}
	if _, found, err := table.Get("c"); err != nil || found {
		t.Fatalf("deleted row after restart = %v, %v", found, err)
	}
}
