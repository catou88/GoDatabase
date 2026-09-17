package db_test

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"godatabase/db"
)

const (
	e2eRecordCount = 24
	e2eValueSize   = 3000
)

func TestDurableDatabaseEndToEnd(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	database := openDatabase(t, path)
	keys := make([]string, e2eRecordCount)

	// Large entries force leaf, non-root, and root splits with a small,
	// deterministic data set.
	for i := range keys {
		keys[i] = e2eKey(i)
		if err := database.Set(keys[i], e2eValue(i)); err != nil {
			t.Fatalf("Set(%d) error = %v", i, err)
		}
	}
	closeDatabase(t, database)

	database = openDatabase(t, path)
	for i, key := range keys {
		assertDatabaseValue(t, database, key, e2eValue(i))
	}
	assertRange(t, database, keys[4], keys[19], e2eItems(keys, 4, 20))

	if err := database.Set(keys[12], "overwritten"); err != nil {
		t.Fatalf("Set(overwrite) error = %v", err)
	}
	closeDatabase(t, database)

	database = openDatabase(t, path)
	assertDatabaseValue(t, database, keys[12], "overwritten")

	// Removing all but the final three records forces underfull leaves to merge
	// and contracts the upper levels of the tree.
	for i := 0; i < e2eRecordCount-3; i++ {
		deleted, err := database.Delete(keys[i])
		if err != nil {
			t.Fatalf("Delete(%d) error = %v", i, err)
		}
		if !deleted {
			t.Fatalf("Delete(%d) = false, want true", i)
		}
	}
	closeDatabase(t, database)

	database = openDatabase(t, path)
	defer closeDatabase(t, database)
	for i := 0; i < e2eRecordCount-3; i++ {
		if _, found, err := database.Get(keys[i]); err != nil || found {
			t.Fatalf("Get(deleted %d) = found %v, error %v; want false, nil", i, found, err)
		}
	}
	for i := e2eRecordCount - 3; i < e2eRecordCount; i++ {
		assertDatabaseValue(t, database, keys[i], e2eValue(i))
	}
	assertRange(
		t,
		database,
		keys[e2eRecordCount-3],
		keys[e2eRecordCount-1],
		e2eItems(keys, e2eRecordCount-3, e2eRecordCount),
	)
}

func TestDurableDatabaseEndToEndReusesPages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	database := openDatabase(t, path)

	for i := 0; i < 12; i++ {
		if err := database.Set("key", fmt.Sprintf("warm-%d", i)); err != nil {
			t.Fatalf("warm-up Set(%d) error = %v", i, err)
		}
	}
	warmSize := databaseFileSize(t, path)

	for i := 0; i < 40; i++ {
		deleted, err := database.Delete("key")
		if err != nil || !deleted {
			t.Fatalf("Delete(%d) = (%v, %v), want (true, nil)", i, deleted, err)
		}
		if err := database.Set("key", fmt.Sprintf("value-%d", i)); err != nil {
			t.Fatalf("Set(%d) error = %v", i, err)
		}
	}
	finalSize := databaseFileSize(t, path)
	const maximumSteadyGrowth = 8 * 1024
	if finalSize > warmSize+maximumSteadyGrowth {
		t.Fatalf("database grew from %d to %d during steady page reuse", warmSize, finalSize)
	}
	closeDatabase(t, database)

	database = openDatabase(t, path)
	defer closeDatabase(t, database)
	assertDatabaseValue(t, database, "key", "value-39")
}

func TestDurableDatabaseEndToEndRejectsInvalidUsage(t *testing.T) {
	if _, err := db.Open(""); err == nil {
		t.Fatal("Open(empty path) error = nil, want error")
	}
	directory := t.TempDir()
	if _, err := db.Open(directory); err == nil {
		t.Fatal("Open(directory) error = nil, want error")
	}
	if _, err := db.Open(filepath.Join(directory, "missing", "database.db")); err == nil {
		t.Fatal("Open(missing parent) error = nil, want error")
	}

	database := openDatabase(t, filepath.Join(directory, "database.db"))
	closeDatabase(t, database)
	if err := database.Set("key", "value"); !errors.Is(err, db.ErrClosed) {
		t.Fatalf("Set() error = %v, want %v", err, db.ErrClosed)
	}
	if _, _, err := database.Get("key"); !errors.Is(err, db.ErrClosed) {
		t.Fatalf("Get() error = %v, want %v", err, db.ErrClosed)
	}
	if _, err := database.Delete("key"); !errors.Is(err, db.ErrClosed) {
		t.Fatalf("Delete() error = %v, want %v", err, db.ErrClosed)
	}
	if _, err := database.Range("a", "z"); !errors.Is(err, db.ErrClosed) {
		t.Fatalf("Range() error = %v, want %v", err, db.ErrClosed)
	}
}

func openDatabase(t *testing.T, path string) *db.Database {
	t.Helper()
	database, err := db.Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	return database
}

func closeDatabase(t *testing.T, database *db.Database) {
	t.Helper()
	if err := database.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func assertDatabaseValue(t *testing.T, database *db.Database, key, want string) {
	t.Helper()
	value, found, err := database.Get(key)
	if err != nil || !found || value != want {
		t.Fatalf("Get(%q) = (%q, %v, %v), want (%q, true, nil)", key, value, found, err, want)
	}
}

func assertRange(t *testing.T, database *db.Database, start, end string, want []db.Item) {
	t.Helper()
	items, err := database.Range(start, end)
	if err != nil {
		t.Fatalf("Range() error = %v", err)
	}
	if len(items) != len(want) {
		t.Fatalf("Range() returned %d items, want %d", len(items), len(want))
	}
	for i, item := range items {
		if item != want[i] {
			t.Fatalf("Range()[%d] = %#v, want %#v", i, item, want[i])
		}
	}
}

func e2eItems(keys []string, start, end int) []db.Item {
	items := make([]db.Item, 0, end-start)
	for i := start; i < end; i++ {
		items = append(items, db.Item{Key: keys[i], Value: e2eValue(i)})
	}
	return items
}

func databaseFileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", path, err)
	}
	return info.Size()
}

func e2eKey(index int) string {
	return fmt.Sprintf("%03d-%s", index, strings.Repeat("k", 900))
}

func e2eValue(index int) string {
	return strings.Repeat(string(rune('a'+index%26)), e2eValueSize)
}
