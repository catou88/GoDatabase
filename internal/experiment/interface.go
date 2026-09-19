// Package experiment contains contracts for running structure experiments.
package experiment

import (
	"godatabase/internal/metrics"
	"godatabase/internal/structures"
)

// Runner executes one experiment against a supplied logical structure. The
// concrete workload and result types belong to the owning experiment engine.
type Runner interface {
	Run(structures.KV) error
}

// ComplexityProvider exposes theoretical operation metadata separately from
// measured experiment results.
type ComplexityProvider interface {
	Complexity(structure metrics.Structure, operation metrics.Operation) (metrics.Metadata, bool)
}

// DefaultComplexityProvider returns the shared complexity catalog used by
// experiment responses.
func DefaultComplexityProvider() ComplexityProvider {
	return complexityProvider{catalog: metrics.DefaultCatalog()}
}

type complexityProvider struct{ catalog metrics.Catalog }

func (p complexityProvider) Complexity(structure metrics.Structure, operation metrics.Operation) (metrics.Metadata, bool) {
	return p.catalog.Lookup(structure, operation)
}
