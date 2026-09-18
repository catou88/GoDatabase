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

func BenchmarkInMemoryTableCreateIndex(b *testing.B) {
	benchmarkCreateIndex(b, false)
}

func BenchmarkDurableTableCreateIndex(b *testing.B) {
	benchmarkCreateIndex(b, true)
}

func benchmarkCreateIndex(b *testing.B, durable bool) {
	for _, size := range tableBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				database, table := seededTable(b, durable, size, fmt.Sprintf("create_index_%d", i))
				b.StartTimer()
				if err := table.CreateIndex(db.Index{Name: "value_idx", Column: "value"}); err != nil {
					b.Fatal(err)
				}
				b.StopTimer()
				if err := database.Close(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkInMemoryTableFindByIndex(b *testing.B) {
	benchmarkFindByIndex(b, false)
}

func BenchmarkDurableTableFindByIndex(b *testing.B) {
	benchmarkFindByIndex(b, true)
}

func benchmarkFindByIndex(b *testing.B, durable bool) {
	for _, size := range tableBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			database, table := seededIndexedTable(b, durable, size, "find_index")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				benchmarkSinkRows, benchmarkSinkErr = table.FindByIndex("value_idx", "value-0")
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

func BenchmarkInMemoryTableIndexedInsert(b *testing.B) {
	benchmarkIndexedInsert(b, false)
}

func BenchmarkDurableTableIndexedInsert(b *testing.B) {
	benchmarkIndexedInsert(b, true)
}

func benchmarkIndexedInsert(b *testing.B, durable bool) {
	for _, size := range tableBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			database, table := seededIndexedTable(b, durable, size, "indexed_insert")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := table.Insert(map[string]any{"id": int64(size + i), "name": fmt.Sprintf("name-%d", i), "value": fmt.Sprintf("value-%d", i), "active": true}); err != nil {
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

func BenchmarkInMemoryTableIndexedUpdate(b *testing.B) {
	benchmarkIndexedUpdate(b, false)
}

func BenchmarkDurableTableIndexedUpdate(b *testing.B) {
	benchmarkIndexedUpdate(b, true)
}

func benchmarkIndexedUpdate(b *testing.B, durable bool) {
	for _, size := range tableBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			database, table := seededIndexedTable(b, durable, size, "indexed_update")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if err := table.Update(map[string]any{"id": int64(i % size), "name": "benchmark-name", "value": fmt.Sprintf("updated-%d", i), "active": true}); err != nil {
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

func BenchmarkInMemoryTableIndexedDelete(b *testing.B) {
	benchmarkIndexedDelete(b, false)
}

func BenchmarkDurableTableIndexedDelete(b *testing.B) {
	benchmarkIndexedDelete(b, true)
}

func benchmarkIndexedDelete(b *testing.B, durable bool) {
	for _, size := range tableBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			database, table := seededIndexedTable(b, durable, size, "indexed_delete")
			b.StopTimer()
			for i := 0; i < b.N; i++ {
				if err := table.Insert(map[string]any{"id": int64(size + i), "name": "delete-name", "value": "delete-value", "active": true}); err != nil {
					b.Fatal(err)
				}
			}
			b.StartTimer()
			for i := 0; i < b.N; i++ {
				if _, err := table.Delete(int64(size + i)); err != nil {
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

func seededIndexedTable(b *testing.B, durable bool, size int, name string) (*db.Database, *db.Table) {
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
		row := benchmarkRow(int64(i))
		row["value"] = fmt.Sprintf("value-%d", i)
		if err := table.Insert(row); err != nil {
			b.Fatal(err)
		}
	}
	if err := table.CreateIndex(db.Index{Name: "value_idx", Column: "value"}); err != nil {
		b.Fatal(err)
	}
	return database, table
}
