package experiment

import (
	"context"
	"godatabase/internal/metrics"
	"reflect"
	"testing"
)

func TestCachedWorkloadMatchesBacking(t *testing.T) {
	for _, kind := range []metrics.Structure{metrics.Map, metrics.SortedSlice, metrics.BTreeMemory, metrics.BTreeDurable} {
		req := ExperimentRequest{Structure: kind, DatasetSize: 3, Operations: []Operation{
			{Name: metrics.Get, Key: "key-000000"}, {Name: metrics.Get, Key: "key-000000"},
			{Name: metrics.Set, Key: "key-000000", Value: "new"}, {Name: metrics.Get, Key: "key-000000"},
			{Name: metrics.Delete, Key: "key-000000"}, {Name: metrics.Get, Key: "key-000000"},
		}}
		runner := NewRunner(nil)
		plain, err := runner.Run(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		req.Cache = &CacheConfig{Capacity: 2, Policy: "lru"}
		cached, err := runner.Run(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(plain.Results, cached.Results) || cached.Cache.Hits != 1 {
			t.Fatalf("cache differs for %s", kind)
		}
	}
}
