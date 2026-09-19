package trace

import (
	"sync"
)

// Disabled is the zero-overhead tracing sink used when tracing is not wanted.
type Disabled struct{}

func (Disabled) Enabled() bool { return false }
func (Disabled) Emit(Event)    {}

// Recorder stores events in emission order. It is safe for concurrent
// producers, although deterministic experiments should use one operation at a
// time when they want a reproducible sequence.
type Recorder struct {
	mu     sync.Mutex
	next   uint64
	events []Event
}

// NewRecorder constructs an enabled in-memory trace recorder.
func NewRecorder() *Recorder { return &Recorder{} }

func (r *Recorder) Enabled() bool { return true }

// Emit appends an event and assigns its sequence number. The caller's event
// sequence is ignored so all recorded events use one monotonic sequence.
func (r *Recorder) Emit(event Event) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.next++
	event.Sequence = r.next
	r.events = append(r.events, event)
}

// Events returns a snapshot that callers may safely modify.
func (r *Recorder) Events() []Event {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Event(nil), r.events...)
}
