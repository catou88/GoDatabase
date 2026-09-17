package btree

import (
	"encoding/binary"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func TestRecoveryRejectsInvalidChildPageID(t *testing.T) {
	pm := newTestPageManager(t)
	defer func() { _ = pm.close() }()

	left := recoveryWriteNode(t, pm, recoveryLeaf("", ""))
	root := recoveryWriteNode(t, pm, &pageNode{
		keys:       []string{"", "m"},
		childPages: []uint64{left, pm.nextPageID + 10},
	})
	recoverySelectRoot(pm, root)

	_, err := loadCommittedPages(pm)
	if err == nil || !strings.Contains(err.Error(), "invalid child page id") {
		t.Fatalf("loadCommittedPages() error = %v, want invalid-child error", err)
	}
}

func TestRecoveryRejectsParentChildLowerBoundMismatch(t *testing.T) {
	pm := newTestPageManager(t)
	defer func() { _ = pm.close() }()

	left := recoveryWriteNode(t, pm, recoveryLeaf("", ""))
	right := recoveryWriteNode(t, pm, &pageNode{
		leaf: true, keys: []string{"a"}, values: []string{"one"},
	})
	root := recoveryWriteNode(t, pm, &pageNode{
		keys:       []string{"", "b"},
		childPages: []uint64{left, right},
	})
	recoverySelectRoot(pm, root)

	_, err := loadCommittedPages(pm)
	if err == nil || !strings.Contains(err.Error(), "does not match parent key") {
		t.Fatalf("loadCommittedPages() error = %v, want lower-bound error", err)
	}
}

func TestRecoveryRejectsChildCrossingParentUpperBound(t *testing.T) {
	pm := newTestPageManager(t)
	defer func() { _ = pm.close() }()

	left := recoveryWriteNode(t, pm, &pageNode{
		leaf: true, keys: []string{"", "z"}, values: []string{"", "outside"},
	})
	right := recoveryWriteNode(t, pm, &pageNode{
		leaf: true, keys: []string{"m"}, values: []string{"right"},
	})
	root := recoveryWriteNode(t, pm, &pageNode{
		keys:       []string{"", "m"},
		childPages: []uint64{left, right},
	})
	recoverySelectRoot(pm, root)

	_, err := loadCommittedPages(pm)
	if err == nil || !strings.Contains(err.Error(), "crosses upper bound") {
		t.Fatalf("loadCommittedPages() error = %v, want upper-bound error", err)
	}
}

func TestRecoveryRejectsSingleChildInternalRoot(t *testing.T) {
	pm := newTestPageManager(t)
	defer func() { _ = pm.close() }()

	leaf := recoveryWriteNode(t, pm, recoveryLeaf("", ""))
	root := recoveryWriteNode(t, pm, &pageNode{
		keys:       []string{""},
		childPages: []uint64{leaf},
	})
	recoverySelectRoot(pm, root)

	_, err := loadCommittedPages(pm)
	if err == nil || !strings.Contains(err.Error(), "fewer than two children") {
		t.Fatalf("loadCommittedPages() error = %v, want root-occupancy error", err)
	}
}

func TestRecoveryRejectsUnequalLeafDepth(t *testing.T) {
	pm := newTestPageManager(t)
	defer func() { _ = pm.close() }()

	left := recoveryWriteNode(t, pm, recoveryLeaf("", ""))
	rightLeaf := recoveryWriteNode(t, pm, &pageNode{
		leaf: true, keys: []string{"m"}, values: []string{"right"},
	})
	right := recoveryWriteNode(t, pm, &pageNode{
		keys:       []string{"m"},
		childPages: []uint64{rightLeaf},
	})
	root := recoveryWriteNode(t, pm, &pageNode{
		keys:       []string{"", "m"},
		childPages: []uint64{left, right},
	})
	recoverySelectRoot(pm, root)

	_, err := loadCommittedPages(pm)
	if err == nil || !strings.Contains(err.Error(), "leaf depth") {
		t.Fatalf("loadCommittedPages() error = %v, want leaf-depth error", err)
	}
}

func TestRecoveryRejectsDuplicateAndCyclicPageReferences(t *testing.T) {
	t.Run("duplicate", func(t *testing.T) {
		pm := newTestPageManager(t)
		defer func() { _ = pm.close() }()

		leaf := recoveryWriteNode(t, pm, recoveryLeaf("", ""))
		root := recoveryWriteNode(t, pm, &pageNode{
			keys:       []string{"", "m"},
			childPages: []uint64{leaf, leaf},
		})
		recoverySelectRoot(pm, root)

		_, err := loadCommittedPages(pm)
		if err == nil || !strings.Contains(err.Error(), "more than once") {
			t.Fatalf("loadCommittedPages() error = %v, want duplicate-reference error", err)
		}
	})

	t.Run("cycle", func(t *testing.T) {
		pm := newTestPageManager(t)
		defer func() { _ = pm.close() }()

		rootID, err := pm.allocatePage()
		if err != nil {
			t.Fatalf("allocatePage() error = %v", err)
		}
		leaf := recoveryWriteNode(t, pm, &pageNode{
			leaf: true, keys: []string{"z"}, values: []string{"value"},
		})
		root := recoveryEncodeNode(t, &pageNode{
			keys:       []string{"", "z"},
			childPages: []uint64{rootID, leaf},
		})
		if err := pm.writePage(rootID, root); err != nil {
			t.Fatalf("writePage() error = %v", err)
		}
		recoverySelectRoot(pm, rootID)

		_, err = loadCommittedPages(pm)
		if err == nil || !strings.Contains(err.Error(), "page cycle") {
			t.Fatalf("loadCommittedPages() error = %v, want cycle error", err)
		}
	})
}

func TestRecoveryInterruptedUpdatesPreserveCommittedData(t *testing.T) {
	tests := []struct {
		name    string
		install func(*KV, error)
	}{
		{
			name: "page write",
			install: func(kv *KV, injected error) {
				kv.hooks.beforePageWrite = func(uint64) error { return injected }
			},
		},
		{
			name: "data sync",
			install: func(kv *KV, injected error) {
				kv.hooks.syncData = func() error { return injected }
			},
		},
		{
			name: "metadata write",
			install: func(kv *KV, injected error) {
				kv.hooks.beforeMetadata = func() error { return injected }
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "database.db")
			kv := mustOpenKV(t, path)
			if err := kv.Set([]byte("key"), []byte("committed")); err != nil {
				t.Fatalf("Set(initial) error = %v", err)
			}
			injected := fmt.Errorf("injected %s failure", tt.name)
			tt.install(kv, injected)
			if err := kv.Set([]byte("key"), []byte("interrupted")); !errors.Is(err, injected) {
				t.Fatalf("Set(interrupted) error = %v, want %v", err, injected)
			}
			assertKVValue(t, kv, "key", "committed")
			if err := kv.Close(); err != nil {
				t.Fatalf("Close() error = %v", err)
			}

			kv = mustOpenKV(t, path)
			defer closeKV(t, kv)
			assertKVValue(t, kv, "key", "committed")
		})
	}
}

