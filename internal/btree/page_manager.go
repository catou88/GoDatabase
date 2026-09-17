package btree

import (
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"io"
	"math"
	"os"
)

const (
	metadataSlotCount = 2
	metadataSize      = metadataSlotCount * pageSize
	metadataMagic     = "GDBM"
	metadataVersion   = 3

	metadataMagicOffset        = 0
	metadataVersionOffset      = 4
	metadataGenerationOffset   = 8
	metadataRootPageOffset     = 16
	metadataPageCountOffset    = 24
	metadataFreeListHeadOffset = 32
	metadataFreeListTailOffset = 40
	metadataHeadSequenceOffset = 48
	metadataTailSequenceOffset = 56
	metadataMaxSequenceOffset  = 64
	metadataChecksumOffset     = 72

	legacyFreeCountOffset    = 32
	legacyRetiredCountOffset = 34
	legacyChecksumOffset     = 36
	legacyPageIDsOffset      = 40
	legacyMaxPageIDs         = (pageSize - legacyPageIDsOffset) / 8
)

var (
	errInvalidPageID   = errors.New("invalid page id")
	errInvalidPageData = errors.New("invalid page data")
)

type pageManager struct {
	file               *os.File
	nextPageID         uint64
	rootPageID         uint64
	generation         uint64
	freePageIDs        []uint64
	retiredPageIDs     []uint64
	pendingFreePageIDs []uint64
	freePageSet        map[uint64]struct{}
	freeListPageIDs    []uint64
}

func openPageManager(path string) (*pageManager, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	if info.Size() == 0 {
		if err := file.Truncate(metadataSize); err != nil {
			_ = file.Close()
			return nil, err
		}
		info, err = file.Stat()
		if err != nil {
			_ = file.Close()
			return nil, err
		}
	}
	if info.Size() < metadataSize || (info.Size()-metadataSize)%pageSize != 0 {
		_ = file.Close()
		return nil, fmt.Errorf("%w: file size %d is not page aligned", errInvalidPageData, info.Size())
	}

	metadata, err := readRootMetadata(file)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	nextPageID := uint64((info.Size()-metadataSize)/pageSize) + 1
	if metadata.rootPageID != 0 && metadata.rootPageID >= nextPageID {
		_ = file.Close()
		return nil, fmt.Errorf("%w: root page %d is not allocated", errInvalidPageID, metadata.rootPageID)
	}
	freePageSet := make(map[uint64]struct{}, len(metadata.freePageIDs))
	for _, pageID := range metadata.freePageIDs {
		if pageID == metadata.rootPageID || pageID >= nextPageID {
			_ = file.Close()
			return nil, fmt.Errorf("%w: invalid free page %d", errInvalidPageData, pageID)
		}
		freePageSet[pageID] = struct{}{}
	}

	return &pageManager{
		file:            file,
		nextPageID:      nextPageID,
		rootPageID:      metadata.rootPageID,
		generation:      metadata.generation,
		freePageIDs:     metadata.freePageIDs,
		retiredPageIDs:  metadata.retiredPageIDs,
		freePageSet:     freePageSet,
		freeListPageIDs: metadata.freeListPageIDs,
	}, nil
}

func (pm *pageManager) close() error {
	return pm.file.Close()
}

func (pm *pageManager) allocatePage() (uint64, error) {
	state := pm.snapshot()
	pageID, err := pm.reservePage()
	if err != nil {
		return 0, err
	}
	offset, err := dataPageOffset(pageID)
	if err != nil {
		pm.restore(state)
		return 0, err
	}

	blankPage := make([]byte, pageSize)
	n, err := pm.file.WriteAt(blankPage, offset)
	if err != nil {
		pm.restore(state)
		return 0, err
	}
	if n != pageSize {
		pm.restore(state)
		return 0, io.ErrShortWrite
	}
	return pageID, nil
}

func (pm *pageManager) reservePage() (uint64, error) {
	if len(pm.freePageIDs) > 0 {
		pageID := pm.freePageIDs[len(pm.freePageIDs)-1]
		pm.freePageIDs = pm.freePageIDs[:len(pm.freePageIDs)-1]
		delete(pm.freePageSet, pageID)
		return pageID, nil
	}
	pageID := pm.nextPageID
	if _, err := dataPageOffset(pageID); err != nil {
		return 0, err
	}
	pm.nextPageID++
	return pageID, nil
}

