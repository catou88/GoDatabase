package experiment

import (
	"context"
	"godatabase/internal/metrics"
	"reflect"
	"testing"
)

func TestBoundedDeterministicTrace(t *testing.T) {
	for _, kind := range []metrics.Structure{metrics.Map, metrics.SortedSlice, metrics.BTreeMemory, metrics.BTreeDurable} {
		req := ExperimentRequest{Structure: kind, DatasetSize: 20, Seed: 3, Trace: true, Cache: &CacheConfig{Capacity: 2, Policy: "lru"}, Operations: []Operation{{Name: metrics.Get, Key: "key-000010"}, {Name: metrics.Get, Key: "key-000010"}}}
		runner := NewRunner(nil)
		a, err := runner.Run(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		b, err := runner.Run(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(a.Trace, b.Trace) || len(a.Trace) == 0 {
			t.Fatalf("nondeterministic trace %s", kind)
		}
		req.Trace = false
		c, err := runner.Run(context.Background(), req)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(a.Results, c.Results) || len(c.Trace) != 0 {
			t.Fatalf("tracing altered outcomes %s", kind)
		}
		hit := false
		for _, e := range a.Trace {
			if e.Type == "cache_hit" {
				hit = true
			}
		}
		if !hit {
			t.Fatal("missing cache hit")
		}
	}
}
func TestTraceCaptureLimit(t *testing.T) {
	req := ExperimentRequest{Structure: metrics.SortedSlice, DatasetSize: 100, Trace: true}
	for i := 0; i < 100; i++ {
		req.Operations = append(req.Operations, Operation{Name: metrics.Get, Key: "key-000050"})
	}
	result, err := NewRunner(nil).Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Trace) != 200 || !result.TraceTruncated {
		t.Fatal("capture is not bounded")
	}
}