func TestRecoveryMetadataSyncInterruptionFallsBackDeterministically(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	kv := mustOpenKV(t, path)
	if err := kv.Set([]byte("key"), []byte("committed")); err != nil {
		t.Fatalf("Set(initial) error = %v", err)
	}

	injected := errors.New("injected metadata sync failure")
	kv.hooks.syncMetadata = func() error { return injected }
	if err := kv.Set([]byte("key"), []byte("uncertain")); !errors.Is(err, injected) {
		t.Fatalf("Set(interrupted) error = %v, want %v", err, injected)
	}
	if !kv.uncertain {
		t.Fatal("metadata sync failure did not mark state uncertain")
	}

	interruptedSlot := int((kv.pm.generation + 1) % metadataSlotCount)
	if _, err := kv.pm.file.WriteAt([]byte("bad"), int64(interruptedSlot*pageSize)); err != nil {
		t.Fatalf("corrupt interrupted metadata slot: %v", err)
	}
	if err := kv.pm.file.Sync(); err != nil {
		t.Fatalf("sync interrupted metadata: %v", err)
	}
	if err := kv.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	kv = mustOpenKV(t, path)
	defer closeKV(t, kv)
	assertKVValue(t, kv, "key", "committed")
}

func TestRecoveryFallsBackFromCorruptLatestStates(t *testing.T) {
	t.Run("metadata", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "database.db")
		kv := recoveryStoreTwoGenerations(t, path)
		latestSlot := int(kv.pm.generation % metadataSlotCount)
		if _, err := kv.pm.file.WriteAt([]byte("bad"), int64(latestSlot*pageSize)); err != nil {
			t.Fatalf("corrupt metadata: %v", err)
		}
		recoveryCloseAndAssertPrevious(t, kv, path)
	})

	t.Run("tree page", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "database.db")
		kv := recoveryStoreTwoGenerations(t, path)
		if err := kv.pm.writePage(kv.tree.root, make([]byte, pageSize)); err != nil {
			t.Fatalf("corrupt tree page: %v", err)
		}
		recoveryCloseAndAssertPrevious(t, kv, path)
	})

	t.Run("free list page", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "database.db")
		kv := mustOpenKV(t, path)
		if err := kv.Set([]byte("key"), []byte("previous")); err != nil {
			t.Fatalf("Set(previous) error = %v", err)
		}
		if err := kv.Set([]byte("key"), []byte("latest")); err != nil {
			t.Fatalf("Set(latest) error = %v", err)
		}
		if len(kv.pm.freeListPageIDs) == 0 {
			t.Fatal("latest generation has no free-list page")
		}
		if err := kv.pm.writePage(kv.pm.freeListPageIDs[0], make([]byte, pageSize)); err != nil {
			t.Fatalf("corrupt free-list page: %v", err)
		}
		recoveryCloseAndAssertPrevious(t, kv, path)
	})
}

