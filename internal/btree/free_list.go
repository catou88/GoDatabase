package btree

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"os"
)

const (
	freeListMagic        = "GDFL"
	freeListVersion      = 1
	freeListHeaderSize   = 32
	freeListPageCapacity = (pageSize - freeListHeaderSize) / 8

	freeListVersionOffset  = 4
	freeListCountOffset    = 6
	freeListNextOffset     = 8
	freeListSequenceOffset = 16
	freeListChecksumOffset = 24
)

type freeListNode struct {
	nextPageID uint64
	sequence   uint64
	pageIDs    []uint64
}

func encodeFreeListNode(node freeListNode) ([]byte, error) {
	if len(node.pageIDs) == 0 || len(node.pageIDs) > freeListPageCapacity {
		return nil, fmt.Errorf("%w: free-list entry count %d", errInvalidPageData, len(node.pageIDs))
	}
	page := make([]byte, pageSize)
	copy(page[:4], freeListMagic)
	binary.LittleEndian.PutUint16(page[freeListVersionOffset:], freeListVersion)
	binary.LittleEndian.PutUint16(page[freeListCountOffset:], uint16(len(node.pageIDs)))
	binary.LittleEndian.PutUint64(page[freeListNextOffset:], node.nextPageID)
	binary.LittleEndian.PutUint64(page[freeListSequenceOffset:], node.sequence)
	for i, pageID := range node.pageIDs {
		if pageID == 0 {
			return nil, fmt.Errorf("%w: free-list page id is zero", errInvalidPageData)
		}
		binary.LittleEndian.PutUint64(page[freeListHeaderSize+i*8:], pageID)
	}
	binary.LittleEndian.PutUint32(page[freeListChecksumOffset:], crc32.ChecksumIEEE(page))
	return page, nil
}

func decodeFreeListNode(page []byte) (freeListNode, error) {
	if len(page) != pageSize {
		return freeListNode{}, errInvalidPageSize
	}
	if string(page[:4]) != freeListMagic || binary.LittleEndian.Uint16(page[freeListVersionOffset:]) != freeListVersion {
		return freeListNode{}, errInvalidPageData
	}
	wantChecksum := binary.LittleEndian.Uint32(page[freeListChecksumOffset:])
	copyPage := append([]byte(nil), page...)
	binary.LittleEndian.PutUint32(copyPage[freeListChecksumOffset:], 0)
	if crc32.ChecksumIEEE(copyPage) != wantChecksum {
		return freeListNode{}, errInvalidPageData
	}
	count := int(binary.LittleEndian.Uint16(page[freeListCountOffset:]))
	if count == 0 || count > freeListPageCapacity {
		return freeListNode{}, errInvalidPageData
	}
	node := freeListNode{
		nextPageID: binary.LittleEndian.Uint64(page[freeListNextOffset:]),
		sequence:   binary.LittleEndian.Uint64(page[freeListSequenceOffset:]),
		pageIDs:    make([]uint64, count),
	}
	for i := range node.pageIDs {
		node.pageIDs[i] = binary.LittleEndian.Uint64(page[freeListHeaderSize+i*8:])
		if node.pageIDs[i] == 0 {
			return freeListNode{}, errInvalidPageData
		}
	}
	return node, nil
}

func readRawDataPage(file *os.File, pageID uint64) ([]byte, error) {
	offset, err := dataPageOffset(pageID)
	if err != nil {
		return nil, err
	}
	page := make([]byte, pageSize)
	n, err := file.ReadAt(page, offset)
	if err != nil {
		return nil, err
	}
	if n != pageSize {
		return nil, io.ErrUnexpectedEOF
	}
	return page, nil
}
