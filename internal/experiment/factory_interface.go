package experiment

import (
	"context"
	"godatabase/internal/metrics"
	"godatabase/internal/structures"
)

// Factory creates an isolated structure and a cleanup function for each run.
type Factory interface {
	Open(context.Context, metrics.Structure) (structures.KV, func() error, error)
}
