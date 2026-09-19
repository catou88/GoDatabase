package trace

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestDisabledTracingDoesNotRecordEvents(t *testing.T) {
	var sink Sink = Disabled{}
	sink.Emit(Event{Type: Lookup, Operation: Get, Key: "a"})
	if sink.Enabled() {
		t.Fatal("disabled sink reports Enabled() = true")
	}
}

func TestRecorderAssignsDeterministicSequence(t *testing.T) {
	recorder := NewRecorder()
	recorder.Emit(Event{Type: OperationSelected, Operation: Get, Structure: "btree-memory"})
	recorder.Emit(Event{Type: Lookup, Operation: Get, Key: "a", NodeID: 2})
	recorder.Emit(Event{Type: ResultDelivered, Operation: Get, Detail: "found"})

	want := []Event{
		{Sequence: 1, Type: OperationSelected, Operation: Get, Structure: "btree-memory"},
		{Sequence: 2, Type: Lookup, Operation: Get, Key: "a", NodeID: 2},
		{Sequence: 3, Type: ResultDelivered, Operation: Get, Detail: "found"},
	}
	if got := recorder.Events(); !reflect.DeepEqual(got, want) {
		t.Fatalf("Events() = %#v, want %#v", got, want)
	}
}

func TestTraceEventsAreJSONSerializable(t *testing.T) {
	recorder := NewRecorder()
	recorder.Emit(Event{Type: Traversal, Operation: Range, Structure: "btree-durable", PageID: 7, Detail: "next leaf"})

	encoded, err := json.Marshal(recorder.Events())
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	var decoded []Event
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if !reflect.DeepEqual(decoded, recorder.Events()) {
		t.Fatalf("decoded events = %#v, want %#v", decoded, recorder.Events())
	}
}
