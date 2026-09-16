package btree

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestPageManagerAllocatesWritesReadsAndPersistsPages(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pages.db")

	pm, err := openPageManager(path)
	if err != nil {
		t.Fatalf("openPageManager() error = %v", err)
	}

	firstID, err := pm.allocatePage()
	if err != nil {
		t.Fatalf("allocatePage() error = %v", err)
	}
	if firstID != 1 {
		t.Fatalf("first page id = %d, want 1", firstID)
	}

	secondID, err := pm.allocatePage()
	if err != nil {
		t.Fatalf("allocatePage() error = %v", err)
	}
	if secondID != 2 {
		t.Fatalf("second page id = %d, want 2", secondID)
	}

	firstPage := testPage('a')
	secondPage := testPage('b')
	if err := pm.writePage(firstID, firstPage); err != nil {
		t.Fatalf("writePage(first) error = %v", err)
	}
	if err := pm.writePage(secondID, secondPage); err != nil {
		t.Fatalf("writePage(second) error = %v", err)
	}
	if err := pm.close(); err != nil {
		t.Fatalf("close() error = %v", err)
	}

	reopened, err := openPageManager(path)
	if err != nil {
		t.Fatalf("reopen page manager error = %v", err)
	}
	defer func() {
		if err := reopened.close(); err != nil {
			t.Fatalf("close reopened manager error = %v", err)
		}
	}()

	assertPageBytesEqual(t, reopened, firstID, firstPage)
	assertPageBytesEqual(t, reopened, secondID, secondPage)

	thirdID, err := reopened.allocatePage()
	if err != nil {
		t.Fatalf("allocatePage() after reopen error = %v", err)
	}
	if thirdID != 3 {
		t.Fatalf("third page id = %d, want 3", thirdID)
	}
}

func TestPageManagerRejectsInvalidPageIDs(t *testing.T) {
	pm := newTestPageManager(t)
	defer func() {
		if err := pm.close(); err != nil {
			t.Fatalf("close() error = %v", err)
		}
	}()

	pageID, err := pm.allocatePage()
	if err != nil {
		t.Fatalf("allocatePage() error = %v", err)
	}

	tests := []struct {
		name   string
		pageID uint64
	}{
		{name: "zero page id", pageID: 0},
		{name: "unallocated page id", pageID: pageID + 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := pm.readPage(tt.pageID); !errors.Is(err, errInvalidPageID) {
				t.Fatalf("readPage() error = %v, want %v", err, errInvalidPageID)
			}
			if err := pm.writePage(tt.pageID, testPage('x')); !errors.Is(err, errInvalidPageID) {
				t.Fatalf("writePage() error = %v, want %v", err, errInvalidPageID)
			}
		})
	}
}

func TestPageManagerRejectsInvalidPageData(t *testing.T) {
	pm := newTestPageManager(t)
	defer func() {
		if err := pm.close(); err != nil {
			t.Fatalf("close() error = %v", err)
		}
	}()

	pageID, err := pm.allocatePage()
	if err != nil {
		t.Fatalf("allocatePage() error = %v", err)
	}

	if err := pm.writePage(pageID, make([]byte, pageSize-1)); !errors.Is(err, errInvalidPageData) {
		t.Fatalf("writePage(short page) error = %v, want %v", err, errInvalidPageData)
	}
	if err := pm.writePage(pageID, make([]byte, pageSize+1)); !errors.Is(err, errInvalidPageData) {
		t.Fatalf("writePage(large page) error = %v, want %v", err, errInvalidPageData)
	}
}

func TestOpenPageManagerRejectsUnalignedFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pages.db")
	if err := os.WriteFile(path, make([]byte, pageSize-1), 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	pm, err := openPageManager(path)
	if err == nil {
		if closeErr := pm.close(); closeErr != nil {
			t.Fatalf("close() error = %v", closeErr)
		}
		t.Fatal("expected unaligned file error")
	}
	if !errors.Is(err, errInvalidPageData) {
		t.Fatalf("openPageManager() error = %v, want %v", err, errInvalidPageData)
	}
}

