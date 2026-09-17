package btree

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
)

const (
	pageSize   = 4096
	pageMagic  = "GDBT"
	pageFormat = 2

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

// BNode is the encoded representation of one persistent B+Tree node.
type BNode []byte

func (node BNode) btype() uint16 {
	return binary.LittleEndian.Uint16(node[6:8])
}

func (node BNode) nkeys() uint16 {
	return binary.LittleEndian.Uint16(node[8:10])
}

func (node BNode) setHeader(nodeType, keyCount uint16) {
	copy(node[0:4], pageMagic)
	binary.LittleEndian.PutUint16(node[4:6], pageFormat)
	binary.LittleEndian.PutUint16(node[6:8], nodeType)
	binary.LittleEndian.PutUint16(node[8:10], keyCount)
}

func (node BNode) getPtr(idx uint16) uint64 {
	pos := pageHeaderSize + int(idx)*pagePtrSize
	return binary.LittleEndian.Uint64(node[pos : pos+pagePtrSize])
}

func (node BNode) setPtr(idx uint16, pageID uint64) {
	pos := pageHeaderSize + int(idx)*pagePtrSize
	binary.LittleEndian.PutUint64(node[pos:pos+pagePtrSize], pageID)
}

func (node BNode) getOffset(idx uint16) uint16 {
	if idx == 0 {
		return 0
	}
	pos := pageHeaderSize + int(node.nkeys())*pagePtrSize + int(idx-1)*pageOffsetSize
	return binary.LittleEndian.Uint16(node[pos : pos+pageOffsetSize])
}

func (node BNode) setOffset(idx, offset uint16) {
	pos := pageHeaderSize + int(node.nkeys())*pagePtrSize + int(idx-1)*pageOffsetSize
	binary.LittleEndian.PutUint16(node[pos:pos+pageOffsetSize], offset)
}

func (node BNode) kvPos(idx uint16) uint16 {
	metadataSize := pageHeaderSize + int(node.nkeys())*(pagePtrSize+pageOffsetSize)
	return uint16(metadataSize) + node.getOffset(idx)
}

func (node BNode) getKey(idx uint16) []byte {
	pos := int(node.kvPos(idx))
	keySize := int(binary.LittleEndian.Uint16(node[pos : pos+2]))
	return node[pos+4 : pos+4+keySize]
}

func (node BNode) getVal(idx uint16) []byte {
	pos := int(node.kvPos(idx))
	keySize := int(binary.LittleEndian.Uint16(node[pos : pos+2]))
	valueSize := int(binary.LittleEndian.Uint16(node[pos+2 : pos+4]))
	return node[pos+4+keySize : pos+4+keySize+valueSize]
}

func (node BNode) nbytes() uint16 {
	return node.kvPos(node.nkeys())
}

func nodeAppendKV(node BNode, idx uint16, pageID uint64, key, value []byte) error {
	if idx >= node.nkeys() {
		return fmt.Errorf("entry index %d outside key count %d", idx, node.nkeys())
	}
	if len(key) > maxKeySize {
		return fmt.Errorf("key %d exceeds max size", idx)
	}
	if len(key) == 0 && idx != 0 {
		return fmt.Errorf("key %d is empty", idx)
	}
	if len(key) == 0 && len(value) != 0 {
		return fmt.Errorf("sentinel entry has value")
	}
	if len(value) > maxValueSize {
		return fmt.Errorf("value %d exceeds max size", idx)
	}
	switch node.btype() {
	case nodeTypeLeaf:
		if pageID != 0 {
			return fmt.Errorf("leaf entry %d has child page", idx)
		}
	case nodeTypeInternal:
		if pageID == 0 {
			return fmt.Errorf("child page %d is zero", idx)
		}
		if len(value) != 0 {
			return fmt.Errorf("internal entry %d has value", idx)
		}
	default:
		return fmt.Errorf("%w: %d", errInvalidNodeType, node.btype())
	}

	pos := int(node.kvPos(idx))
	recordSize := 4 + len(key) + len(value)
	if pos+recordSize > len(node) {
		return fmt.Errorf("encoded node exceeds page size")
	}

	node.setPtr(idx, pageID)
	binary.LittleEndian.PutUint16(node[pos:pos+2], uint16(len(key)))
	binary.LittleEndian.PutUint16(node[pos+2:pos+4], uint16(len(value)))
	copy(node[pos+4:], key)
	copy(node[pos+4+len(key):], value)
	node.setOffset(idx+1, node.getOffset(idx)+uint16(recordSize))
	return nil
}

func nodeAppendRange(dst BNode, src BNode, dstStart, srcStart, count uint16) error {
	if int(dstStart)+int(count) > int(dst.nkeys()) || int(srcStart)+int(count) > int(src.nkeys()) {
		return fmt.Errorf("entry range outside node")
	}
	for i := uint16(0); i < count; i++ {
		if err := nodeAppendKV(
			dst,
			dstStart+i,
			src.getPtr(srcStart+i),
			src.getKey(srcStart+i),
			src.getVal(srcStart+i),
		); err != nil {
			return err
		}
	}
	return nil
}

// pageNode is a convenient decoded form used by tests and page-manager code.
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

	node := BNode(make([]byte, pageSize))
	node.setHeader(pageNodeType(n), uint16(len(n.keys)))
	for i, key := range n.keys {
		pageID := uint64(0)
		value := ""
		if n.leaf {
			value = n.values[i]
		} else {
			pageID = n.childPages[i]
		}
		if err := nodeAppendKV(node, uint16(i), pageID, []byte(key), []byte(value)); err != nil {
			return nil, err
		}
	}
	if err := validateBNode(node); err != nil {
		return nil, err
	}
	return node, nil
}

