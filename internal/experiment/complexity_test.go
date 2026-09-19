package experiment

import (
	"testing"

	"godatabase/internal/metrics"
)

func TestDefaultComplexityProviderReturnsMetadata(t *testing.T) {
	provider := DefaultComplexityProvider()
	metadata, ok := provider.Complexity(metrics.BTreeMemory, metrics.Range)
	if !ok {
		t.Fatal("Complexity() did not return B+Tree range metadata")
	}
	if metadata.Complexity.Average != "O(log n + k)" {
		t.Fatalf("average range complexity = %q, want O(log n + k)", metadata.Complexity.Average)
	}
}
