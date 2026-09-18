package db_test

import (
	"fmt"
	"sync"
	"testing"

	"godatabase/db"
)

func TestConcurrentReadersAndWriters(t *testing.T) {
	database := db.New()
	for i := 0; i < 32; i++ {
		if err := database.Set(fmt.Sprintf("key-%02d", i), "initial"); err != nil {
			t.Fatal(err)
		}
	}

	var start sync.WaitGroup
	start.Add(1)
	var workers sync.WaitGroup
	for reader := 0; reader < 8; reader++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			start.Wait()
			for i := 0; i < 100; i++ {
				if _, found, err := database.Get("key-01"); err != nil || !found {
					t.Errorf("Get() = found %v, error %v", found, err)
				}
				if _, err := database.Range("key-00", "key-31"); err != nil {
					t.Errorf("Range() error = %v", err)
				}
			}
		}()
	}
	for writer := 0; writer < 4; writer++ {
		workers.Add(1)
		go func(writer int) {
			defer workers.Done()
			start.Wait()
			for i := 0; i < 100; i++ {
				if err := database.Set("key-01", fmt.Sprintf("writer-%d-%d", writer, i)); err != nil {
					t.Errorf("Set() error = %v", err)
				}
			}
		}(writer)
	}
	start.Done()
	workers.Wait()
	value, found, err := database.Get("key-01")
	if err != nil || !found || len(value) == 0 {
		t.Fatalf("final Get() = (%q, %v, %v), want one writer value", value, found, err)
	}
}

func TestConcurrentWritesPreserveDatabaseConsistency(t *testing.T) {
	database := db.New()
	const workerCount = 8
	const writesPerWorker = 50
	var group sync.WaitGroup
	for worker := 0; worker < workerCount; worker++ {
		group.Add(1)
		go func(worker int) {
			defer group.Done()
			for i := 0; i < writesPerWorker; i++ {
				key := fmt.Sprintf("worker-%d-%d", worker, i)
				if err := database.Set(key, key); err != nil {
					t.Errorf("Set(%q) error = %v", key, err)
				}
			}
		}(worker)
	}
	group.Wait()
	items, err := database.Range("worker-0-0", "worker-9-99")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != workerCount*writesPerWorker {
		t.Fatalf("Range() returned %d items, want %d", len(items), workerCount*writesPerWorker)
	}
	for _, item := range items {
		if item.Key != item.Value {
			t.Errorf("item %q has value %q", item.Key, item.Value)
		}
	}
}
