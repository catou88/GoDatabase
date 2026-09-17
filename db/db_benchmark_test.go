package db

import (
	"fmt"
	"testing"
)

var (
	benchmarkValue   string
	benchmarkFound   bool
	benchmarkDeleted bool
	benchmarkItems   []Item
)

var benchmarkDatasetSizes = []int{10, 1_000, 100_000}

func BenchmarkDatabaseSet(b *testing.B) {
	for _, size := range benchmarkDatasetSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			database, keys := newBenchmarkDatabase(b, size)
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

func BenchmarkDatabaseGet(b *testing.B) {
	for _, size := range benchmarkDatasetSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			database, keys := newBenchmarkDatabase(b, size)
			var value string
			var found bool
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				value, found = database.Get(keys[i%size])
			}

			benchmarkValue = value
			benchmarkFound = found
		})
	}
}

func BenchmarkDatabaseDelete(b *testing.B) {
	const minimumBatchSize = 8_192

	for _, size := range benchmarkDatasetSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			_, keys := newBenchmarkDatabase(b, size)
			databaseCount := (minimumBatchSize + size - 1) / size
			databases := make([]*Database, databaseCount)
			for i := range databases {
				databases[i] = newBenchmarkDatabaseFromKeys(b, keys)
			}
			batchCapacity := databaseCount * size
			var deleted bool
			b.ReportAllocs()
			b.ResetTimer()

			for completed := 0; completed < b.N; {
				batchSize := min(batchCapacity, b.N-completed)
				for i := 0; i < batchSize; i++ {
					deleted = databases[i/size].Delete(keys[i%size])
				}
				completed += batchSize

				if completed < b.N {
					b.StopTimer()
					for i := 0; i < batchSize; i++ {
						databases[i/size].data[keys[i%size]] = "value"
					}
					b.StartTimer()
				}
			}

			benchmarkDeleted = deleted
		})
	}
}

func BenchmarkDatabaseRange(b *testing.B) {
	for _, size := range benchmarkDatasetSizes {
		b.Run(fmt.Sprintf("size=%d", size), func(b *testing.B) {
			database, keys := newBenchmarkDatabase(b, size)
			var items []Item
			b.ReportAllocs()
			b.ResetTimer()

			for i := 0; i < b.N; i++ {
				items = database.Range(keys[0], keys[len(keys)-1])
			}

			benchmarkItems = items
		})
	}
}

func newBenchmarkDatabase(b *testing.B, size int) (*Database, []string) {
	b.Helper()

	keys := make([]string, size)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%08d", i)
	}
	database := newBenchmarkDatabaseFromKeys(b, keys)
	return database, keys
}

func newBenchmarkDatabaseFromKeys(b *testing.B, keys []string) *Database {
	b.Helper()

	database := New()
	for _, key := range keys {
		if err := database.Set(key, "value"); err != nil {
			b.Fatalf("Set(%q) error = %v", key, err)
		}
	}
	return database
}
