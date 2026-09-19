package btree

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestStorageOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	kv := mustOpenKV(t, path)
	defer closeKV(t, kv)
	for _, kind := range []string{"original", "symlink", "hardlink"} {
		t.Run(kind, func(t *testing.T) {
			alias := path
			if kind != "original" {
				alias = filepath.Join(filepath.Dir(path), kind)
				link := os.Link
				if kind == "symlink" {
					link = os.Symlink
				}
				if err := link(path, alias); err != nil {
					t.Fatal(err)
				}
			}
			other, err := Open(alias)
			if other != nil {
				_ = other.Close()
			}
			if !errors.Is(err, ErrDatabaseLocked) {
				t.Fatalf("second Open: %v", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestStorageOwnershipChild$")
			cmd.Env = append(os.Environ(), "BTREE_LOCK_PATH="+alias)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("child: %v\n%s", err, out)
			}
		})
	}
	closeKV(t, kv)
	reopened := mustOpenKV(t, path)
	closeKV(t, reopened)
}

func TestStorageOwnershipChild(t *testing.T) {
	path := os.Getenv("BTREE_LOCK_PATH")
	if path == "" {
		return
	}
	kv, err := Open(path)
	if kv != nil {
		_ = kv.Close()
	}
	if !errors.Is(err, ErrDatabaseLocked) {
		t.Fatalf("child Open: %v", err)
	}
}

func TestStorageRejectsInvalidExistingMetadata(t *testing.T) {
	for _, size := range []int{1, metadataSize, metadataSize + pageSize} {
		t.Run(stringSize(size), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "database.db")
			if err := os.WriteFile(path, make([]byte, size), 0600); err != nil {
				t.Fatal(err)
			}
			for attempt := 0; attempt < 2; attempt++ {
				kv, err := Open(path)
				if kv != nil {
					_ = kv.Close()
				}
				if !errors.Is(err, errInvalidPageData) {
					t.Fatalf("Open: %v", err)
				}
			}
			// A successful open after repair proves the failed opens released ownership.
			if err := os.Truncate(path, 0); err != nil {
				t.Fatal(err)
			}
			kv := mustOpenKV(t, path)
			closeKV(t, kv)
			kv = mustOpenKV(t, path)
			closeKV(t, kv)
		})
	}
}

func stringSize(size int) string {
	if size == 1 {
		return "short"
	}
	if size == metadataSize {
		return "zero metadata"
	}
	return "zero metadata with pages"
}

func TestStorageFallbackFromImpossibleMetadata(t *testing.T) {
	for _, field := range []string{"root", "allocated non-tree root", "page count", "free list"} {
		t.Run(field, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "database.db")
			kv := recoveryStoreTwoGenerations(t, path)
			slot := int(kv.pm.generation % metadataSlotCount)
			meta, err := readRootMetadataSlot(kv.pm.file, slot)
			if err != nil {
				t.Fatal(err)
			}
			switch field {
			case "root":
				meta.rootPageID = kv.pm.nextPageID + 10
				meta.pageCount = meta.rootPageID
			case "allocated non-tree root":
				id, err := kv.pm.allocatePage()
				if err != nil {
					t.Fatal(err)
				}
				meta.rootPageID = id
				meta.pageCount = id
			case "page count":
				meta.pageCount = kv.pm.nextPageID + 10
			case "free list":
				meta.freeListHeadPageID = meta.pageCount + 1
			}
			if err := writeRootMetadataSlot(kv.pm.file, slot, meta); err != nil {
				t.Fatal(err)
			}
			recoveryCloseAndAssertPrevious(t, kv, path)
		})
	}
}

func TestStorageRepairRetainsOwnership(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	kv := recoveryStoreTwoGenerations(t, path)
	defer closeKV(t, kv)
	injected := errors.New("sync failure")
	kv.hooks.syncMetadata = func() error { return injected }
	if err := kv.Set([]byte("key"), []byte("uncertain")); !errors.Is(err, injected) {
		t.Fatal(err)
	}
	ownedFile := kv.pm.file
	if err := kv.Set([]byte("key"), []byte("repaired")); err != nil {
		t.Fatal(err)
	}
	if kv.pm.file != ownedFile {
		t.Fatal("repair replaced owned descriptor")
	}
	other, err := Open(path)
	if other != nil {
		_ = other.Close()
	}
	if !errors.Is(err, ErrDatabaseLocked) {
		t.Fatal(err)
	}
	assertKVValue(t, kv, "key", "repaired")
}

func TestStorageBothMetadataSlotsCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	kv := recoveryStoreTwoGenerations(t, path)
	if _, err := kv.pm.file.WriteAt(make([]byte, metadataSize), 0); err != nil {
		t.Fatal(err)
	}
	closeKV(t, kv)
	for i := 0; i < 2; i++ {
		opened, err := Open(path)
		if opened != nil {
			_ = opened.Close()
		}
		if !errors.Is(err, errInvalidPageData) {
			t.Fatalf("Open: %v", err)
		}
	}
}

func TestStoragePartialWritesAndSyncFailures(t *testing.T) {
	for _, boundary := range []string{"partial tree", "data sync", "free list write", "free list sync", "partial metadata", "metadata sync"} {
		t.Run(boundary, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "database.db")
			kv := recoveryStoreTwoGenerations(t, path)
			injected := errors.New("injected storage failure")
			switch boundary {
			case "partial tree":
				kv.hooks.beforePageWrite = func(id uint64) error {
					offset, err := dataPageOffset(id)
					if err != nil {
						t.Fatal(err)
					}
					if _, err := kv.pm.file.WriteAt([]byte("partial"), offset); err != nil {
						t.Fatal(err)
					}
					return injected
				}
			case "data sync":
				kv.hooks.syncData = func() error { return injected }
			case "free list write":
				kv.pm.beforeFreeListWrite = func(id uint64) error {
					if err := kv.pm.writePage(id, make([]byte, pageSize)); err != nil {
						t.Fatal(err)
					}
					return injected
				}
			case "free list sync":
				kv.pm.syncFreeList = func() error { return injected }
			case "partial metadata":
				kv.hooks.beforeMetadata = func() error {
					offset := int64(((kv.pm.generation + 1) % metadataSlotCount) * pageSize)
					if _, err := kv.pm.file.WriteAt([]byte("partial"), offset); err != nil {
						t.Fatal(err)
					}
					return injected
				}
			case "metadata sync":
				kv.hooks.syncMetadata = func() error { return injected }
			}
			if err := kv.Set([]byte("key"), []byte("attempted")); !errors.Is(err, injected) {
				t.Fatal(err)
			}
			assertKVValue(t, kv, "key", "latest")
			closeKV(t, kv)
			kv = mustOpenKV(t, path)
			defer closeKV(t, kv)
			// The metadata bytes are complete in this deterministic sync-failure model.
			want := "latest"
			if boundary == "metadata sync" {
				want = "attempted"
			}
			assertKVValue(t, kv, "key", want)
			if err := kv.Set([]byte("key"), []byte("retry")); err != nil {
				t.Fatal(err)
			}
			assertKVValue(t, kv, "key", "retry")
		})
	}
}