func TestRecoveryNeverReclaimsLivePages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	kv := mustOpenKV(t, path)
	defer closeKV(t, kv)

	for i := 0; i < 24; i++ {
		key := []byte(largeTestKey(i))
		if err := kv.Set(key, []byte(strings.Repeat("v", maxValueSize))); err != nil {
			t.Fatalf("Set(%d) error = %v", i, err)
		}
	}
	for i := 0; i < 12; i++ {
		deleted, err := kv.Delete([]byte(largeTestKey(i)))
		if err != nil || !deleted {
			t.Fatalf("Delete(%d) = %v, %v; want true, nil", i, deleted, err)
		}
	}

	live := recoveryReachablePageIDs(kv.pages, kv.tree.root)
	for pageID := range live {
		if containsPageID(kv.pm.freePageIDs, pageID) ||
			containsPageID(kv.pm.retiredPageIDs, pageID) ||
			containsPageID(kv.pm.pendingFreePageIDs, pageID) ||
			containsPageID(kv.pm.freeListPageIDs, pageID) {
			t.Fatalf("live page %d appears in a reclaimable or free-list set", pageID)
		}
	}

	for i := 12; i < 24; i++ {
		assertKVValue(t, kv, largeTestKey(i), strings.Repeat("v", maxValueSize))
	}
}

func TestRecoveryDetectsAndQuarantinesUnreachablePages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	kv := mustOpenKV(t, path)
	if err := kv.Set([]byte("key"), []byte("committed")); err != nil {
		t.Fatalf("Set(initial) error = %v", err)
	}
	orphanID, err := kv.pm.allocatePage()
	if err != nil {
		t.Fatalf("allocate orphan page: %v", err)
	}
	if err := kv.pm.writePage(orphanID, testPage('x')); err != nil {
		t.Fatalf("write orphan page: %v", err)
	}
	if err := kv.pm.file.Sync(); err != nil {
		t.Fatalf("sync orphan page: %v", err)
	}
	if err := kv.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	kv = mustOpenKV(t, path)
	defer closeKV(t, kv)
	if !containsPageID(kv.pm.pendingFreePageIDs, orphanID) {
		t.Fatalf("unreachable page %d was not detected and quarantined", orphanID)
	}
	if containsPageID(kv.pm.freePageIDs, orphanID) {
		t.Fatalf("unreachable page %d became reusable before a commit", orphanID)
	}
	assertKVValue(t, kv, "key", "committed")

	if err := kv.Set([]byte("second"), []byte("value")); err != nil {
		t.Fatalf("Set(first recovery commit) error = %v", err)
	}
	if !containsPageID(kv.pm.retiredPageIDs, orphanID) {
		t.Fatalf("orphan page %d was not protected in committed metadata", orphanID)
	}
	if err := kv.Set([]byte("third"), []byte("value")); err != nil {
		t.Fatalf("Set(second recovery commit) error = %v", err)
	}
	if !containsPageID(kv.pm.freePageIDs, orphanID) {
		t.Fatalf("orphan page %d did not cross the reuse boundary", orphanID)
	}
	assertKVValue(t, kv, "key", "committed")
}

