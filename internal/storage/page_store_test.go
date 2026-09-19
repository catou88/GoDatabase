package storage

import (
	"errors"
	"os"
	"testing"
)

func TestFilePagesRoundTripAndAllocation(t *testing.T) {
	directory := t.TempDir()
	file, err := os.OpenFile(directory+"/pages", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("close page file: %v", err)
		}
	}()

	pages, err := OpenFilePages(file, 64)
	if err != nil {
		t.Fatal(err)
	}
	pageID, err := pages.AllocatePage()
	if err != nil {
		t.Fatal(err)
	}
	want := make([]byte, 64)
	want[3] = 0x7f
	if err := pages.WritePage(pageID, want); err != nil {
		t.Fatal(err)
	}
	got, err := pages.ReadPage(pageID)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("page mismatch: got %v, want %v", got, want)
	}
}

func TestFilePagesRejectsInvalidOperations(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "pages")
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := file.Close(); err != nil {
			t.Errorf("close page file: %v", err)
		}
	}()
	pages, err := OpenFilePages(file, 64)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pages.ReadPage(1); !errors.Is(err, ErrInvalidPageID) {
		t.Fatalf("ReadPage(1) error = %v", err)
	}
	if err := pages.WritePage(1, make([]byte, 63)); !errors.Is(err, ErrInvalidPageID) {
		t.Fatalf("WritePage invalid ID error = %v", err)
	}
	if _, err := file.Write([]byte{1}); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenFilePages(file, 64); err == nil {
		t.Fatal("OpenFilePages accepted a non-page-aligned file")
	}
}