func (pm *pageManager) freePage(pageID uint64) error {
	if err := pm.validateExistingPageID(pageID); err != nil {
		return err
	}
	if containsPageID(pm.freeListPageIDs, pageID) {
		return fmt.Errorf("%w: page %d stores the active free list", errInvalidPageID, pageID)
	}
	if containsPageID(pm.retiredPageIDs, pageID) || containsPageID(pm.pendingFreePageIDs, pageID) {
		return fmt.Errorf("%w: page %d is already freed", errInvalidPageID, pageID)
	}
	pm.pendingFreePageIDs = append(pm.pendingFreePageIDs, pageID)
	return nil
}

func (pm *pageManager) readPage(pageID uint64) ([]byte, error) {
	if err := pm.validateExistingPageID(pageID); err != nil {
		return nil, err
	}
	offset, err := dataPageOffset(pageID)
	if err != nil {
		return nil, err
	}

	page := make([]byte, pageSize)
	n, err := pm.file.ReadAt(page, offset)
	if err != nil {
		return nil, err
	}
	if n != pageSize {
		return nil, io.ErrUnexpectedEOF
	}

	return page, nil
}

func (pm *pageManager) writePage(pageID uint64, page []byte) error {
	if err := pm.validateExistingPageID(pageID); err != nil {
		return err
	}
	if len(page) != pageSize {
		return fmt.Errorf("%w: page size %d, want %d", errInvalidPageData, len(page), pageSize)
	}
	offset, err := dataPageOffset(pageID)
	if err != nil {
		return err
	}

	n, err := pm.file.WriteAt(page, offset)
	if err != nil {
		return err
	}
	if n != pageSize {
		return io.ErrShortWrite
	}

	return nil
}

func (pm *pageManager) rootPage() uint64 {
	return pm.rootPageID
}

func (pm *pageManager) commitRoot(rootPageID uint64) error {
	if err := pm.file.Sync(); err != nil {
		return err
	}
	return pm.publishRoot(rootPageID, nil, pm.file.Sync)
}

func (pm *pageManager) publishRoot(
	rootPageID uint64,
	beforeWrite func() error,
	syncMetadata func() error,
) error {
	if rootPageID != 0 {
		if err := pm.validateExistingPageID(rootPageID); err != nil {
			return err
		}
	}
	if rootPageID != 0 && (containsPageID(pm.retiredPageIDs, rootPageID) || containsPageID(pm.pendingFreePageIDs, rootPageID)) {
		return fmt.Errorf("%w: root page %d is marked obsolete", errInvalidPageID, rootPageID)
	}
	if containsPageID(pm.freeListPageIDs, rootPageID) {
		return fmt.Errorf("%w: root page %d stores the active free list", errInvalidPageID, rootPageID)
	}

	nextGeneration := pm.generation + 1
	slot := int(nextGeneration % metadataSlotCount)
	metadata, err := pm.writeFreeListSnapshot(nextGeneration, rootPageID)
	if err != nil {
		return err
	}
	if beforeWrite != nil {
		if err := beforeWrite(); err != nil {
			return err
		}
	}
	if err := pm.file.Sync(); err != nil {
		return err
	}
	if err := writeRootMetadataSlot(pm.file, slot, metadata); err != nil {
		return err
	}
	if err := syncMetadata(); err != nil {
		return err
	}

	pm.rootPageID = rootPageID
	pm.generation = nextGeneration
	pm.pendingFreePageIDs = nil
	pm.freePageSet = make(map[uint64]struct{}, len(pm.freePageIDs))
	for _, pageID := range pm.freePageIDs {
		pm.freePageSet[pageID] = struct{}{}
	}
	return nil
}