func decodePageNode(page []byte) (*pageNode, error) {
	node := BNode(page)
	if err := validateBNode(node); err != nil {
		return nil, err
	}

	keyCount := int(node.nkeys())
	decoded := &pageNode{
		leaf: node.btype() == nodeTypeLeaf,
		keys: make([]string, keyCount),
	}
	if decoded.leaf {
		decoded.values = make([]string, keyCount)
	} else {
		decoded.childPages = make([]uint64, keyCount)
	}
	for i := 0; i < keyCount; i++ {
		idx := uint16(i)
		decoded.keys[i] = string(node.getKey(idx))
		if decoded.leaf {
			decoded.values[i] = string(node.getVal(idx))
		} else {
			decoded.childPages[i] = node.getPtr(idx)
		}
	}
	return decoded, nil
}

func validateBNode(node BNode) error {
	if len(node) != pageSize {
		return errInvalidPageSize
	}
	if string(node[0:4]) != pageMagic {
		return errInvalidMagic
	}
	if version := binary.LittleEndian.Uint16(node[4:6]); version != pageFormat {
		return fmt.Errorf("%w: %d", errInvalidVersion, version)
	}
	nodeType := binary.LittleEndian.Uint16(node[6:8])
	if nodeType != nodeTypeLeaf && nodeType != nodeTypeInternal {
		return fmt.Errorf("%w: %d", errInvalidNodeType, nodeType)
	}

	keyCount := int(binary.LittleEndian.Uint16(node[8:10]))
	recordStart := pageHeaderSize + keyCount*(pagePtrSize+pageOffsetSize)
	if recordStart > pageSize {
		return fmt.Errorf("page metadata exceeds page size")
	}

	for i := 0; i < keyCount; i++ {
		pageID := binary.LittleEndian.Uint64(node[pageHeaderSize+i*pagePtrSize:])
		if nodeType == nodeTypeInternal && pageID == 0 {
			return fmt.Errorf("child page %d is zero", i)
		}
		if nodeType == nodeTypeLeaf && pageID != 0 {
			return fmt.Errorf("leaf entry %d has child page", i)
		}
	}

	previousOffset := 0
	var previousKey []byte
	for i := 0; i < keyCount; i++ {
		offsetPos := pageHeaderSize + keyCount*pagePtrSize + i*pageOffsetSize
		endOffset := int(binary.LittleEndian.Uint16(node[offsetPos : offsetPos+pageOffsetSize]))
		if endOffset < previousOffset {
			return fmt.Errorf("offset %d moves backward", i+1)
		}
		if recordStart+endOffset > pageSize {
			return fmt.Errorf("offset %d outside page", i+1)
		}

		recordPos := recordStart + previousOffset
		recordEnd := recordStart + endOffset
		if recordEnd-recordPos < 4 {
			return fmt.Errorf("record %d is too small", i)
		}
		keySize := int(binary.LittleEndian.Uint16(node[recordPos : recordPos+2]))
		valueSize := int(binary.LittleEndian.Uint16(node[recordPos+2 : recordPos+4]))
		if keySize == 0 && i != 0 {
			return fmt.Errorf("record %d has empty key", i)
		}
		if keySize == 0 && valueSize != 0 {
			return fmt.Errorf("sentinel record has value")
		}
		if keySize > maxKeySize {
			return fmt.Errorf("record %d key exceeds max size", i)
		}
		if valueSize > maxValueSize {
			return fmt.Errorf("record %d value exceeds max size", i)
		}
		if nodeType == nodeTypeInternal && valueSize != 0 {
			return fmt.Errorf("internal record %d has value", i)
		}
		if recordPos+4+keySize+valueSize != recordEnd {
			return fmt.Errorf("record %d length mismatch", i)
		}

		key := node[recordPos+4 : recordPos+4+keySize]
		if i > 0 && bytes.Compare(previousKey, key) >= 0 {
			return fmt.Errorf("keys are not strictly sorted")
		}
		previousKey = key
		previousOffset = endOffset
	}
	return nil
}

func validatePageNode(n *pageNode) error {
	if n == nil {
		return fmt.Errorf("node is nil")
	}
	if len(n.keys) > 65535 {
		return fmt.Errorf("too many keys")
	}
	if pageHeaderSize+len(n.keys)*(pagePtrSize+pageOffsetSize) > pageSize {
		return fmt.Errorf("page metadata exceeds page size")
	}
	for i, key := range n.keys {
		if key == "" && i != 0 {
			return fmt.Errorf("key %d is empty", i)
		}
		if key == "" && n.leaf && len(n.values) > i && n.values[i] != "" {
			return fmt.Errorf("sentinel entry has value")
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
	if len(n.childPages) != len(n.keys) {
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
