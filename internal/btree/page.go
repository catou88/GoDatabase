package btree

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	pageSize   = 4096
	pageMagic  = "GDBT"
	pageFormat = 1

	pageHeaderSize = 16
	pageOffsetSize = 2
	pagePtrSize    = 8

	nodeTypeInternal = 1
	nodeTypeLeaf     = 2

	maxKeySize   = 1000
	maxValueSize = 3000
)

var (
	errInvalidPageSize = errors.New("invalid page size")
	errInvalidMagic    = errors.New("invalid page magic")
	errInvalidVersion  = errors.New("invalid page version")
	errInvalidNodeType = errors.New("invalid node type")
)

type pageNode struct {
	leaf       bool
	keys       []string
	values     []string
	childPages []uint64
}

func encodePageNode(n *pageNode) ([]byte, error) {
	if err := validatePageNode(n); err != nil {
		return nil, err
	}

	page := make([]byte, pageSize)
	copy(page[0:4], pageMagic)
	binary.LittleEndian.PutUint16(page[4:6], pageFormat)
	binary.LittleEndian.PutUint16(page[6:8], pageNodeType(n))
	binary.LittleEndian.PutUint16(page[8:10], uint16(len(n.keys)))

	ptrStart := pageHeaderSize
	ptrEnd := ptrStart + len(n.childPages)*pagePtrSize
	for i, childPage := range n.childPages {
		binary.LittleEndian.PutUint64(page[ptrStart+i*pagePtrSize:], childPage)
	}

	offsetStart := ptrEnd
	recordStart := offsetStart + (len(n.keys)+1)*pageOffsetSize
	if recordStart > pageSize {
		return nil, fmt.Errorf("page metadata exceeds page size")
	}

	recordPos := recordStart
	for i, key := range n.keys {
		offset := recordPos - recordStart
		if offset > pageSize {
			return nil, fmt.Errorf("record offset exceeds page size")
		}
		binary.LittleEndian.PutUint16(page[offsetStart+i*pageOffsetSize:], uint16(offset))

		value := ""
		if n.leaf {
			value = n.values[i]
		}

		recordSize := 4 + len(key) + len(value)
		if recordPos+recordSize > pageSize {
			return nil, fmt.Errorf("encoded node exceeds page size")
		}

		binary.LittleEndian.PutUint16(page[recordPos:], uint16(len(key)))
		binary.LittleEndian.PutUint16(page[recordPos+2:], uint16(len(value)))
		copy(page[recordPos+4:], key)
		copy(page[recordPos+4+len(key):], value)
		recordPos += recordSize
	}

	endOffset := recordPos - recordStart
	binary.LittleEndian.PutUint16(page[offsetStart+len(n.keys)*pageOffsetSize:], uint16(endOffset))

	return page, nil
}

