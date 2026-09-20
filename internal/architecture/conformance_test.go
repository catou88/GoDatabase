// Package architecture contains contract tests for the repository's layer
// boundaries. It intentionally imports public contracts, not private fields.
package architecture

import (
	"context"
	"testing"

	"godatabase/internal/btree"
	"godatabase/internal/engine"
	"godatabase/internal/metrics"
	"godatabase/internal/server/controllers"
	"godatabase/internal/server/models"
	"godatabase/internal/storage"
	"godatabase/internal/structures"
	"godatabase/internal/trace"
)

var (
	_ structures.KV                = (*btree.KV)(nil)
	_ storage.PageAccess           = (*storage.FilePages)(nil)
	_ engine.Store                 = (*engine.Durable)(nil)
	_ engine.CommitCoordinator     = (*engine.Durable)(nil)
	_ metrics.Catalog              = metrics.DefaultCatalog()
	_ trace.Sink                   = (*trace.Recorder)(nil)
	_ controllers.ExperimentRunner = (*fakeRunner)(nil)
)

type fakeRunner struct{}

func (fakeRunner) Run(context.Context, models.ExperimentRequest) (models.ExperimentResult, error) {
	return models.ExperimentResult{}, nil
}

func TestConstructorWiringUsesInjectedStore(t *testing.T) {
	store := &fakeStore{}
	if engine.New(store) == nil {
		t.Fatal("engine.New returned nil for an injected store")
	}
}

type fakeStore struct{}

func (fakeStore) Get([]byte) ([]byte, bool, error)             { return nil, false, nil }
func (fakeStore) Range([]byte, []byte) ([]engine.Entry, error) { return nil, nil }
func (fakeStore) Set([]byte, []byte) error                     { return nil }
func (fakeStore) Delete([]byte) (bool, error)                  { return false, nil }
func (fakeStore) ApplyBatch([]engine.Mutation) error           { return nil }
func (fakeStore) Close() error                                 { return nil }
