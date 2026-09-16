package btree

import (
	"encoding/binary"
	"errors"
	"strings"
	"testing"
)

func TestEncodeDecodeLeafPageNode(t *testing.T) {
	want := &pageNode{
		leaf:   true,
		keys:   []string{"a", "m", "z"},
		values: []string{"one", "middle", "last"},
	}

	page, err := encodePageNode(want)
	if err != nil {
		t.Fatalf("encodePageNode() error = %v", err)
	}
	if len(page) != pageSize {
		t.Fatalf("encoded page size = %d, want %d", len(page), pageSize)
	}

	got, err := decodePageNode(page)
	if err != nil {
		t.Fatalf("decodePageNode() error = %v", err)
	}
	assertPageNodeEqual(t, got, want)
}

func TestEncodeDecodeEmptyLeafPageNode(t *testing.T) {
	want := &pageNode{leaf: true}

	page, err := encodePageNode(want)
	if err != nil {
		t.Fatalf("encodePageNode() error = %v", err)
	}

	got, err := decodePageNode(page)
	if err != nil {
		t.Fatalf("decodePageNode() error = %v", err)
	}
	assertPageNodeEqual(t, got, want)
}

func TestEncodeDecodeInternalPageNode(t *testing.T) {
	want := &pageNode{
		keys:       []string{"g", "r"},
		childPages: []uint64{2, 5, 8},
	}

	page, err := encodePageNode(want)
	if err != nil {
		t.Fatalf("encodePageNode() error = %v", err)
	}

	got, err := decodePageNode(page)
	if err != nil {
		t.Fatalf("decodePageNode() error = %v", err)
	}
	assertPageNodeEqual(t, got, want)
}

