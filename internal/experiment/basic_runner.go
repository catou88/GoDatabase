package experiment

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"time"

	"godatabase/internal/metrics"
	"godatabase/internal/server/models"
	"godatabase/internal/trace"
)

// BasicRunner is the local lab runner. It uses a map reference implementation
// until the other structure adapters are connected to the experiment engine.
type BasicRunner struct {
	Complexity metrics.Catalog
}

func (r BasicRunner) Run(ctx context.Context, request models.ExperimentRequest) (models.ExperimentResult, error) {
	if err := ctx.Err(); err != nil {
		return models.ExperimentResult{}, err
	}
	values := make(map[string]string, request.DatasetSize)
	for i := 0; i < request.DatasetSize; i++ {
		values[fmt.Sprintf("key-%06d", i)] = fmt.Sprintf("value-%06d", i)
	}
	var recorder trace.Sink = trace.Disabled{}
	if request.Trace {
		recorder = trace.NewRecorder()
	}
	start := time.Now()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	results := make([]models.OperationResult, 0, len(request.Operations))
	for _, operation := range request.Operations {
		recorder.Emit(trace.Event{Type: trace.OperationSelected, Operation: trace.Operation(operation.Name), Structure: string(request.Structure)})
		result, err := runOperation(values, operation, recorder)
		if err != nil {
			return models.ExperimentResult{}, err
		}
		results = append(results, result)
		recorder.Emit(trace.Event{Type: trace.ResultDelivered, Operation: trace.Operation(operation.Name)})
	}
	runtime.ReadMemStats(&after)
	result := models.ExperimentResult{Structure: request.Structure, Results: results, Metrics: models.Measurements{DurationNS: time.Since(start).Nanoseconds(), Bytes: int64(after.TotalAlloc - before.TotalAlloc), Allocs: int64(after.Mallocs - before.Mallocs)}}
	for _, operation := range []metrics.Operation{metrics.Set, metrics.Get, metrics.Delete, metrics.Range} {
		if metadata, ok := r.catalog().Lookup(request.Structure, operation); ok {
			result.Complexity = append(result.Complexity, metadata)
		}
	}
	if recorder.Enabled() {
		result.Trace = recorder.(*trace.Recorder).Events()
	}
	return result, nil
}

func (r BasicRunner) catalog() metrics.Catalog {
	if r.Complexity != nil {
		return r.Complexity
	}
	return metrics.DefaultCatalog()
}

func runOperation(values map[string]string, operation models.Operation, sink trace.Sink) (models.OperationResult, error) {
	sink.Emit(trace.Event{Type: trace.Lookup, Operation: trace.Operation(operation.Name), Key: operation.Key})
	switch operation.Name {
	case metrics.Set:
		values[operation.Key] = operation.Value
		return models.OperationResult{}, nil
	case metrics.Get:
		value, ok := values[operation.Key]
		return models.OperationResult{Found: ok, Value: value}, nil
	case metrics.Delete:
		_, ok := values[operation.Key]
		delete(values, operation.Key)
		return models.OperationResult{Found: ok}, nil
	case metrics.Range:
		keys := make([]string, 0, len(values))
		for key := range values {
			if key >= operation.Start && key <= operation.End {
				keys = append(keys, key)
			}
		}
		sort.Strings(keys)
		return models.OperationResult{Count: len(keys)}, nil
	default:
		return models.OperationResult{}, fmt.Errorf("unsupported operation: %s", operation.Name)
	}
}
