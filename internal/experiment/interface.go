// Package experiment contains contracts for running structure experiments.
package experiment

import "godatabase/internal/structures"

// Runner executes one experiment against a supplied logical structure. The
// concrete workload and result types belong to the owning experiment engine.
type Runner interface {
	Run(structures.KV) error
}