func (pm *pageManager) writeFreeListSnapshot(generation, rootPageID uint64) (rootMetadata, error) {
	eligible := append([]uint64(nil), pm.freePageIDs...)
	eligible = append(eligible, pm.retiredPageIDs...)
	protected := append([]uint64(nil), pm.pendingFreePageIDs...)
	protected = append(protected, pm.freeListPageIDs...)

	totalEntries := len(eligible) + len(protected)
	recyclable := make([]uint64, 0)
	for _, pageID := range eligible {
		page, err := readRawDataPage(pm.file, pageID)
		if err == nil {
			if _, err := decodeFreeListNode(page); err == nil {
				recyclable = append(recyclable, pageID)
			}
		}
	}
	nodeCount := 0
	reusedNodeCount := 0
	if totalEntries > 0 {
		maxNodes := (totalEntries + freeListPageCapacity - 1) / freeListPageCapacity
		for candidate := 1; candidate <= maxNodes; candidate++ {
			reused := candidate
			if reused > len(recyclable) {
				reused = len(recyclable)
			}
			needed := (totalEntries - reused + freeListPageCapacity - 1) / freeListPageCapacity
			if needed == candidate {
				nodeCount = candidate
				reusedNodeCount = reused
				break
			}
		}
		if nodeCount == 0 {
			nodeCount = maxNodes
		}
	}

	nodePageIDs := make([]uint64, nodeCount)
	for i := range nodePageIDs {
		if i < reusedNodeCount {
			last := len(recyclable) - 1
			nodePageIDs[i] = recyclable[last]
			recyclable = recyclable[:last]
			eligible = removePageID(eligible, nodePageIDs[i])
			delete(pm.freePageSet, nodePageIDs[i])
			continue
		}
		pageID := pm.nextPageID
		if _, err := dataPageOffset(pageID); err != nil {
			return rootMetadata{}, err
		}
		pm.nextPageID++
		nodePageIDs[i] = pageID
	}

	entries := append(append([]uint64(nil), eligible...), protected...)
	for i, pageID := range nodePageIDs {
		start := i * freeListPageCapacity
		end := start + freeListPageCapacity
		if end > len(entries) {
			end = len(entries)
		}
		nextPageID := uint64(0)
		if i+1 < len(nodePageIDs) {
			nextPageID = nodePageIDs[i+1]
		}
		page, err := encodeFreeListNode(freeListNode{
			nextPageID: nextPageID,
			sequence:   uint64(start),
			pageIDs:    entries[start:end],
		})
		if err != nil {
			return rootMetadata{}, err
		}
		if err := pm.writePage(pageID, page); err != nil {
			return rootMetadata{}, err
		}
	}

	pm.freePageIDs = eligible
	pm.retiredPageIDs = protected
	pm.freeListPageIDs = nodePageIDs
	metadata := rootMetadata{
		generation:      generation,
		rootPageID:      rootPageID,
		pageCount:       pm.nextPageID - 1,
		headSequence:    0,
		tailSequence:    uint64(len(entries)),
		maxSequence:     uint64(len(eligible)),
		freePageIDs:     append([]uint64(nil), eligible...),
		retiredPageIDs:  append([]uint64(nil), protected...),
		freeListPageIDs: append([]uint64(nil), nodePageIDs...),
	}
	if len(nodePageIDs) > 0 {
		metadata.freeListHeadPageID = nodePageIDs[0]
		metadata.freeListTailPageID = nodePageIDs[len(nodePageIDs)-1]
	}
	return metadata, nil
}

func removePageID(pageIDs []uint64, target uint64) []uint64 {
	for i, pageID := range pageIDs {
		if pageID == target {
			return append(pageIDs[:i], pageIDs[i+1:]...)
		}
	}
	return pageIDs
}

type pageManagerState struct {
	nextPageID         uint64
	rootPageID         uint64
	generation         uint64
	freePageIDs        []uint64
	retiredPageIDs     []uint64
	pendingFreePageIDs []uint64
	freeListPageIDs    []uint64
}

func (pm *pageManager) snapshot() pageManagerState {
	return pageManagerState{
		nextPageID:         pm.nextPageID,
		rootPageID:         pm.rootPageID,
		generation:         pm.generation,
		freePageIDs:        append([]uint64(nil), pm.freePageIDs...),
		retiredPageIDs:     append([]uint64(nil), pm.retiredPageIDs...),
		pendingFreePageIDs: append([]uint64(nil), pm.pendingFreePageIDs...),
		freeListPageIDs:    append([]uint64(nil), pm.freeListPageIDs...),
	}
}

func (pm *pageManager) restore(state pageManagerState) {
	pm.nextPageID = state.nextPageID
	pm.rootPageID = state.rootPageID
	pm.generation = state.generation
	pm.freePageIDs = append([]uint64(nil), state.freePageIDs...)
	pm.retiredPageIDs = append([]uint64(nil), state.retiredPageIDs...)
	pm.pendingFreePageIDs = append([]uint64(nil), state.pendingFreePageIDs...)
	pm.freeListPageIDs = append([]uint64(nil), state.freeListPageIDs...)
	pm.freePageSet = make(map[uint64]struct{}, len(pm.freePageIDs))
	for _, pageID := range pm.freePageIDs {
		pm.freePageSet[pageID] = struct{}{}
	}
}

func (pm *pageManager) validateExistingPageID(pageID uint64) error {
	if pageID == 0 || pageID >= pm.nextPageID {
		return fmt.Errorf("%w: %d", errInvalidPageID, pageID)
	}
	if _, freed := pm.freePageSet[pageID]; freed {
		return fmt.Errorf("%w: page %d is free", errInvalidPageID, pageID)
	}
	return nil
}

