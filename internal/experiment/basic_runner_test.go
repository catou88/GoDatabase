package experiment

import (
	"context"
	"testing"

	"godatabase/internal/metrics"
)

func TestBasicRunnerRunsSupportedOperations(t *testing.T) {
	result, err := (BasicRunner{}).Run(context.Background(), ExperimentRequest{
		Structure: metrics.Map,
		Trace:     true,
		Operations: []Operation{
			{Name: metrics.Set, Key: "a", Value: "one"},
			{Name: metrics.Get, Key: "a"},
			{Name: metrics.Get, Key: "missing"},
			{Name: metrics.Range, Start: "a", End: "z"},
			{Name: metrics.Delete, Key: "a"},
			{Name: metrics.Delete, Key: "missing"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Results) != 6 || len(result.Trace) == 0 {
		t.Fatalf("got %d results and %d trace events", len(result.Results), len(result.Trace))
	}
	if !result.Results[1].Found || result.Results[2].Found || result.Results[3].Count != 1 {
		t.Fatalf("unexpected operation results: %+v", result.Results)
	}
	if !result.Results[4].Found || result.Results[5].Found {
		t.Fatalf("unexpected delete results: %+v", result.Results)
	}
}

func TestBasicRunnerRejectsOversizedDatasetBeforeAllocation(t *testing.T) {
	for _, size := range []int{-1, maxRunnerDatasetSize + 1} {
		if _, err := (BasicRunner{}).Run(context.Background(), ExperimentRequest{DatasetSize: size}); err == nil {
			t.Fatalf("DatasetSize=%d was accepted", size)
		}
	}
}

func TestBasicRunnerHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (BasicRunner{}).Run(ctx, ExperimentRequest{}); err == nil {
		t.Fatal("expected canceled context error")
	}
}

func TestBasicRunnerRejectsUnsupportedOperation(t *testing.T) {
	_, err := (BasicRunner{}).Run(context.Background(), ExperimentRequest{
		Operations: []Operation{{Name: "unknown", Key: "a"}},
	})
	if err == nil {
		t.Fatal("expected unsupported operation error")
	}
}

func TestBasicRunnerUsesInjectedCatalogWithoutTracing(t *testing.T) {
	result, err := (BasicRunner{Complexity: metrics.DefaultCatalog()}).Run(context.Background(), ExperimentRequest{
		Structure: metrics.Map,
		Operations: []Operation{
			{Name: metrics.Set, Key: "a", Value: "one"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Trace != nil || len(result.Complexity) == 0 {
		t.Fatalf("unexpected trace or complexity metadata: trace=%v complexity=%d", result.Trace, len(result.Complexity))
	}
}
