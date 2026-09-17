package btree

import (
	"errors"
	"fmt"
	"testing"
)

func TestFreeListNodeRoundTrip(t *testing.T) {
	want := freeListNode{
		nextPageID: 99,
		sequence:   508,
		pageIDs:    []uint64{3, 7, 11, 15},
	}
	page, err := encodeFreeListNode(want)
	if err != nil {
		t.Fatalf("encodeFreeListNode() error = %v", err)
	}
	got, err := decodeFreeListNode(page)
	if err != nil {
		t.Fatalf("decodeFreeListNode() error = %v", err)
	}
	if got.nextPageID != want.nextPageID || got.sequence != want.sequence ||
		fmt.Sprint(got.pageIDs) != fmt.Sprint(want.pageIDs) {
		t.Fatalf("decoded node = %+v, want %+v", got, want)
	}
}

func TestFreeListNodeRejectsCorruption(t *testing.T) {
	page, err := encodeFreeListNode(freeListNode{pageIDs: []uint64{1}})
	if err != nil {
		t.Fatalf("encodeFreeListNode() error = %v", err)
	}
	page[freeListHeaderSize] ^= 0xff
	if _, err := decodeFreeListNode(page); !errors.Is(err, errInvalidPageData) {
		t.Fatalf("decodeFreeListNode(corrupt) error = %v, want %v", err, errInvalidPageData)
	}
}
