package trace

// Bounded is a single-run recorder. Producers are called sequentially.
type Bounded struct {
	Limit          int
	OperationIndex int
	Operation      Operation
	events         []Event
	dropped        bool
}

func (b *Bounded) Enabled() bool { return b.Limit > 0 }
func (b *Bounded) Emit(e Event) {
	if b.Limit <= 0 {
		return
	}
	if len(b.events) >= b.Limit {
		b.dropped = true
		return
	}
	e.Sequence = uint64(len(b.events) + 1)
	e.OperationIndex = b.OperationIndex
	if e.Operation == "" {
		e.Operation = b.Operation
	}
	// Bound copied snapshot payload as well as event count.
	e.Keys = append([]string(nil), e.Keys[:min(len(e.Keys), 16)]...)
	e.Children = append([]uint64(nil), e.Children[:min(len(e.Children), 17)]...)
	b.events = append(b.events, e)
}
func (b *Bounded) Events() []Event { return append([]Event(nil), b.events...) }
func (b *Bounded) Truncated() bool { return b.dropped }
