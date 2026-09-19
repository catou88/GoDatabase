package metrics

import "testing"

func TestDefaultCatalogCoversSupportedOperations(t *testing.T) {
	catalog := DefaultCatalog()
	structures := []Structure{Map, SortedSlice, BTreeMemory, BTreeDurable}
	operations := []Operation{Set, Get, Delete, Range}

	for _, structure := range structures {
		for _, operation := range operations {
			metadata, ok := catalog.Lookup(structure, operation)
			if !ok {
				t.Fatalf("Lookup(%q, %q) did not return metadata", structure, operation)
			}
			if metadata.Structure != structure || metadata.Operation != operation {
				t.Fatalf("metadata identity = %#v, want %q/%q", metadata, structure, operation)
			}
			if metadata.Complexity.Best == "" || metadata.Complexity.Average == "" || metadata.Complexity.Worst == "" || metadata.Complexity.Memory == "" {
				t.Fatalf("incomplete metadata for %q/%q: %#v", structure, operation, metadata)
			}
		}
	}
}

func TestCatalogDistinguishesUnsupportedOperations(t *testing.T) {
	if _, ok := DefaultCatalog().Lookup(Structure("unknown"), Get); ok {
		t.Fatal("unknown structure unexpectedly returned metadata")
	}
	if _, ok := DefaultCatalog().Lookup(Map, Operation("scan")); ok {
		t.Fatal("unsupported operation unexpectedly returned metadata")
	}
}