func TestPageManagerHandlesShortReads(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pages.db")

	pm, err := openPageManager(path)
	if err != nil {
		t.Fatalf("openPageManager() error = %v", err)
	}
	defer func() {
		if err := pm.close(); err != nil {
			t.Fatalf("close() error = %v", err)
		}
	}()

	pageID, err := pm.allocatePage()
	if err != nil {
		t.Fatalf("allocatePage() error = %v", err)
	}
	if err := pm.file.Truncate(metadataSize + pageSize - 1); err != nil {
		t.Fatalf("Truncate() error = %v", err)
	}

	_, err = pm.readPage(pageID)
	if !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		t.Fatalf("readPage() error = %v, want short read error", err)
	}
}

func TestPageManagerReopensCommittedRoot(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pages.db")

	pm, err := openPageManager(path)
	if err != nil {
		t.Fatalf("openPageManager() error = %v", err)
	}

	rootPage := mustWritePageNode(t, pm, &pageNode{
		leaf:   true,
		keys:   []string{"a"},
		values: []string{"one"},
	})
	if err := pm.commitRoot(rootPage); err != nil {
		t.Fatalf("commitRoot() error = %v", err)
	}
	if err := pm.close(); err != nil {
		t.Fatalf("close() error = %v", err)
	}

	reopened, err := openPageManager(path)
	if err != nil {
		t.Fatalf("reopen page manager error = %v", err)
	}
	defer func() {
		if err := reopened.close(); err != nil {
			t.Fatalf("close reopened manager error = %v", err)
		}
	}()

	if got := reopened.rootPage(); got != rootPage {
		t.Fatalf("rootPage() = %d, want %d", got, rootPage)
	}
	got := mustReadPageNode(t, reopened, reopened.rootPage())
	assertPageNodeEqual(t, got, &pageNode{
		leaf:   true,
		keys:   []string{"a"},
		values: []string{"one"},
	})
}

func TestPageManagerKeepsOldRootWhenWriteIsInterruptedBeforeCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pages.db")

	pm, err := openPageManager(path)
	if err != nil {
		t.Fatalf("openPageManager() error = %v", err)
	}

	oldRoot := mustWritePageNode(t, pm, &pageNode{
		leaf:   true,
		keys:   []string{"a"},
		values: []string{"old"},
	})
	if err := pm.commitRoot(oldRoot); err != nil {
		t.Fatalf("commitRoot(old root) error = %v", err)
	}

	newRoot := mustWritePageNode(t, pm, &pageNode{
		leaf:   true,
		keys:   []string{"a"},
		values: []string{"new"},
	})
	if newRoot == oldRoot {
		t.Fatal("new root reused old root page")
	}
	if err := pm.close(); err != nil {
		t.Fatalf("close() error = %v", err)
	}

	reopened, err := openPageManager(path)
	if err != nil {
		t.Fatalf("reopen page manager error = %v", err)
	}
	defer func() {
		if err := reopened.close(); err != nil {
			t.Fatalf("close reopened manager error = %v", err)
		}
	}()

	if got := reopened.rootPage(); got != oldRoot {
		t.Fatalf("rootPage() = %d, want old root %d", got, oldRoot)
	}
	got := mustReadPageNode(t, reopened, reopened.rootPage())
	assertPageNodeEqual(t, got, &pageNode{
		leaf:   true,
		keys:   []string{"a"},
		values: []string{"old"},
	})
}

func TestPageManagerPublishesNewRootAtCommitPoint(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pages.db")

	pm, err := openPageManager(path)
	if err != nil {
		t.Fatalf("openPageManager() error = %v", err)
	}

	oldRoot := mustWritePageNode(t, pm, &pageNode{
		leaf:   true,
		keys:   []string{"a"},
		values: []string{"old"},
	})
	if err := pm.commitRoot(oldRoot); err != nil {
		t.Fatalf("commitRoot(old root) error = %v", err)
	}

	newRoot := mustWritePageNode(t, pm, &pageNode{
		leaf:   true,
		keys:   []string{"a"},
		values: []string{"new"},
	})
	if err := pm.commitRoot(newRoot); err != nil {
		t.Fatalf("commitRoot(new root) error = %v", err)
	}
	if err := pm.close(); err != nil {
		t.Fatalf("close() error = %v", err)
	}

	reopened, err := openPageManager(path)
	if err != nil {
		t.Fatalf("reopen page manager error = %v", err)
	}
	defer func() {
		if err := reopened.close(); err != nil {
			t.Fatalf("close reopened manager error = %v", err)
		}
	}()

	if got := reopened.rootPage(); got != newRoot {
		t.Fatalf("rootPage() = %d, want new root %d", got, newRoot)
	}
	got := mustReadPageNode(t, reopened, reopened.rootPage())
	assertPageNodeEqual(t, got, &pageNode{
		leaf:   true,
		keys:   []string{"a"},
		values: []string{"new"},
	})
}

