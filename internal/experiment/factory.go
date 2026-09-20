package experiment

import (
	"context"
	"errors"
	"fmt"
	"godatabase/internal/btree"
	"godatabase/internal/engine"
	"godatabase/internal/metrics"
	"godatabase/internal/structures"
	"os"
	"path/filepath"
)

type DefaultFactory struct{ TempDir string }

func (f DefaultFactory) Open(ctx context.Context, kind metrics.Structure) (structures.KV, func() error, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	cleanup := func() error { return nil }
	switch kind {
	case metrics.Map:
		return structures.NewMap(), cleanup, nil
	case metrics.SortedSlice:
		return structures.NewSortedSlice(), cleanup, nil
	case metrics.BTreeMemory:
		return btree.NewMemory(), cleanup, nil
	case metrics.BTreeDurable:
		dir, err := os.MkdirTemp(f.TempDir, "algodb-run-")
		if err != nil {
			return nil, nil, err
		}
		store, err := engine.Open(filepath.Join(dir, "data.db"))
		if err != nil {
			return nil, nil, errors.Join(err, os.RemoveAll(dir))
		}
		return store, func() error { return errors.Join(store.Close(), os.RemoveAll(dir)) }, nil
	default:
		return nil, nil, fmt.Errorf("unsupported structure %q", kind)
	}
}
