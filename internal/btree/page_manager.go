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
	metadataVersion   = 2

	metadataMagicOffset        = 0
	metadataVersionOffset      = 4
	metadataGenerationOffset   = 8
	metadataRootPageOffset     = 16
	metadataPageCountOffset    = 24
	metadataFreeCountOffset    = 32
	metadataRetiredCountOffset = 34
	metadataChecksumOffset     = 36
	metadataPageIDsOffset      = 40
	metadataMaxPageIDs         = (pageSize - metadataPageIDsOffset) / 8
)

var (
	errInvalidPageID   = errors.New("invalid page id")
	errInvalidPageData = errors.New("invalid page data")
	errFreeListFull    = errors.New("free list exceeds metadata capacity")
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
	if metadata.rootPageID >= nextPageID {
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
		file:           file,
		nextPageID:     nextPageID,
		rootPageID:     metadata.rootPageID,
		generation:     metadata.generation,
		freePageIDs:    metadata.freePageIDs,
		retiredPageIDs: metadata.retiredPageIDs,
		freePageSet:    freePageSet,
	}, nil
}

func (pm *pageManager) close() error {
	return pm.file.Close()
}

func (pm *pageManager) allocatePage() (uint64, error) {
	pageID := pm.nextPageID
	reusing := len(pm.freePageIDs) > 0
	if reusing {
		pageID = pm.freePageIDs[len(pm.freePageIDs)-1]
	}
	offset, err := dataPageOffset(pageID)
	if err != nil {
		return 0, err
	}

	blankPage := make([]byte, pageSize)
	n, err := pm.file.WriteAt(blankPage, offset)
	if err != nil {
		return 0, err
	}
	if n != pageSize {
		return 0, io.ErrShortWrite
	}

	if reusing {
		pm.freePageIDs = pm.freePageIDs[:len(pm.freePageIDs)-1]
		delete(pm.freePageSet, pageID)
	} else {
		pm.nextPageID++
	}
	return pageID, nil
}

func (pm *pageManager) freePage(pageID uint64) error {
	if err := pm.validateExistingPageID(pageID); err != nil {
		return err
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
	if err := pm.validateExistingPageID(rootPageID); err != nil {
		return err
	}
	if containsPageID(pm.retiredPageIDs, rootPageID) || containsPageID(pm.pendingFreePageIDs, rootPageID) {
		return fmt.Errorf("%w: root page %d is marked obsolete", errInvalidPageID, rootPageID)
	}
	if err := pm.file.Sync(); err != nil {
		return err
	}

	nextGeneration := pm.generation + 1
	slot := int(nextGeneration % metadataSlotCount)
	nextFreePageIDs := append([]uint64(nil), pm.freePageIDs...)
	nextFreePageIDs = append(nextFreePageIDs, pm.retiredPageIDs...)
	if len(nextFreePageIDs)+len(pm.pendingFreePageIDs) > metadataMaxPageIDs {
		return errFreeListFull
	}
	if err := writeRootMetadataSlot(pm.file, slot, rootMetadata{
		generation:     nextGeneration,
		rootPageID:     rootPageID,
		pageCount:      pm.nextPageID - 1,
		freePageIDs:    nextFreePageIDs,
		retiredPageIDs: pm.pendingFreePageIDs,
	}); err != nil {
		return err
	}
	if err := pm.file.Sync(); err != nil {
		return err
	}

	pm.rootPageID = rootPageID
	pm.generation = nextGeneration
	pm.freePageIDs = nextFreePageIDs
	pm.retiredPageIDs = append([]uint64(nil), pm.pendingFreePageIDs...)
	pm.pendingFreePageIDs = nil
	pm.freePageSet = make(map[uint64]struct{}, len(pm.freePageIDs))
	for _, pageID := range pm.freePageIDs {
		pm.freePageSet[pageID] = struct{}{}
	}
	return nil
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
	generation     uint64
	rootPageID     uint64
	pageCount      uint64
	freePageIDs    []uint64
	retiredPageIDs []uint64
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
	if version != 1 && version != metadataVersion {
		return rootMetadata{}, errInvalidPageData
	}

	if version == 1 {
		wantChecksum := binary.LittleEndian.Uint32(page[32:])
		binary.LittleEndian.PutUint32(page[32:], 0)
		if gotChecksum := crc32.ChecksumIEEE(page[:36]); gotChecksum != wantChecksum {
			return rootMetadata{}, errInvalidPageData
		}
	} else {
		wantChecksum := binary.LittleEndian.Uint32(page[metadataChecksumOffset:])
		binary.LittleEndian.PutUint32(page[metadataChecksumOffset:], 0)
		if gotChecksum := crc32.ChecksumIEEE(page); gotChecksum != wantChecksum {
			return rootMetadata{}, errInvalidPageData
		}
	}

	metadata := rootMetadata{
		generation: binary.LittleEndian.Uint64(page[metadataGenerationOffset:]),
		rootPageID: binary.LittleEndian.Uint64(page[metadataRootPageOffset:]),
		pageCount:  binary.LittleEndian.Uint64(page[metadataPageCountOffset:]),
	}
	if metadata.rootPageID == 0 || metadata.rootPageID > metadata.pageCount {
		return rootMetadata{}, errInvalidPageData
	}
	if version == metadataVersion {
		freeCount := int(binary.LittleEndian.Uint16(page[metadataFreeCountOffset:]))
		retiredCount := int(binary.LittleEndian.Uint16(page[metadataRetiredCountOffset:]))
		if freeCount+retiredCount > metadataMaxPageIDs {
			return rootMetadata{}, errInvalidPageData
		}
		seen := make(map[uint64]struct{}, freeCount+retiredCount)
		for i := 0; i < freeCount+retiredCount; i++ {
			pos := metadataPageIDsOffset + i*8
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
	}
	return metadata, nil
}

func writeRootMetadataSlot(file *os.File, slot int, metadata rootMetadata) error {
	if len(metadata.freePageIDs)+len(metadata.retiredPageIDs) > metadataMaxPageIDs {
		return errFreeListFull
	}
	page := make([]byte, pageSize)
	copy(page[metadataMagicOffset:metadataVersionOffset], metadataMagic)
	binary.LittleEndian.PutUint16(page[metadataVersionOffset:], metadataVersion)
	binary.LittleEndian.PutUint64(page[metadataGenerationOffset:], metadata.generation)
	binary.LittleEndian.PutUint64(page[metadataRootPageOffset:], metadata.rootPageID)
	binary.LittleEndian.PutUint64(page[metadataPageCountOffset:], metadata.pageCount)
	binary.LittleEndian.PutUint16(page[metadataFreeCountOffset:], uint16(len(metadata.freePageIDs)))
	binary.LittleEndian.PutUint16(page[metadataRetiredCountOffset:], uint16(len(metadata.retiredPageIDs)))
	pageIDs := append(append([]uint64(nil), metadata.freePageIDs...), metadata.retiredPageIDs...)
	for i, pageID := range pageIDs {
		binary.LittleEndian.PutUint64(page[metadataPageIDsOffset+i*8:], pageID)
	}
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
