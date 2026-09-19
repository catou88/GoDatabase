package db_test

import (
	"fmt"
	"path/filepath"
	"testing"

	"godatabase/db"
)

var transactionBenchmarkSizes = []int{1, 10, 100}

func BenchmarkInMemoryTransactionReadOnly(b *testing.B) { benchmarkTransactionReadOnly(b, false) }
func BenchmarkDurableTransactionReadOnly(b *testing.B)  { benchmarkTransactionReadOnly(b, true) }

func benchmarkTransactionReadOnly(b *testing.B, durable bool) {
	for _, size := range transactionBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			database := transactionBenchmarkDatabase(b, durable, size)
			b.ResetTimer()
			for range b.N {
				tx, err := database.Begin(db.TxOptions{ReadOnly: true})
				if err != nil {
					b.Fatal(err)
				}
				transactionBenchmarkValue, transactionBenchmarkFound, transactionBenchmarkErr = tx.Get("key-0")
				if transactionBenchmarkErr != nil || !transactionBenchmarkFound {
					b.Fatal(transactionBenchmarkErr)
				}
				if err := tx.Rollback(); err != nil {
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

func BenchmarkInMemoryTransactionCommit(b *testing.B) { benchmarkTransactionCommit(b, false) }
func BenchmarkDurableTransactionCommit(b *testing.B)  { benchmarkTransactionCommit(b, true) }

func benchmarkTransactionCommit(b *testing.B, durable bool) {
	for _, size := range transactionBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			database := transactionBenchmarkDatabase(b, durable, size)
			b.ResetTimer()
			for range b.N {
				tx, err := database.Begin(db.TxOptions{})
				if err != nil {
					b.Fatal(err)
				}
				if err := tx.Set("write-key", "value"); err != nil {
					b.Fatal(err)
				}
				if err := tx.Commit(); err != nil {
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

func BenchmarkInMemoryTransactionMultiWriteCommit(b *testing.B) {
	benchmarkTransactionMultiWriteCommit(b, false)
}
func BenchmarkDurableTransactionMultiWriteCommit(b *testing.B) {
	benchmarkTransactionMultiWriteCommit(b, true)
}

func benchmarkTransactionMultiWriteCommit(b *testing.B, durable bool) {
	for _, size := range transactionBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			database := transactionBenchmarkDatabase(b, durable, size)
			b.ResetTimer()
			for range b.N {
				tx, err := database.Begin(db.TxOptions{})
				if err != nil {
					b.Fatal(err)
				}
				for field := 0; field < 10; field++ {
					if err := tx.Set(fmt.Sprintf("batch-key-%d", field), "value"); err != nil {
						b.Fatal(err)
					}
				}
				if err := tx.Commit(); err != nil {
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

func BenchmarkInMemoryTransactionRollback(b *testing.B) { benchmarkTransactionRollback(b, false) }
func BenchmarkDurableTransactionRollback(b *testing.B)  { benchmarkTransactionRollback(b, true) }

func benchmarkTransactionRollback(b *testing.B, durable bool) {
	for _, size := range transactionBenchmarkSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			database := transactionBenchmarkDatabase(b, durable, size)
			b.ResetTimer()
			for range b.N {
				tx, err := database.Begin(db.TxOptions{})
				if err != nil {
					b.Fatal(err)
				}
				if err := tx.Set("rollback-key", "value"); err != nil {
					b.Fatal(err)
				}
				if err := tx.Rollback(); err != nil {
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

func BenchmarkInMemoryTransactionTableInsert(b *testing.B) { benchmarkTransactionTableInsert(b, false) }
func BenchmarkDurableTransactionTableInsert(b *testing.B)  { benchmarkTransactionTableInsert(b, true) }

func benchmarkTransactionTableInsert(b *testing.B, durable bool) {
	for _, size := range []int{10, 100} {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			database, table := seededIndexedTable(b, durable, size, "transaction_insert")
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				tx, err := database.Begin(db.TxOptions{})
				if err != nil {
					b.Fatal(err)
				}
				row := map[string]any{"id": int64(size + i), "name": fmt.Sprintf("tx-name-%d", i), "value": fmt.Sprintf("tx-value-%d", i), "active": true}
				if err := tx.Insert(table, row); err != nil {
					b.Fatal(err)
				}
				if err := tx.Commit(); err != nil {
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
	transactionBenchmarkValue string
	transactionBenchmarkFound bool
	transactionBenchmarkErr   error
)

func transactionBenchmarkDatabase(b *testing.B, durable bool, size int) *db.Database {
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
	for i := 0; i < size; i++ {
		if err := database.Set(fmt.Sprintf("key-%d", i), "value"); err != nil {
			b.Fatal(err)
		}
	}
	return database
}
