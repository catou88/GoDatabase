package db_test

import (
	"errors"
	"path/filepath"
	"testing"

	"godatabase/db"
)

func testSchema() db.TableSchema {
	return db.TableSchema{
		Name: "users",
		Columns: []db.Column{
			{Name: "id", Type: db.ColumnInt64, PrimaryKey: true},
			{Name: "name", Type: db.ColumnString},
			{Name: "active", Type: db.ColumnBool},
		},
	}
}

func TestCreateTableAndInsert(t *testing.T) {
	database := db.New()
	table, err := database.CreateTable(testSchema())
	if err != nil {
		t.Fatalf("CreateTable() error = %v", err)
	}
	if err := table.Insert(map[string]any{"id": int64(1), "name": "Ada", "active": true}); err != nil {
		t.Fatalf("Insert() error = %v", err)
	}
	if err := table.Insert(map[string]any{"id": int64(1), "name": "Grace", "active": false}); !errors.Is(err, db.ErrDuplicatePrimary) {
		t.Fatalf("duplicate Insert() error = %v, want %v", err, db.ErrDuplicatePrimary)
	}
	if _, err := database.CreateTable(testSchema()); !errors.Is(err, db.ErrTableExists) {
		t.Fatalf("duplicate CreateTable() error = %v, want %v", err, db.ErrTableExists)
	}
}

func TestTablePersistsDescriptorAndRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	table, err := database.CreateTable(testSchema())
	if err != nil {
		t.Fatal(err)
	}
	if err := table.Insert(map[string]any{"id": int64(7), "name": "Lin", "active": true}); err != nil {
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
	reopened, err := database.OpenTable("users")
	if err != nil {
		t.Fatalf("OpenTable() error = %v", err)
	}
	if err := reopened.Insert(map[string]any{"id": int64(8), "name": "Ken", "active": false}); err != nil {
		t.Fatal(err)
	}
}

func TestTableRejectsInvalidRowsAndSchemas(t *testing.T) {
	database := db.New()
	if _, err := database.CreateTable(db.TableSchema{Name: "bad", Columns: []db.Column{{Name: "id", Type: db.ColumnString}}}); !errors.Is(err, db.ErrInvalidSchema) {
		t.Fatalf("invalid schema error = %v, want %v", err, db.ErrInvalidSchema)
	}
	table, err := database.CreateTable(testSchema())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []map[string]any{
		{"name": "missing primary", "active": true},
		{"id": "wrong type", "name": "Ada", "active": true},
		{"id": int64(2), "name": "Ada", "active": "wrong"},
	} {
		if err := table.Insert(row); !errors.Is(err, db.ErrInvalidRow) {
			t.Errorf("Insert(%v) error = %v, want %v", row, err, db.ErrInvalidRow)
		}
	}
}
