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
	if err := pm.file.Truncate(pageSize - 1); err != nil {
		t.Fatalf("Truncate() error = %v", err)
	}

	_, err = pm.readPage(pageID)
	if !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		t.Fatalf("readPage() error = %v, want short read error", err)
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
