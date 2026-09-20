package experiment

import (
	"context"
	"errors"
	"godatabase/internal/metrics"
	"godatabase/internal/structures"
	"godatabase/internal/trace"
	"runtime"
	"time"
)

// Executor owns one bounded experiment at a time, including setup and cleanup.
type Executor struct {
	factory Factory
	gate    chan struct{}
}

func NewRunner(factory Factory) *Executor {
	if factory == nil {
		factory = DefaultFactory{}
	}
	return &Executor{factory: factory, gate: make(chan struct{}, 1)}
}
func Validate(req ExperimentRequest) error {
	if req.Version != 0 && req.Version != 1 {
		return errors.New("unsupported session version")
	}
	if req.Cache != nil && (req.Cache.Capacity < 1 || req.Cache.Capacity > 256 || req.Cache.Policy != "lru") {
		return errors.New("unsupported cache configuration")
	}
	if req.DatasetSize < 0 || req.DatasetSize > 5000 || req.Seed < 0 || len(req.Operations) > 128 {
		return errors.New("workload limit exceeded")
	}
	if req.Structure == metrics.BTreeDurable && req.DatasetSize > 1000 {
		return errors.New("durable dataset limit exceeded")
	}
	for _, op := range req.Operations {
		if len(op.Key) > 256 || len(op.Value) > 1024 || len(op.Start) > 256 || len(op.End) > 256 {
			return errors.New("entry limit exceeded")
		}
		switch op.Name {
		case metrics.Get, metrics.Set, metrics.Delete:
			if op.Key == "" {
				return errors.New("key is required")
			}
		case metrics.Range:
			if op.Start > op.End {
				return errors.New("reversed range")
			}
		default:
			return errors.New("unsupported operation")
		}
	}
	return nil
}
func (r *Executor) Run(ctx context.Context, req ExperimentRequest) (result ExperimentResult, err error) {
	if err = Validate(req); err != nil {
		return result, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	select {
	case r.gate <- struct{}{}:
		defer func() { <-r.gate }()
	case <-ctx.Done():
		return result, ctx.Err()
	}
	store, cleanup, err := r.factory.Open(ctx, req.Structure)
	if err != nil {
		return result, err
	}
	defer func() { err = errors.Join(err, cleanup()) }()
	result.Request = req
	previewSize := min(req.DatasetSize, PreviewLimit)
	if previewSize < 0 || previewSize > PreviewLimit {
		return result, errors.New("workload limit exceeded")
	}
	result.Dataset = make([]Record, 0, previewSize)
	result.DatasetTruncated = req.DatasetSize > PreviewLimit
	for i := 0; i < req.DatasetSize; i++ {
		if err = ctx.Err(); err != nil {
			return result, err
		}
		record := SampleRecord(req.Seed, i)
		if i < PreviewLimit {
			result.Dataset = append(result.Dataset, record)
		}
		if err = store.Set([]byte(record.Key), []byte(record.Value)); err != nil {
			return result, err
		}
	}
	result.Structure = req.Structure
	recorder := &trace.Bounded{}
	if req.Trace {
		recorder.Limit = 200
	}
	if observed, ok := store.(structures.Traceable); ok {
		observed.SetTrace(recorder)
	}
	var cache *structures.Cache
	if req.Cache != nil {
		cache, err = structures.NewCache(store, req.Cache.Capacity)
		if err != nil {
			return result, err
		}
		store = cache
		cache.SetTrace(recorder)
	}
	result.Results = make([]OperationResult, 0, len(req.Operations))
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	started := time.Now()
	for index, op := range req.Operations {
		recorder.OperationIndex = index
		recorder.Operation = trace.Operation(op.Name)
		recorder.Emit(trace.Event{Type: trace.OperationSelected, Layer: "request", Structure: string(req.Structure), Key: op.Key, Detail: "Start operation"})
		if err = ctx.Err(); err != nil {
			return result, err
		}
		var item OperationResult
		item, err = execute(store, op)
		if err != nil {
			return result, err
		}
		result.Results = append(result.Results, item)
		recorder.Emit(trace.Event{Type: trace.ResultDelivered, Layer: "result", Key: op.Key, Detail: item.Status})
	}
	duration := time.Since(started).Nanoseconds()
	result.Trace = recorder.Events()
	result.TraceTruncated = recorder.Truncated()
	runtime.ReadMemStats(&after)
	if cache != nil {
		stats := cache.Stats()
		result.Cache = &stats
	}
	result.Metrics = Measurements{DurationNS: duration, Bytes: int64(after.TotalAlloc - before.TotalAlloc), Allocs: int64(after.Mallocs - before.Mallocs), Scope: "operation loop; setup and cleanup excluded; allocations are process-wide estimates"}
	for _, op := range []metrics.Operation{metrics.Set, metrics.Get, metrics.Delete, metrics.Range} {
		if c, ok := metrics.DefaultCatalog().Lookup(req.Structure, op); ok {
			result.Complexity = append(result.Complexity, c)
		}
	}
	return result, nil
}
func execute(store structures.KV, op Operation) (OperationResult, error) {
	out := OperationResult{Operation: op.Name, Key: op.Key, Status: "success"}
	switch op.Name {
	case metrics.Set:
		out.Value = op.Value
		out.Status = "stored"
		return out, store.Set([]byte(op.Key), []byte(op.Value))
	case metrics.Get:
		v, ok, err := store.Get([]byte(op.Key))
		out.Value = string(v)
		out.Found = ok
		if ok {
			out.Status = "found"
		} else {
			out.Status = "not_found"
		}
		return out, err
	case metrics.Delete:
		ok, err := store.Delete([]byte(op.Key))
		out.Found = ok
		if ok {
			out.Status = "deleted"
		} else {
			out.Status = "not_found"
		}
		return out, err
	case metrics.Range:
		entries, err := store.Range([]byte(op.Start), []byte(op.End))
		out.Count = len(entries)
		out.Status = "matched"
		if len(entries) == 0 {
			out.Status = "empty"
		}
		out.Truncated = len(entries) > PreviewLimit
		for _, entry := range entries[:min(len(entries), PreviewLimit)] {
			out.Entries = append(out.Entries, Record{Key: string(entry.Key), Value: string(entry.Value)})
		}
		return out, err
	default:
		return out, errors.New("unsupported operation")
	}
}
