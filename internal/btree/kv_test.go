package btree

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestKVPersistsSplitsOverwritesMergesAndDeletes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	kv, err := Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	keys := make([]string, 24)
	for i := range keys {
		keys[i] = largeTestKey(i)
		if err := kv.Set([]byte(keys[i]), []byte(strings.Repeat("v", maxValueSize))); err != nil {
			t.Fatalf("Set(%d) error = %v", i, err)
		}
	}
	if pageTreeHeightFromPages(kv.pages, kv.tree.root) < 3 {
		t.Fatalf("tree height = %d, want at least 3", pageTreeHeightFromPages(kv.pages, kv.tree.root))
	}
	if err := kv.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	kv = mustOpenKV(t, path)
	if err := kv.Set([]byte(keys[12]), []byte("overwritten")); err != nil {
		t.Fatalf("Set(overwrite) error = %v", err)
	}
	if err := kv.Close(); err != nil {
		t.Fatalf("Close after overwrite error = %v", err)
	}

	kv = mustOpenKV(t, path)
	assertKVValue(t, kv, keys[12], "overwritten")
	for i := 0; i < len(keys)-2; i++ {
		deleted, err := kv.Delete([]byte(keys[i]))
		if err != nil {
			t.Fatalf("Delete(%d) error = %v", i, err)
		}
		if !deleted {
			t.Fatalf("Delete(%d) = false, want true", i)
		}
	}
	if err := kv.Close(); err != nil {
		t.Fatalf("Close after merges error = %v", err)
	}

	kv = mustOpenKV(t, path)
	defer closeKV(t, kv)
	for i := 0; i < len(keys)-2; i++ {
		if _, found, err := kv.Get([]byte(keys[i])); err != nil || found {
			t.Fatalf("Get(deleted %d) = found %v, error %v; want false, nil", i, found, err)
		}
	}
	for i := len(keys) - 2; i < len(keys); i++ {
		if _, found, err := kv.Get([]byte(keys[i])); err != nil || !found {
			t.Fatalf("Get(live %d) = found %v, error %v; want true, nil", i, found, err)
		}
	}
	for i := len(keys) - 2; i < len(keys); i++ {
		if deleted, err := kv.Delete([]byte(keys[i])); err != nil || !deleted {
			t.Fatalf("Delete(final %d) = %v, %v; want true, nil", i, deleted, err)
		}
	}
	if kv.tree.root != 0 {
		t.Fatalf("root = %d, want empty tree", kv.tree.root)
	}
	if err := kv.Close(); err != nil {
		t.Fatalf("Close empty database error = %v", err)
	}
	kv = mustOpenKV(t, path)
	defer closeKV(t, kv)
	if _, found, err := kv.Get([]byte(keys[len(keys)-1])); err != nil || found {
		t.Fatalf("Get after empty reopen = found %v, error %v; want false, nil", found, err)
	}
}

func TestKVCommitOrdersPageWritesBeforeMetadata(t *testing.T) {
	kv := mustOpenKV(t, filepath.Join(t.TempDir(), "database.db"))
	defer closeKV(t, kv)

	events := make([]string, 0, 4)
	kv.hooks.beforePageWrite = func(uint64) error {
		events = append(events, "write-page")
		return nil
	}
	kv.hooks.syncData = func() error {
		events = append(events, "sync-data")
		return kv.pm.file.Sync()
	}
	kv.hooks.beforeMetadata = func() error {
		events = append(events, "write-metadata")
		return nil
	}
	kv.hooks.syncMetadata = func() error {
		events = append(events, "sync-metadata")
		return kv.pm.file.Sync()
	}

	if err := kv.Set([]byte("alpha"), []byte("one")); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	want := []string{"write-page", "sync-data", "write-metadata", "sync-metadata"}
	if fmt.Sprint(events) != fmt.Sprint(want) {
		t.Fatalf("commit events = %v, want %v", events, want)
	}
}

func TestKVFailedPageWritePreservesCommittedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	kv := mustOpenKV(t, path)
	if err := kv.Set([]byte("alpha"), []byte("old")); err != nil {
		t.Fatalf("Set(initial) error = %v", err)
	}

	injected := errors.New("injected page write failure")
	kv.hooks.beforePageWrite = func(uint64) error { return injected }
	if err := kv.Set([]byte("alpha"), []byte("new")); !errors.Is(err, injected) {
		t.Fatalf("Set(failing) error = %v, want %v", err, injected)
	}
	assertKVValue(t, kv, "alpha", "old")
	if err := kv.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	kv = mustOpenKV(t, path)
	defer closeKV(t, kv)
	assertKVValue(t, kv, "alpha", "old")
}

func TestKVFailedDataSyncPreservesCommittedState(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	kv := mustOpenKV(t, path)
	if err := kv.Set([]byte("alpha"), []byte("old")); err != nil {
		t.Fatalf("Set(initial) error = %v", err)
	}

	injected := errors.New("injected data sync failure")
	kv.hooks.syncData = func() error { return injected }
	if err := kv.Set([]byte("alpha"), []byte("new")); !errors.Is(err, injected) {
		t.Fatalf("Set(failing) error = %v, want %v", err, injected)
	}
	assertKVValue(t, kv, "alpha", "old")
	if err := kv.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	kv = mustOpenKV(t, path)
	defer closeKV(t, kv)
	assertKVValue(t, kv, "alpha", "old")
}

