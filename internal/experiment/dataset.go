package experiment

import (
	"crypto/sha256"
	"fmt"
)

const PreviewLimit = 64

// SampleRecord is stable across runs, independent of Go's random generator.
func SampleRecord(seed int64, index int) Record {
	digest := sha256.Sum256([]byte(fmt.Sprintf("%d:%d", seed, index)))
	return Record{Key: fmt.Sprintf("key-%06d", index), Value: fmt.Sprintf("record-%x", digest[:6])}
}
