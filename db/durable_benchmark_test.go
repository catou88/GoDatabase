package db_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"godatabase/db"
)

var (
	durableBenchmarkValue   string
	durableBenchmarkFound   bool
	durableBenchmarkDeleted bool
	durableBenchmarkItems   []db.Item
)

var durableBenchmarkDatasetSizes = []int{10, 100, 1_000}

func BenchmarkDurableSet(b *testing.B) {
	for _, size := range durableBenchmarkDatasetSizes {
		b.Run(fmt.Sprintf("starting_size=%d", size), func(b *testing.B) {
			database, _ := newDurableBenchmarkDatabase(b, size)
			keys := make([]string, b.N)
			for i := range keys {
				keys[i] = fmt.Sprintf("insert-%012d", i)
			}
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if err := database.Set(keys[i], "value"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkDurableOverwrite(b *testing.B) {
	for _, size := range durableBenchmarkDatasetSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			database, keys := newDurableBenchmarkDatabase(b, size)
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if err := database.Set(keys[i%size], "updated-value"); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkDurableGet(b *testing.B) {
	for _, size := range durableBenchmarkDatasetSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			database, keys := newDurableBenchmarkDatabase(b, size)
			var value string
			var found bool
			var err error
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				value, found, err = database.Get(keys[i%size])
				if err != nil {
					b.Fatal(err)
				}
			}

			durableBenchmarkValue = value
			durableBenchmarkFound = found
		})
	}
}

func BenchmarkDurableDelete(b *testing.B) {
	for _, size := range durableBenchmarkDatasetSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			database, keys := newDurableBenchmarkDatabase(b, size)
			var deleted bool
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				key := keys[i%size]
				b.StopTimer()
				if err := database.Set(key, "value"); err != nil {
					b.Fatal(err)
				}
				b.StartTimer()
				var err error
				deleted, err = database.Delete(key)
				if err != nil {
					b.Fatal(err)
				}
			}

			durableBenchmarkDeleted = deleted
		})
	}
}

func BenchmarkDurableRange(b *testing.B) {
	for _, size := range durableBenchmarkDatasetSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			database, keys := newDurableBenchmarkDatabase(b, size)
			var items []db.Item
			var err error
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				items, err = database.Range(keys[0], keys[len(keys)-1])
				if err != nil {
					b.Fatal(err)
				}
			}

			durableBenchmarkItems = items
		})
	}
}

func BenchmarkDurableCloseOpen(b *testing.B) {
	for _, size := range durableBenchmarkDatasetSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			path := filepath.Join(b.TempDir(), "database.db")
			database, _ := newDurableBenchmarkDatabaseAtPath(b, path, size)
			b.Cleanup(func() {
				if err := database.Close(); err != nil {
					b.Errorf("Close() error = %v", err)
				}
			})
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				if err := database.Close(); err != nil {
					b.Fatal(err)
				}
				var err error
				database, err = db.Open(path)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func newDurableBenchmarkDatabase(b *testing.B, size int) (*db.Database, []string) {
	b.Helper()
	path := filepath.Join(b.TempDir(), "database.db")
	database, keys := newDurableBenchmarkDatabaseAtPath(b, path, size)
	b.Cleanup(func() {
		if err := database.Close(); err != nil {
			b.Errorf("Close() error = %v", err)
		}
	})
	return database, keys
}

func newDurableBenchmarkDatabaseAtPath(b *testing.B, path string, size int) (*db.Database, []string) {
	b.Helper()
	database, err := db.Open(path)
	if err != nil {
		b.Fatalf("Open() error = %v", err)
	}
	keys := make([]string, size)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%08d", i)
		if err := database.Set(keys[i], "value"); err != nil {
			_ = database.Close()
			b.Fatalf("Set(%q) error = %v", keys[i], err)
		}
	}
	return database, keys
}