func decodePageNode(page []byte) (*pageNode, error) {
	if len(page) != pageSize {
		return nil, errInvalidPageSize
	}
	if string(page[0:4]) != pageMagic {
		return nil, errInvalidMagic
	}
	if version := binary.LittleEndian.Uint16(page[4:6]); version != pageFormat {
		return nil, fmt.Errorf("%w: %d", errInvalidVersion, version)
	}

	nodeType := binary.LittleEndian.Uint16(page[6:8])
	keyCount := int(binary.LittleEndian.Uint16(page[8:10]))

	leaf := false
	childCount := keyCount + 1
	switch nodeType {
	case nodeTypeLeaf:
		leaf = true
		childCount = 0
	case nodeTypeInternal:
	default:
		return nil, fmt.Errorf("%w: %d", errInvalidNodeType, nodeType)
	}

	ptrStart := pageHeaderSize
	ptrEnd := ptrStart + childCount*pagePtrSize
	offsetStart := ptrEnd
	recordStart := offsetStart + (keyCount+1)*pageOffsetSize
	if recordStart > pageSize {
		return nil, fmt.Errorf("page metadata exceeds page size")
	}

	childPages := make([]uint64, childCount)
	for i := range childPages {
		childPage := binary.LittleEndian.Uint64(page[ptrStart+i*pagePtrSize:])
		if childPage == 0 {
			return nil, fmt.Errorf("child page %d is zero", i)
		}
		childPages[i] = childPage
	}

	offsets := make([]int, keyCount+1)
	for i := range offsets {
		offset := int(binary.LittleEndian.Uint16(page[offsetStart+i*pageOffsetSize:]))
		if i > 0 && offset < offsets[i-1] {
			return nil, fmt.Errorf("offset %d moves backward", i)
		}
		if recordStart+offset > pageSize {
			return nil, fmt.Errorf("offset %d outside page", i)
		}
		offsets[i] = offset
	}
	if offsets[0] != 0 {
		return nil, fmt.Errorf("first offset is %d, want 0", offsets[0])
	}

	keys := make([]string, keyCount)
	values := make([]string, 0, keyCount)
	for i := 0; i < keyCount; i++ {
		recordPos := recordStart + offsets[i]
		recordEnd := recordStart + offsets[i+1]
		if recordEnd-recordPos < 4 {
			return nil, fmt.Errorf("record %d is too small", i)
		}

		keySize := int(binary.LittleEndian.Uint16(page[recordPos:]))
		valueSize := int(binary.LittleEndian.Uint16(page[recordPos+2:]))
		if keySize == 0 {
			return nil, fmt.Errorf("record %d has empty key", i)
		}
		if keySize > maxKeySize {
			return nil, fmt.Errorf("record %d key exceeds max size", i)
		}
		if valueSize > maxValueSize {
			return nil, fmt.Errorf("record %d value exceeds max size", i)
		}
		if recordPos+4+keySize+valueSize != recordEnd {
			return nil, fmt.Errorf("record %d length mismatch", i)
		}

		key := string(page[recordPos+4 : recordPos+4+keySize])
		if i > 0 && keys[i-1] >= key {
			return nil, fmt.Errorf("keys are not strictly sorted")
		}
		keys[i] = key

		value := string(page[recordPos+4+keySize : recordEnd])
		if leaf {
			values = append(values, value)
		} else if valueSize != 0 {
			return nil, fmt.Errorf("internal record %d has value", i)
		}
	}

	return &pageNode{
		leaf:       leaf,
		keys:       keys,
		values:     values,
		childPages: childPages,
	}, nil
}

func validatePageNode(n *pageNode) error {
	if n == nil {
		return fmt.Errorf("node is nil")
	}
	if len(n.keys) > 65535 {
		return fmt.Errorf("too many keys")
	}
	for i, key := range n.keys {
		if key == "" {
			return fmt.Errorf("key %d is empty", i)
		}
		if len(key) > maxKeySize {
			return fmt.Errorf("key %d exceeds max size", i)
		}
		if i > 0 && n.keys[i-1] >= key {
			return fmt.Errorf("keys are not strictly sorted")
		}
	}

	if n.leaf {
		if len(n.values) != len(n.keys) {
			return fmt.Errorf("leaf has %d values for %d keys", len(n.values), len(n.keys))
		}
		if len(n.childPages) != 0 {
			return fmt.Errorf("leaf has child pages")
		}
		for i, value := range n.values {
			if len(value) > maxValueSize {
				return fmt.Errorf("value %d exceeds max size", i)
			}
		}
		return nil
	}

	if len(n.values) != 0 {
		return fmt.Errorf("internal node has values")
	}
	if len(n.childPages) != len(n.keys)+1 {
		return fmt.Errorf("internal node has %d child pages for %d keys", len(n.childPages), len(n.keys))
	}
	for i, childPage := range n.childPages {
		if childPage == 0 {
			return fmt.Errorf("child page %d is zero", i)
		}
	}
	return nil
}

func pageNodeType(n *pageNode) uint16 {
	if n.leaf {
		return nodeTypeLeaf
	}
	return nodeTypeInternal
}