func containsPageID(pageIDs []uint64, target uint64) bool {
	for _, pageID := range pageIDs {
		if pageID == target {
			return true
		}
	}
	return false
}

func dataPageOffset(pageID uint64) (int64, error) {
	if pageID == 0 {
		return 0, fmt.Errorf("%w: %d", errInvalidPageID, pageID)
	}
	if pageID > uint64((math.MaxInt64-metadataSize)/pageSize)+1 {
		return 0, fmt.Errorf("%w: %d overflows file offset", errInvalidPageID, pageID)
	}
	return int64(metadataSize + (pageID-1)*pageSize), nil
}

type rootMetadata struct {
	generation         uint64
	rootPageID         uint64
	pageCount          uint64
	freeListHeadPageID uint64
	freeListTailPageID uint64
	headSequence       uint64
	tailSequence       uint64
	maxSequence        uint64
	freePageIDs        []uint64
	retiredPageIDs     []uint64
	freeListPageIDs    []uint64
}

func readRootMetadata(file *os.File) (rootMetadata, error) {
	var best rootMetadata
	found := false
	for slot := 0; slot < metadataSlotCount; slot++ {
		metadata, err := readRootMetadataSlot(file, slot)
		if err != nil {
			if errors.Is(err, errInvalidPageData) {
				continue
			}
			return rootMetadata{}, err
		}
		if !found || metadata.generation > best.generation {
			best = metadata
			found = true
		}
	}
	if !found {
		return rootMetadata{}, nil
	}
	return best, nil
}

func readRootMetadataSlot(file *os.File, slot int) (rootMetadata, error) {
	page := make([]byte, pageSize)
	n, err := file.ReadAt(page, int64(slot*pageSize))
	if err != nil {
		return rootMetadata{}, err
	}
	if n != pageSize {
		return rootMetadata{}, io.ErrUnexpectedEOF
	}
	if string(page[metadataMagicOffset:metadataVersionOffset]) != metadataMagic {
		return rootMetadata{}, errInvalidPageData
	}
	version := binary.LittleEndian.Uint16(page[metadataVersionOffset:])
	if version != 1 && version != 2 && version != metadataVersion {
		return rootMetadata{}, errInvalidPageData
	}

	if version == 1 {
		wantChecksum := binary.LittleEndian.Uint32(page[32:])
		binary.LittleEndian.PutUint32(page[32:], 0)
		if gotChecksum := crc32.ChecksumIEEE(page[:36]); gotChecksum != wantChecksum {
			return rootMetadata{}, errInvalidPageData
		}
	} else {
		checksumOffset := metadataChecksumOffset
		if version == 2 {
			checksumOffset = legacyChecksumOffset
		}
		wantChecksum := binary.LittleEndian.Uint32(page[checksumOffset:])
		binary.LittleEndian.PutUint32(page[checksumOffset:], 0)
		if gotChecksum := crc32.ChecksumIEEE(page); gotChecksum != wantChecksum {
			return rootMetadata{}, errInvalidPageData
		}
	}

	metadata := rootMetadata{
		generation: binary.LittleEndian.Uint64(page[metadataGenerationOffset:]),
		rootPageID: binary.LittleEndian.Uint64(page[metadataRootPageOffset:]),
		pageCount:  binary.LittleEndian.Uint64(page[metadataPageCountOffset:]),
	}
	if metadata.rootPageID > metadata.pageCount {
		return rootMetadata{}, errInvalidPageData
	}
	switch version {
	case 2:
		freeCount := int(binary.LittleEndian.Uint16(page[legacyFreeCountOffset:]))
		retiredCount := int(binary.LittleEndian.Uint16(page[legacyRetiredCountOffset:]))
		if freeCount+retiredCount > legacyMaxPageIDs {
			return rootMetadata{}, errInvalidPageData
		}
		seen := make(map[uint64]struct{}, freeCount+retiredCount)
		for i := 0; i < freeCount+retiredCount; i++ {
			pos := legacyPageIDsOffset + i*8
			pageID := binary.LittleEndian.Uint64(page[pos:])
			if pageID == 0 || pageID > metadata.pageCount || pageID == metadata.rootPageID {
				return rootMetadata{}, errInvalidPageData
			}
			if _, duplicate := seen[pageID]; duplicate {
				return rootMetadata{}, errInvalidPageData
			}
			seen[pageID] = struct{}{}
			if i < freeCount {
				metadata.freePageIDs = append(metadata.freePageIDs, pageID)
			} else {
				metadata.retiredPageIDs = append(metadata.retiredPageIDs, pageID)
			}
		}
	case metadataVersion:
		metadata.freeListHeadPageID = binary.LittleEndian.Uint64(page[metadataFreeListHeadOffset:])
		metadata.freeListTailPageID = binary.LittleEndian.Uint64(page[metadataFreeListTailOffset:])
		metadata.headSequence = binary.LittleEndian.Uint64(page[metadataHeadSequenceOffset:])
		metadata.tailSequence = binary.LittleEndian.Uint64(page[metadataTailSequenceOffset:])
		metadata.maxSequence = binary.LittleEndian.Uint64(page[metadataMaxSequenceOffset:])
		if err := loadFreeListMetadata(file, &metadata); err != nil {
			return rootMetadata{}, err
		}
	}
	return metadata, nil
}