func TestPageManagerFallsBackToPreviousRootWhenLatestMetadataIsCorrupt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pages.db")

	pm, err := openPageManager(path)
	if err != nil {
		t.Fatalf("openPageManager() error = %v", err)
	}

	oldRoot := mustWritePageNode(t, pm, &pageNode{
		leaf:   true,
		keys:   []string{"a"},
		values: []string{"old"},
	})
	if err := pm.commitRoot(oldRoot); err != nil {
		t.Fatalf("commitRoot(old root) error = %v", err)
	}

	newRoot := mustWritePageNode(t, pm, &pageNode{
		leaf:   true,
		keys:   []string{"a"},
		values: []string{"new"},
	})
	if err := pm.commitRoot(newRoot); err != nil {
		t.Fatalf("commitRoot(new root) error = %v", err)
	}
	if err := pm.close(); err != nil {
		t.Fatalf("close() error = %v", err)
	}

	file, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatalf("OpenFile() error = %v", err)
	}
	if _, err := file.WriteAt([]byte("bad"), int64((2%metadataSlotCount)*pageSize)); err != nil {
		t.Fatalf("corrupt metadata slot error = %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close corrupting file error = %v", err)
	}

	reopened, err := openPageManager(path)
	if err != nil {
		t.Fatalf("reopen page manager error = %v", err)
	}
	defer func() {
		if err := reopened.close(); err != nil {
			t.Fatalf("close reopened manager error = %v", err)
		}
	}()

	if got := reopened.rootPage(); got != oldRoot {
		t.Fatalf("rootPage() = %d, want previous root %d", got, oldRoot)
	}
}

func newTestPageManager(t *testing.T) *pageManager {
	t.Helper()

	pm, err := openPageManager(filepath.Join(t.TempDir(), "pages.db"))
	if err != nil {
		t.Fatalf("openPageManager() error = %v", err)
	}
	return pm
}

func testPage(fill byte) []byte {
	page := make([]byte, pageSize)
	for i := range page {
		page[i] = fill
	}
	return page
}

func assertPageBytesEqual(t *testing.T, pm *pageManager, pageID uint64, want []byte) {
	t.Helper()

	got, err := pm.readPage(pageID)
	if err != nil {
		t.Fatalf("readPage(%d) error = %v", pageID, err)
	}
	if len(got) != len(want) {
		t.Fatalf("readPage(%d) length = %d, want %d", pageID, len(got), len(want))
	}
	for i, gotByte := range got {
		if gotByte != want[i] {
			t.Fatalf("readPage(%d)[%d] = %d, want %d", pageID, i, gotByte, want[i])
		}
	}
}

func mustWritePageNode(t *testing.T, pm *pageManager, node *pageNode) uint64 {
	t.Helper()

	page, err := encodePageNode(node)
	if err != nil {
		t.Fatalf("encodePageNode() error = %v", err)
	}
	pageID, err := pm.allocatePage()
	if err != nil {
		t.Fatalf("allocatePage() error = %v", err)
	}
	if err := pm.writePage(pageID, page); err != nil {
		t.Fatalf("writePage() error = %v", err)
	}
	return pageID
}

func mustReadPageNode(t *testing.T, pm *pageManager, pageID uint64) *pageNode {
	t.Helper()

	page, err := pm.readPage(pageID)
	if err != nil {
		t.Fatalf("readPage() error = %v", err)
	}
	node, err := decodePageNode(page)
	if err != nil {
		t.Fatalf("decodePageNode() error = %v", err)
	}
	return node
}
