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

func stringPrimarySchema() db.TableSchema {
	return db.TableSchema{
		Name: "sessions",
		Columns: []db.Column{
			{Name: "key", Type: db.ColumnString, PrimaryKey: true},
			{Name: "value", Type: db.ColumnString},
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
	row, found, err := reopened.Get(int64(7))
	if err != nil || !found || row["name"] != "Lin" {
		t.Fatalf("reopened Get() = (%v, %v, %v), want persisted row", row, found, err)
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

func TestTableGetByPrimaryKey(t *testing.T) {
	database := db.New()
	table, err := database.CreateTable(testSchema())
	if err != nil {
		t.Fatal(err)
	}
	if err := table.Insert(map[string]any{"id": int64(7), "name": "Lin", "active": true}); err != nil {
		t.Fatal(err)
	}

	row, found, err := table.Get(int64(7))
	if err != nil || !found {
		t.Fatalf("Get() = (%v, %v, %v), want found row", row, found, err)
	}
	if row["id"] != int64(7) || row["name"] != "Lin" || row["active"] != true {
		t.Fatalf("Get() row = %#v, want decoded values", row)
	}
	if row, found, err := table.Get(int64(8)); err != nil || found || row != nil {
		t.Fatalf("missing Get() = (%v, %v, %v), want (nil, false, nil)", row, found, err)
	}
}

func TestTableGetDoesNotCrossTableBoundaries(t *testing.T) {
	database := db.New()
	first, err := database.CreateTable(testSchema())
	if err != nil {
		t.Fatal(err)
	}
	secondSchema := testSchema()
	secondSchema.Name = "accounts"
	second, err := database.CreateTable(secondSchema)
	if err != nil {
		t.Fatal(err)
	}
	row := map[string]any{"id": int64(1), "name": "same key", "active": true}
	if err := first.Insert(row); err != nil {
		t.Fatal(err)
	}
	if got, found, err := second.Get(int64(1)); err != nil || found || got != nil {
		t.Fatalf("second table Get() = (%v, %v, %v), want missing", got, found, err)
	}
	if err := second.Insert(row); err != nil {
		t.Fatal(err)
	}
	if got, found, err := first.Get(int64(1)); err != nil || !found || got["name"] != "same key" {
		t.Fatalf("first table Get() = (%v, %v, %v), want its own row", got, found, err)
	}
}

func TestTableGetIntegerPrimaryKeyBoundaries(t *testing.T) {
	database := db.New()
	table, err := database.CreateTable(testSchema())
	if err != nil {
		t.Fatal(err)
	}
	keys := []int64{-1, 0, 1, -1 << 63, 1<<63 - 1}
	for _, key := range keys {
		if err := table.Insert(map[string]any{"id": key, "name": "value", "active": true}); err != nil {
			t.Fatalf("Insert(%d) error = %v", key, err)
		}
	}
	for _, key := range keys {
		row, found, err := table.Get(key)
		if err != nil || !found || row["id"] != key {
			t.Errorf("Get(%d) = (%v, %v, %v), want matching row", key, row, found, err)
		}
	}
}

func TestTableGetRejectsWrongPrimaryKeyTypeAndNil(t *testing.T) {
	database := db.New()
	table, err := database.CreateTable(testSchema())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []any{"1", nil, uint64(1)} {
		if row, found, err := table.Get(key); !errors.Is(err, db.ErrInvalidRow) || found || row != nil {
			t.Errorf("Get(%v) = (%v, %v, %v), want invalid row", key, row, found, err)
		}
	}
}

func TestTableGetSupportsEmptyStringPrimaryKey(t *testing.T) {
	database := db.New()
	table, err := database.CreateTable(stringPrimarySchema())
	if err != nil {
		t.Fatal(err)
	}
	if err := table.Insert(map[string]any{"key": "", "value": "empty key"}); err != nil {
		t.Fatalf("Insert(empty string key) error = %v", err)
	}
	row, found, err := table.Get("")
	if err != nil || !found || row["key"] != "" || row["value"] != "empty key" {
		t.Fatalf("Get(empty string key) = (%v, %v, %v), want matching row", row, found, err)
	}
}

func TestTableGetAfterDatabaseClose(t *testing.T) {
	database := db.New()
	table, err := database.CreateTable(testSchema())
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Close(); err != nil {
		t.Fatal(err)
	}
	if row, found, err := table.Get(int64(1)); !errors.Is(err, db.ErrClosed) || found || row != nil {
		t.Fatalf("Get after Close() = (%v, %v, %v), want closed error", row, found, err)
	}
}

func TestTableRangeReturnsOrderedRows(t *testing.T) {
	database := db.New()
	table, err := database.CreateTable(testSchema())
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []int64{5, 1, 3, 2, 4} {
		if err := table.Insert(map[string]any{"id": key, "name": "row", "active": true}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := table.Range(int64(2), int64(4))
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("Range() returned %d rows, want 3", len(rows))
	}
	for i, want := range []int64{2, 3, 4} {
		if got := rows[i]["id"]; got != want {
			t.Errorf("rows[%d][id] = %v, want %d", i, got, want)
		}
	}
}

func TestTableRangeHandlesEmptyAndNonmatchingRanges(t *testing.T) {
	database := db.New()
	table, err := database.CreateTable(testSchema())
	if err != nil {
		t.Fatal(err)
	}
	if err := table.Insert(map[string]any{"id": int64(2), "name": "row", "active": true}); err != nil {
		t.Fatal(err)
	}
	for name, bounds := range map[string][2]int64{
		"no match": [2]int64{3, 4},
		"reversed": [2]int64{4, 3},
	} {
		rows, err := table.Range(bounds[0], bounds[1])
		if err != nil || len(rows) != 0 {
			t.Errorf("%s Range() = (%v, %v), want empty, nil", name, rows, err)
		}
	}
}

func TestTableRangeDoesNotCrossTableBoundaries(t *testing.T) {
	database := db.New()
	first, err := database.CreateTable(testSchema())
	if err != nil {
		t.Fatal(err)
	}
	secondSchema := testSchema()
	secondSchema.Name = "accounts"
	second, err := database.CreateTable(secondSchema)
	if err != nil {
		t.Fatal(err)
	}
	for _, table := range []*db.Table{first, second} {
		if err := table.Insert(map[string]any{"id": int64(1), "name": "row", "active": true}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := first.Range(int64(1), int64(1))
	if err != nil || len(rows) != 1 || rows[0]["id"] != int64(1) {
		t.Fatalf("first Range() = (%v, %v), want one row from first table", rows, err)
	}
}

func TestTableRangeRejectsInvalidBounds(t *testing.T) {
	database := db.New()
	table, err := database.CreateTable(testSchema())
	if err != nil {
		t.Fatal(err)
	}
	for _, bounds := range [][2]any{{"1", int64(2)}, {nil, int64(2)}, {int64(1), "2"}} {
		if rows, err := table.Range(bounds[0], bounds[1]); !errors.Is(err, db.ErrInvalidRow) || rows != nil {
			t.Errorf("Range(%v, %v) = (%v, %v), want invalid row", bounds[0], bounds[1], rows, err)
		}
	}
}

func TestCreateIndexOnEmptyTable(t *testing.T) {
	database := db.New()
	table, err := database.CreateTable(testSchema())
	if err != nil {
		t.Fatal(err)
	}
	index := db.Index{Name: "name_idx", Column: "name"}
	if err := table.CreateIndex(index); err != nil {
		t.Fatalf("CreateIndex() error = %v", err)
	}
	if got := table.Indexes(); len(got) != 1 || got[0] != index {
		t.Fatalf("Indexes() = %#v, want %#v", got, []db.Index{index})
	}
	if err := table.CreateIndex(index); !errors.Is(err, db.ErrIndexExists) {
		t.Fatalf("duplicate CreateIndex() error = %v, want %v", err, db.ErrIndexExists)
	}
}

func TestCreateIndexBackfillsExistingRows(t *testing.T) {
	database := db.New()
	table, err := database.CreateTable(testSchema())
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range []map[string]any{
		{"id": int64(1), "name": "Ada", "active": true},
		{"id": int64(2), "name": "Grace", "active": true},
		{"id": int64(3), "name": "Ada", "active": false},
	} {
		if err := table.Insert(row); err != nil {
			t.Fatal(err)
		}
	}
	nonUnique := db.Index{Name: "name_idx", Column: "name"}
	if err := table.CreateIndex(nonUnique); err != nil {
		t.Fatalf("CreateIndex(non-unique) error = %v", err)
	}
	unique := db.Index{Name: "unique_name_idx", Column: "name", Unique: true}
	if err := table.CreateIndex(unique); !errors.Is(err, db.ErrDuplicateIndexed) {
		t.Fatalf("CreateIndex(unique duplicate) error = %v, want %v", err, db.ErrDuplicateIndexed)
	}
}

func TestIndexMetadataPersistsAndDefinitionsValidate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	database, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	table, err := database.CreateTable(testSchema())
	if err != nil {
		t.Fatal(err)
	}
	index := db.Index{Name: "active_idx", Column: "active", Unique: true}
	if err := table.CreateIndex(index); err != nil {
		t.Fatal(err)
	}
	if err := table.Insert(map[string]any{"id": int64(1), "name": "Ada", "active": true}); err != nil {
		t.Fatalf("Insert(first indexed row) error = %v", err)
	}
	if err := table.Insert(map[string]any{"id": int64(2), "name": "Grace", "active": true}); !errors.Is(err, db.ErrDuplicateIndexed) {
		t.Fatalf("Insert(duplicate indexed value) error = %v, want %v", err, db.ErrDuplicateIndexed)
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
		t.Fatal(err)
	}
	if got := reopened.Indexes(); len(got) != 1 || got[0] != index {
		t.Fatalf("reopened Indexes() = %#v, want %#v", got, []db.Index{index})
	}
}

func TestCreateIndexRejectsInvalidDefinitions(t *testing.T) {
	database := db.New()
	table, err := database.CreateTable(testSchema())
	if err != nil {
		t.Fatal(err)
	}
	for _, index := range []db.Index{
		{Name: "", Column: "name"},
		{Name: "missing_column", Column: "missing"},
		{Name: "primary_key", Column: "id"},
	} {
		if err := table.CreateIndex(index); !errors.Is(err, db.ErrInvalidSchema) {
			t.Errorf("CreateIndex(%#v) error = %v, want %v", index, err, db.ErrInvalidSchema)
		}
	}
}
