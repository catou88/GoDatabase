// Package structures contains contracts shared by educational data
// structure implementations.
package structures

// Entry is a key-value pair returned by a range scan.
type Entry struct {
	Key   []byte
	Value []byte
}

// KV is the common logical operation boundary for educational structures.
// Implementations may expose additional capabilities in their owning package,
// but consumers of the experiment layer should depend on this interface.
type KV interface {
	Get(key []byte) ([]byte, bool, error)
	Range(start, end []byte) ([]Entry, error)
	Set(key, value []byte) error
	Delete(key []byte) (bool, error)
}
