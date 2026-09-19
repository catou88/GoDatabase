// Package metrics contains measurements and explanatory metadata for lab runs.
package metrics

// Operation identifies a logical key-value operation.
type Operation string

const (
	Set    Operation = "set"
	Get    Operation = "get"
	Delete Operation = "delete"
	Range  Operation = "range"
)

// Structure identifies an implementation that can be selected by an
// experiment. These names are part of the experiment response format.
type Structure string

const (
	Map          Structure = "map"
	SortedSlice  Structure = "sorted-slice"
	BTreeMemory  Structure = "btree-memory"
	BTreeDurable Structure = "btree-durable"
)

// Complexity describes asymptotic behavior. The strings are deliberately
// explanatory rather than executable expressions so the API can state the
// assumptions behind a claim.
type Complexity struct {
	Best        string `json:"best"`
	Average     string `json:"average"`
	Worst       string `json:"worst"`
	Memory      string `json:"memory"`
	Assumptions string `json:"assumptions,omitempty"`
}

// Metadata is the theoretical complexity contract for one operation.
// Measured runtime data belongs in a separate experiment result.
type Metadata struct {
	Structure  Structure  `json:"structure"`
	Operation  Operation  `json:"operation"`
	Complexity Complexity `json:"complexity"`
}

// Catalog provides theoretical metadata independently of a benchmark run.
type Catalog interface {
	Lookup(structure Structure, operation Operation) (Metadata, bool)
}