func recoveryStoreTwoGenerations(t *testing.T, path string) *KV {
	t.Helper()
	kv := mustOpenKV(t, path)
	if err := kv.Set([]byte("key"), []byte("previous")); err != nil {
		t.Fatalf("Set(previous) error = %v", err)
	}
	if err := kv.Set([]byte("key"), []byte("latest")); err != nil {
		t.Fatalf("Set(latest) error = %v", err)
	}
	return kv
}

func recoveryCloseAndAssertPrevious(t *testing.T, kv *KV, path string) {
	t.Helper()
	if err := kv.pm.file.Sync(); err != nil {
		t.Fatalf("Sync(corruption) error = %v", err)
	}
	if err := kv.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	reopened := mustOpenKV(t, path)
	defer closeKV(t, reopened)
	assertKVValue(t, reopened, "key", "previous")
}

func recoveryLeaf(key, value string) *pageNode {
	keys := []string{key}
	values := []string{value}
	if key != "" {
		keys = []string{"", key}
		values = []string{"", value}
	}
	return &pageNode{leaf: true, keys: keys, values: values}
}

func recoveryWriteNode(t *testing.T, pm *pageManager, node *pageNode) uint64 {
	t.Helper()
	pageID, err := pm.allocatePage()
	if err != nil {
		t.Fatalf("allocatePage() error = %v", err)
	}
	if err := pm.writePage(pageID, recoveryEncodeNode(t, node)); err != nil {
		t.Fatalf("writePage(%d) error = %v", pageID, err)
	}
	return pageID
}

func recoverySelectRoot(pm *pageManager, root uint64) {
	pm.rootPageID = root
	pm.committedPageCount = pm.nextPageID - 1
}

func recoveryEncodeNode(t *testing.T, node *pageNode) []byte {
	t.Helper()
	page, err := encodePageNode(node)
	if err != nil {
		t.Fatalf("encodePageNode() error = %v", err)
	}
	return page
}

func recoveryReachablePageIDs(pages map[uint64]BNode, root uint64) map[uint64]struct{} {
	reachable := make(map[uint64]struct{})
	var walk func(uint64)
	walk = func(pageID uint64) {
		if pageID == 0 {
			return
		}
		if _, seen := reachable[pageID]; seen {
			return
		}
		reachable[pageID] = struct{}{}
		node := pages[pageID]
		if node.btype() == nodeTypeInternal {
			for i := uint16(0); i < node.nkeys(); i++ {
				walk(node.getPtr(i))
			}
		}
	}
	walk(root)
	return reachable
}

func TestRecoveryRejectsCorruptMetadataChecksum(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	kv := recoveryStoreTwoGenerations(t, path)
	latestSlot := int(kv.pm.generation % metadataSlotCount)

	page := make([]byte, pageSize)
	if _, err := kv.pm.file.ReadAt(page, int64(latestSlot*pageSize)); err != nil {
		t.Fatalf("ReadAt(metadata) error = %v", err)
	}
	binary.LittleEndian.PutUint32(page[metadataChecksumOffset:], 0)
	if _, err := kv.pm.file.WriteAt(page, int64(latestSlot*pageSize)); err != nil {
		t.Fatalf("WriteAt(metadata) error = %v", err)
	}
	recoveryCloseAndAssertPrevious(t, kv, path)
}

func TestRecoveryReturnsClearErrorWhenEveryCommittedTreeIsCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "database.db")
	kv := recoveryStoreTwoGenerations(t, path)

	metadata := make([]rootMetadata, 0, metadataSlotCount)
	for slot := 0; slot < metadataSlotCount; slot++ {
		candidate, err := readRootMetadataSlot(kv.pm.file, slot)
		if err != nil {
			t.Fatalf("readRootMetadataSlot(%d) error = %v", slot, err)
		}
		metadata = append(metadata, candidate)
	}
	for _, candidate := range metadata {
		if candidate.rootPageID != 0 {
			if err := kv.pm.writePage(candidate.rootPageID, make([]byte, pageSize)); err != nil {
				t.Fatalf("corrupt root %d: %v", candidate.rootPageID, err)
			}
		}
	}
	if err := kv.pm.file.Sync(); err != nil {
		t.Fatalf("Sync(corruption) error = %v", err)
	}
	if err := kv.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	opened, err := Open(path)
	if err == nil {
		_ = opened.Close()
		t.Fatal("Open() error = nil, want corrupt-tree error")
	}
	if !strings.Contains(err.Error(), "no valid committed B+Tree") || !errors.Is(err, errInvalidMagic) {
		t.Fatalf("Open() error = %v, want clear invalid-tree error", err)
	}
}