func TestEncodePageNodeRejectsInvalidNodes(t *testing.T) {
	tests := []struct {
		name    string
		node    *pageNode
		wantErr string
	}{
		{
			name:    "nil node",
			node:    nil,
			wantErr: "node is nil",
		},
		{
			name: "empty key",
			node: &pageNode{
				leaf:   true,
				keys:   []string{""},
				values: []string{"value"},
			},
			wantErr: "empty",
		},
		{
			name: "unsorted keys",
			node: &pageNode{
				leaf:   true,
				keys:   []string{"b", "a"},
				values: []string{"two", "one"},
			},
			wantErr: "sorted",
		},
		{
			name: "duplicate keys",
			node: &pageNode{
				leaf:   true,
				keys:   []string{"a", "a"},
				values: []string{"one", "two"},
			},
			wantErr: "sorted",
		},
		{
			name: "leaf value count mismatch",
			node: &pageNode{
				leaf:   true,
				keys:   []string{"a"},
				values: nil,
			},
			wantErr: "values",
		},
		{
			name: "leaf with child pages",
			node: &pageNode{
				leaf:       true,
				keys:       []string{"a"},
				values:     []string{"one"},
				childPages: []uint64{2},
			},
			wantErr: "child pages",
		},
		{
			name: "internal with values",
			node: &pageNode{
				keys:       []string{"m"},
				values:     []string{"bad"},
				childPages: []uint64{2, 3},
			},
			wantErr: "values",
		},
		{
			name: "internal child count mismatch",
			node: &pageNode{
				keys:       []string{"m"},
				childPages: []uint64{2},
			},
			wantErr: "child pages",
		},
		{
			name: "internal zero child page",
			node: &pageNode{
				keys:       []string{"m"},
				childPages: []uint64{2, 0},
			},
			wantErr: "zero",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := encodePageNode(tt.node)
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("encodePageNode() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestDecodePageNodeRejectsMalformedPages(t *testing.T) {
	validPage, err := encodePageNode(&pageNode{
		leaf:   true,
		keys:   []string{"a", "b"},
		values: []string{"one", "two"},
	})
	if err != nil {
		t.Fatalf("encodePageNode() error = %v", err)
	}

	tests := []struct {
		name    string
		page    func() []byte
		wantErr string
	}{
		{
			name: "invalid size",
			page: func() []byte {
				return validPage[:pageSize-1]
			},
			wantErr: errInvalidPageSize.Error(),
		},
		{
			name: "invalid magic",
			page: func() []byte {
				page := cloneBytes(validPage)
				copy(page[0:4], "BAD!")
				return page
			},
			wantErr: errInvalidMagic.Error(),
		},
		{
			name: "invalid version",
			page: func() []byte {
				page := cloneBytes(validPage)
				binary.LittleEndian.PutUint16(page[4:6], pageFormat+1)
				return page
			},
			wantErr: errInvalidVersion.Error(),
		},
		{
			name: "invalid node type",
			page: func() []byte {
				page := cloneBytes(validPage)
				binary.LittleEndian.PutUint16(page[6:8], 99)
				return page
			},
			wantErr: errInvalidNodeType.Error(),
		},
		{
			name: "first offset is not zero",
			page: func() []byte {
				page := cloneBytes(validPage)
				offsetStart := pageHeaderSize
				binary.LittleEndian.PutUint16(page[offsetStart:], 1)
				return page
			},
			wantErr: "first offset",
		},
		{
			name: "offset moves backward",
			page: func() []byte {
				page := cloneBytes(validPage)
				offsetStart := pageHeaderSize
				binary.LittleEndian.PutUint16(page[offsetStart+pageOffsetSize:], 0)
				return page
			},
			wantErr: "too small",
		},
		{
			name: "record length mismatch",
			page: func() []byte {
				page := cloneBytes(validPage)
				recordStart := pageHeaderSize + 3*pageOffsetSize
				binary.LittleEndian.PutUint16(page[recordStart:], 10)
				return page
			},
			wantErr: "length mismatch",
		},
		{
			name: "internal value is not empty",
			page: func() []byte {
				page, err := encodePageNode(&pageNode{
					keys:       []string{"m"},
					childPages: []uint64{2, 3},
				})
				if err != nil {
					t.Fatalf("encodePageNode() error = %v", err)
				}
				recordStart := pageHeaderSize + 2*pagePtrSize + 2*pageOffsetSize
				binary.LittleEndian.PutUint16(page[recordStart+2:], 1)
				page[recordStart+5] = 'x'
				binary.LittleEndian.PutUint16(page[pageHeaderSize+2*pagePtrSize+pageOffsetSize:], 6)
				return page
			},
			wantErr: "internal",
		},
		{
			name: "internal zero child page",
			page: func() []byte {
				page, err := encodePageNode(&pageNode{
					keys:       []string{"m"},
					childPages: []uint64{2, 3},
				})
				if err != nil {
					t.Fatalf("encodePageNode() error = %v", err)
				}
				binary.LittleEndian.PutUint64(page[pageHeaderSize:], 0)
				return page
			},
			wantErr: "zero",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodePageNode(tt.page())
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) && !errors.Is(err, errorByMessage(tt.wantErr)) {
				t.Fatalf("decodePageNode() error = %q, want substring %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func cloneBytes(values []byte) []byte {
	clone := make([]byte, len(values))
	copy(clone, values)
	return clone
}

func assertPageNodeEqual(t *testing.T, got, want *pageNode) {
	t.Helper()

	if got.leaf != want.leaf {
		t.Fatalf("leaf = %v, want %v", got.leaf, want.leaf)
	}
	assertStringSlicesEqual(t, "keys", got.keys, want.keys)
	assertStringSlicesEqual(t, "values", got.values, want.values)
	if len(got.childPages) != len(want.childPages) {
		t.Fatalf("childPages length = %d, want %d", len(got.childPages), len(want.childPages))
	}
	for i, gotChild := range got.childPages {
		if gotChild != want.childPages[i] {
			t.Fatalf("childPages[%d] = %d, want %d", i, gotChild, want.childPages[i])
		}
	}
}

func assertStringSlicesEqual(t *testing.T, name string, got, want []string) {
	t.Helper()

	if len(got) != len(want) {
		t.Fatalf("%s length = %d, want %d", name, len(got), len(want))
	}
	for i, gotValue := range got {
		if gotValue != want[i] {
			t.Fatalf("%s[%d] = %q, want %q", name, i, gotValue, want[i])
		}
	}
}

func errorByMessage(message string) error {
	switch message {
	case errInvalidPageSize.Error():
		return errInvalidPageSize
	case errInvalidMagic.Error():
		return errInvalidMagic
	case errInvalidVersion.Error():
		return errInvalidVersion
	case errInvalidNodeType.Error():
		return errInvalidNodeType
	default:
		return nil
	}
}