func writeRootMetadataSlot(file *os.File, slot int, metadata rootMetadata) error {
	page := make([]byte, pageSize)
	copy(page[metadataMagicOffset:metadataVersionOffset], metadataMagic)
	binary.LittleEndian.PutUint16(page[metadataVersionOffset:], metadataVersion)
	binary.LittleEndian.PutUint64(page[metadataGenerationOffset:], metadata.generation)
	binary.LittleEndian.PutUint64(page[metadataRootPageOffset:], metadata.rootPageID)
	binary.LittleEndian.PutUint64(page[metadataPageCountOffset:], metadata.pageCount)
	binary.LittleEndian.PutUint64(page[metadataFreeListHeadOffset:], metadata.freeListHeadPageID)
	binary.LittleEndian.PutUint64(page[metadataFreeListTailOffset:], metadata.freeListTailPageID)
	binary.LittleEndian.PutUint64(page[metadataHeadSequenceOffset:], metadata.headSequence)
	binary.LittleEndian.PutUint64(page[metadataTailSequenceOffset:], metadata.tailSequence)
	binary.LittleEndian.PutUint64(page[metadataMaxSequenceOffset:], metadata.maxSequence)
	checksum := crc32.ChecksumIEEE(page)
	binary.LittleEndian.PutUint32(page[metadataChecksumOffset:], checksum)

	n, err := file.WriteAt(page, int64(slot*pageSize))
	if err != nil {
		return err
	}
	if n != pageSize {
		return io.ErrShortWrite
	}
	return nil
}

func loadFreeListMetadata(file *os.File, metadata *rootMetadata) error {
	if metadata.freeListHeadPageID == 0 {
		if metadata.freeListTailPageID != 0 || metadata.headSequence != 0 ||
			metadata.tailSequence != 0 || metadata.maxSequence != 0 {
			return errInvalidPageData
		}
		return nil
	}
	if metadata.freeListTailPageID == 0 || metadata.headSequence > metadata.maxSequence ||
		metadata.maxSequence > metadata.tailSequence {
		return errInvalidPageData
	}

	seenPages := make(map[uint64]struct{})
	seenEntries := make(map[uint64]struct{})
	pageID := metadata.freeListHeadPageID
	expectedSequence := metadata.headSequence
	for pageID != 0 {
		if pageID > metadata.pageCount || pageID == metadata.rootPageID {
			return errInvalidPageData
		}
		if _, duplicate := seenPages[pageID]; duplicate {
			return errInvalidPageData
		}
		seenPages[pageID] = struct{}{}
		page, err := readRawDataPage(file, pageID)
		if err != nil {
			return err
		}
		node, err := decodeFreeListNode(page)
		if err != nil || node.sequence != expectedSequence {
			return errInvalidPageData
		}
		metadata.freeListPageIDs = append(metadata.freeListPageIDs, pageID)
		for _, entryID := range node.pageIDs {
			if entryID > metadata.pageCount || entryID == metadata.rootPageID {
				return errInvalidPageData
			}
			if _, duplicate := seenPages[entryID]; duplicate {
				return errInvalidPageData
			}
			if _, duplicate := seenEntries[entryID]; duplicate {
				return errInvalidPageData
			}
			seenEntries[entryID] = struct{}{}
			if expectedSequence < metadata.maxSequence {
				metadata.freePageIDs = append(metadata.freePageIDs, entryID)
			} else {
				metadata.retiredPageIDs = append(metadata.retiredPageIDs, entryID)
			}
			expectedSequence++
		}
		if node.nextPageID == 0 && pageID != metadata.freeListTailPageID {
			return errInvalidPageData
		}
		pageID = node.nextPageID
	}
	if expectedSequence != metadata.tailSequence {
		return errInvalidPageData
	}
	for listPageID := range seenPages {
		if _, alsoFree := seenEntries[listPageID]; alsoFree {
			return errInvalidPageData
		}
	}
	return nil
}
