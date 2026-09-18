package db_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"godatabase/db"
)

var tableBenchmarkSizes = []int{10, 100, 1000}

func BenchmarkInMemoryTableInsert(b *testing.B) {
	for _, size := range tableBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			_, table := seededTable(b, false, size, "memory_insert")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := table.Insert(benchmarkRow(int64(size + i))); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkInMemoryTableGet(b *testing.B) {
	for _, size := range tableBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			database, table := seededTable(b, false, size, "memory_get")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkSinkRow, benchmarkSinkFound, benchmarkSinkErr = table.Get(int64(i % size))
				if benchmarkSinkErr != nil || !benchmarkSinkFound {
					b.Fatal(benchmarkSinkErr)
				}
				_ = benchmarkSinkRow
			}
			_ = database
		})
	}
}

func BenchmarkInMemoryTableRange(b *testing.B) {
	for _, size := range tableBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			database, table := seededTable(b, false, size, "memory_range")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkSinkRows, benchmarkSinkErr = table.Range(int64(0), int64(size-1))
				if benchmarkSinkErr != nil {
					b.Fatal(benchmarkSinkErr)
				}
			}
			_ = database
		})
	}
}

func BenchmarkDurableTableInsert(b *testing.B) {
	for _, size := range tableBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			database, table := seededTable(b, true, size, "durable_insert")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := table.Insert(benchmarkRow(int64(size + i))); err != nil {
					b.Fatal(err)
				}
			}
			b.StopTimer()
			if err := database.Close(); err != nil {
				b.Fatal(err)
			}
		})
	}
}

func BenchmarkDurableTableGet(b *testing.B) {
	for _, size := range tableBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			database, table := seededTable(b, true, size, "durable_get")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkSinkRow, benchmarkSinkFound, benchmarkSinkErr = table.Get(int64(i % size))
				if benchmarkSinkErr != nil || !benchmarkSinkFound {
					b.Fatal(benchmarkSinkErr)
				}
				_ = benchmarkSinkRow
			}
			b.StopTimer()
			if err := database.Close(); err != nil {
				b.Fatal(err)
			}
		})
	}
}

func BenchmarkDurableTableRange(b *testing.B) {
	for _, size := range tableBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			database, table := seededTable(b, true, size, "durable_range")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkSinkRows, benchmarkSinkErr = table.Range(int64(0), int64(size-1))
				if benchmarkSinkErr != nil {
					b.Fatal(benchmarkSinkErr)
				}
			}
			b.StopTimer()
			if err := database.Close(); err != nil {
				b.Fatal(err)
			}
		})
	}
}

var (
	benchmarkSinkRow   map[string]any
	benchmarkSinkRows  []map[string]any
	benchmarkSinkFound bool
	benchmarkSinkErr   error
)

func benchmarkTableSchema(name string) db.TableSchema {
	return db.TableSchema{Name: name, Columns: []db.Column{{Name: "id", Type: db.ColumnInt64, PrimaryKey: true}, {Name: "value", Type: db.ColumnString}}}
}

func benchmarkRow(id int64) map[string]any {
	return map[string]any{"id": id, "value": "benchmark-value"}
}

func seededTable(b *testing.B, durable bool, size int, name string) (*db.Database, *db.Table) {
	b.Helper()
	var database *db.Database
	var err error
	if durable {
		database, err = db.Open(filepath.Join(b.TempDir(), "database.db"))
	} else {
		database = db.New()
	}
	if err != nil {
		b.Fatal(err)
	}
	table, err := database.CreateTable(benchmarkTableSchema(name))
	if err != nil {
		b.Fatal(err)
	}
	for i := 0; i < size; i++ {
		if err := table.Insert(benchmarkRow(int64(i))); err != nil {
			b.Fatal(err)
		}
	}
	return database, table
}