func TestKVRepairsStateBeforeRetryAfterMetadataFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	kv := mustOpenKV(t, path)
	defer closeKV(t, kv)
	if err := kv.Set([]byte("alpha"), []byte("old")); err != nil {
		t.Fatalf("Set(initial) error = %v", err)
	}

	injected := errors.New("injected metadata failure")
	kv.hooks.beforeMetadata = func() error { return injected }
	if err := kv.Set([]byte("alpha"), []byte("new")); !errors.Is(err, injected) {
		t.Fatalf("Set(failing) error = %v, want %v", err, injected)
	}
	assertKVValue(t, kv, "alpha", "old")
	if !kv.uncertain {
		t.Fatal("metadata failure did not mark state uncertain")
	}

	if err := kv.Set([]byte("beta"), []byte("two")); err != nil {
		t.Fatalf("Set after repair error = %v", err)
	}
	if kv.uncertain {
		t.Fatal("successful retry left state uncertain")
	}
	assertKVValue(t, kv, "alpha", "old")
	assertKVValue(t, kv, "beta", "two")
}

func TestKVOpenFallsBackWhenLatestCommittedRootIsCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	kv := mustOpenKV(t, path)
	if err := kv.Set([]byte("alpha"), []byte("old")); err != nil {
		t.Fatalf("Set(old) error = %v", err)
	}
	if err := kv.Set([]byte("alpha"), []byte("new")); err != nil {
		t.Fatalf("Set(new) error = %v", err)
	}
	latestRoot := kv.tree.root
	if err := kv.pm.writePage(latestRoot, make([]byte, pageSize)); err != nil {
		t.Fatalf("corrupt latest root: %v", err)
	}
	if err := kv.pm.file.Sync(); err != nil {
		t.Fatalf("sync corrupted root: %v", err)
	}
	if err := kv.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	kv = mustOpenKV(t, path)
	defer closeKV(t, kv)
	assertKVValue(t, kv, "alpha", "old")
}

func TestKVRepeatedUpdatesAndDeletesReusePages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	kv := mustOpenKV(t, path)
	defer closeKV(t, kv)

	for i := 0; i < 12; i++ {
		if err := kv.Set([]byte("key"), []byte(fmt.Sprintf("warm-%d", i))); err != nil {
			t.Fatalf("warmup Set(%d) error = %v", i, err)
		}
	}
	warmInfo, err := kv.pm.file.Stat()
	if err != nil {
		t.Fatalf("Stat(warm) error = %v", err)
	}

	for i := 0; i < 40; i++ {
		deleted, err := kv.Delete([]byte("key"))
		if err != nil || !deleted {
			t.Fatalf("Delete(%d) = %v, %v; want true, nil", i, deleted, err)
		}
		if err := kv.Set([]byte("key"), []byte(fmt.Sprintf("value-%d", i))); err != nil {
			t.Fatalf("Set(%d) error = %v", i, err)
		}
	}
	finalInfo, err := kv.pm.file.Stat()
	if err != nil {
		t.Fatalf("Stat(final) error = %v", err)
	}
	if finalInfo.Size() > warmInfo.Size()+2*pageSize {
		t.Fatalf("file grew from %d to %d during steady reuse", warmInfo.Size(), finalInfo.Size())
	}
	assertKVValue(t, kv, "key", "value-39")
}

func TestKVRejectsInvalidPathsAndClosedOperations(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Fatal("Open(empty path) error = nil, want error")
	}
	directory := t.TempDir()
	if _, err := Open(directory); err == nil {
		t.Fatal("Open(directory) error = nil, want error")
	}
	if _, err := Open(filepath.Join(directory, "missing", "database.db")); err == nil {
		t.Fatal("Open(missing parent) error = nil, want error")
	}

	kv := mustOpenKV(t, filepath.Join(directory, "database.db"))
	if err := kv.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if _, _, err := kv.Get([]byte("key")); !errors.Is(err, ErrClosed) {
		t.Fatalf("Get() error = %v, want %v", err, ErrClosed)
	}
	if err := kv.Set([]byte("key"), []byte("value")); !errors.Is(err, ErrClosed) {
		t.Fatalf("Set() error = %v, want %v", err, ErrClosed)
	}
	if _, err := kv.Delete([]byte("key")); !errors.Is(err, ErrClosed) {
		t.Fatalf("Delete() error = %v, want %v", err, ErrClosed)
	}
}

func mustOpenKV(t *testing.T, path string) *KV {
	t.Helper()
	kv, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%q) error = %v", path, err)
	}
	return kv
}

func closeKV(t *testing.T, kv *KV) {
	t.Helper()
	if err := kv.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func assertKVValue(t *testing.T, kv *KV, key, want string) {
	t.Helper()
	got, found, err := kv.Get([]byte(key))
	if err != nil {
		t.Fatalf("Get(%q) error = %v", key, err)
	}
	if !found || string(got) != want {
		t.Fatalf("Get(%q) = (%q, %v), want (%q, true)", key, got, found, want)
	}
}

func largeTestKey(index int) string {
	return fmt.Sprintf("%03d-%s", index, strings.Repeat("k", 900))
}

func pageTreeHeightFromPages(pages map[uint64]BNode, root uint64) int {
	height := 0
	for root != 0 {
		height++
		node := pages[root]
		if node.btype() == nodeTypeLeaf {
			break
		}
		root = node.getPtr(0)
	}
	return height
}
