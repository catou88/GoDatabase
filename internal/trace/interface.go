// Package trace contains optional, structured execution tracing for lab runs.
package trace

// EventType identifies a stage in an operation's execution path.
type EventType string

const (
	OperationSelected EventType = "operation_selected"
	Lookup            EventType = "lookup"
	Comparison        EventType = "comparison"
	Traversal         EventType = "traversal"
	ResultDelivered   EventType = "result_delivered"
)

// Operation identifies a logical key-value operation in a trace.
type Operation string

const (
	Set    Operation = "set"
	Get    Operation = "get"
	Delete Operation = "delete"
	Range  Operation = "range"
)

// Event is one serializable step in an operation. Sequence is assigned by the
// recorder and makes event order explicit without relying on timestamps.
type Event struct {
	OperationIndex int       `json:"operation_index"`
	Layer          string    `json:"layer,omitempty"`
	Keys           []string  `json:"keys,omitempty"`
	Children       []uint64  `json:"children,omitempty"`
	Sequence       uint64    `json:"sequence"`
	Type           EventType `json:"type"`
	Operation      Operation `json:"operation"`
	Structure      string    `json:"structure,omitempty"`
	Key            string    `json:"key,omitempty"`
	NodeID         uint64    `json:"node_id,omitempty"`
	PageID         uint64    `json:"page_id,omitempty"`
	Detail         string    `json:"detail,omitempty"`
}

// Sink receives optional operation events. Implementations should return
// quickly because callers may emit events in the operation's hot path.
type Sink interface {
	Enabled() bool
	Emit(Event)
}
