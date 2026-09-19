package db_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"sort"
	"testing"

	"godatabase/db"
)

func TestTableScanStringKeys(t *testing.T) {
	for _, backend := range []string{"memory", "durable"} {
		t.Run(backend, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "scan.db")
			database := db.New()
			if backend == "durable" {
				var err error
				database, err = db.Open(path)
				if err != nil {
					t.Fatal(err)
				}
			}
			t.Cleanup(func() {
				if err := database.Close(); err != nil {
					t.Error(err)
				}
			})
			schema := db.TableSchema{Name: "records", Columns: []db.Column{
				{Name: "key", Type: db.ColumnString, PrimaryKey: true},
				{Name: "value", Type: db.ColumnString},
			}}
			table, err := database.CreateTable(schema)
			if err != nil {
				t.Fatal(err)
			}
			if rows, err := table.Scan(); err != nil || len(rows) != 0 {
				t.Fatalf("empty Scan() = (%v, %v)", rows, err)
			}
			keys := []string{"z", "", "a\x00b", "a", "\x00", "\xff\xff\xff\xff\xff", "ends\x00\x00"}
			for _, key := range keys {
				if err := table.Insert(map[string]any{"key": key, "value": "value:" + key}); err != nil {
					t.Fatalf("Insert(%q): %v", key, err)
				}
			}
			schema.Name = "recordss"
			neighbor, err := database.CreateTable(schema)
			if err != nil {
				t.Fatal(err)
			}
			if err := neighbor.Insert(map[string]any{"key": "a", "value": "other table"}); err != nil {
				t.Fatal(err)
			}
			if backend == "durable" {
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
			}
			sort.Strings(keys)
			want := make([]map[string]any, 0, len(keys))
			for _, key := range keys {
				want = append(want, map[string]any{"key": key, "value": "value:" + key})
			}
			rows, err := table.Scan()
			if err != nil || !reflect.DeepEqual(rows, want) {
				t.Fatalf("Scan() = (%#v, %v), want %#v", rows, err, want)
			}
			rows, err = table.Range(keys[0], keys[len(keys)-1])
			if err != nil || !reflect.DeepEqual(rows, want) {
				t.Fatalf("Range() = (%#v, %v), want %#v", rows, err, want)
			}
			if err := database.Close(); err != nil {
				t.Fatal(err)
			}
			if _, err := table.Scan(); !errors.Is(err, db.ErrClosed) {
				t.Fatalf("closed Scan() error = %v, want ErrClosed", err)
			}
		})
	}
}
