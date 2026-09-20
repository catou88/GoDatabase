package experiment

import (
	"context"
	"encoding/json"
	"godatabase/internal/metrics"
	"reflect"
	"strings"
	"testing"
)

func TestSeededResults(t *testing.T) {
	req := ExperimentRequest{Structure: metrics.Map, Seed: 42, DatasetSize: 100, Operations: []Operation{
		{Name: metrics.Get, Key: "key-000000"}, {Name: metrics.Get, Key: "missing"},
		{Name: metrics.Range, Start: "", End: "z"},
		{Name: metrics.Set, Key: "empty", Value: ""}, {Name: metrics.Get, Key: "empty"},
	}}
	runner := NewRunner(nil)
	a, err := runner.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	b, err := runner.Run(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.Dataset, b.Dataset) || !reflect.DeepEqual(a.Results, b.Results) {
		t.Fatal("replay differs")
	}
	if len(a.Dataset) != PreviewLimit || !a.DatasetTruncated || a.Results[2].Count != 100 || !a.Results[2].Truncated {
		t.Fatal("preview limits incorrect")
	}
	if a.Results[0].Value != SampleRecord(42, 0).Value || a.Results[1].Status != "not_found" || !a.Results[4].Found {
		t.Fatal("explicit results incorrect")
	}
	data, err := json.Marshal(a.Results[4])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"value":""`) {
		t.Fatal("empty value omitted")
	}
	if SampleRecord(42, 0) == SampleRecord(43, 0) {
		t.Fatal("seed ignored")
	}
}
