package structures

import "godatabase/internal/trace"

// Traceable attaches a run-local observer after dataset setup.
type Traceable interface{ SetTrace(trace.Sink) }
