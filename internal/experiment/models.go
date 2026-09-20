package experiment

import (
	"godatabase/internal/metrics"
	"godatabase/internal/structures"
	"godatabase/internal/trace"
)

type ExperimentRequest struct {
	Version     int               `json:"version,omitempty"`
	Cache       *CacheConfig      `json:"cache,omitempty"`
	Structure   metrics.Structure `json:"structure"`
	Operations  []Operation       `json:"operations"`
	DatasetSize int               `json:"dataset_size"`
	Seed        int64             `json:"seed"`
	Trace       bool              `json:"trace"`
}
type Operation struct {
	Name  metrics.Operation `json:"name"`
	Key   string            `json:"key,omitempty"`
	Value string            `json:"value,omitempty"`
	Start string            `json:"start,omitempty"`
	End   string            `json:"end,omitempty"`
}
type ExperimentResult struct {
	Cache            *structures.CacheStats `json:"cache,omitempty"`
	Request          ExperimentRequest      `json:"request"`
	Dataset          []Record               `json:"dataset"`
	DatasetTruncated bool                   `json:"dataset_truncated"`
	Structure        metrics.Structure      `json:"structure"`
	Results          []OperationResult      `json:"results"`
	Complexity       []metrics.Metadata     `json:"complexity"`
	Metrics          Measurements           `json:"metrics"`
	Trace            []trace.Event          `json:"trace,omitempty"`
}
type OperationResult struct {
	Operation metrics.Operation `json:"operation"`
	Key       string            `json:"key,omitempty"`
	Status    string            `json:"status"`
	Entries   []Record          `json:"entries,omitempty"`
	Truncated bool              `json:"truncated,omitempty"`
	Found     bool              `json:"found"`
	Value     string            `json:"value"`
	Count     int               `json:"count"`
	Error     string            `json:"error,omitempty"`
}
type Record struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}
type Measurements struct {
	DurationNS int64  `json:"duration_ns"`
	Bytes      int64  `json:"bytes_allocated"`
	Allocs     int64  `json:"allocations"`
	Scope      string `json:"scope"`
}

type CacheConfig struct {
	Capacity int    `json:"capacity"`
	Policy   string `json:"policy"`
}
