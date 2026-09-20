package experiment

import (
	"context"
	"godatabase/internal/metrics"
	"os"
	"reflect"
	"testing"
)

func TestRealAdaptersMatch(t *testing.T) {
	req := ExperimentRequest{DatasetSize: 30, Seed: 42, Operations: []Operation{
		{Name: metrics.Set, Key: "a", Value: "one"},
		{Name: metrics.Set, Key: "a", Value: "two"},
		{Name: metrics.Get, Key: "a"},
		{Name: metrics.Delete, Key: "key-000003"},
		{Name: metrics.Delete, Key: "missing"},
		{Name: metrics.Range, Start: "", End: "z"},
	}}
	var expected []OperationResult
	for _, kind := range []metrics.Structure{metrics.Map, metrics.SortedSlice, metrics.BTreeMemory, metrics.BTreeDurable} {
		t.Run(string(kind), func(t *testing.T) {
			dir := t.TempDir()
			req.Structure = kind
			got, err := NewRunner(DefaultFactory{TempDir: dir}).Run(context.Background(), req)
			if err != nil {
				t.Fatal(err)
			}
			if expected == nil {
				expected = got.Results
			} else if !reflect.DeepEqual(expected, got.Results) {
				t.Fatalf("got %+v, want %+v", got.Results, expected)
			}
			files, err := os.ReadDir(dir)
			if err != nil || len(files) != 0 {
				t.Fatalf("temporary files not cleaned: %v %v", files, err)
			}
			if got.Structure != kind || got.Metrics.Scope == "" {
				t.Fatal("missing measurement identity")
			}
		})
	}
}
func TestCanceledAndInvalidRuns(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := NewRunner(nil).Run(ctx, ExperimentRequest{Structure: metrics.Map}); err == nil {
		t.Fatal("expected cancellation")
	}
	for _, req := range []ExperimentRequest{{DatasetSize: 5001}, {Structure: metrics.BTreeDurable, DatasetSize: 1001}, {Structure: "unknown"}, {Operations: []Operation{{Name: metrics.Get}}}} {
		if _, err := NewRunner(nil).Run(context.Background(), req); err == nil {
			t.Fatalf("accepted %+v", req)
		}
	}
}
