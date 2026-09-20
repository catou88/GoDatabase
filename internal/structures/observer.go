package structures

import "godatabase/internal/trace"

type observer struct{ sink trace.Sink }

func (o *observer) SetTrace(s trace.Sink) { o.sink = s }
func (o *observer) emit(kind trace.EventType, layer, key, detail string) {
	if o.sink != nil && o.sink.Enabled() {
		o.sink.Emit(trace.Event{Type: kind, Layer: layer, Key: key, Detail: detail})
	}
}
