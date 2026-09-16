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
	metadataVersion   = 1

	metadataMagicOffset      = 0
	metadataVersionOffset    = 4
	metadataGenerationOffset = 8
	metadataRootPageOffset   = 16
	metadataPageCountOffset  = 24
	metadataChecksumOffset   = 32
	metadataChecksumEnd      = 36
)

var (
	errInvalidPageID   = errors.New("invalid page id")
	errInvalidPageData = errors.New("invalid page data")
)

type pageManager struct {
	file       *os.File
	nextPageID uint64
	rootPageID uint64
	generation uint64
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

	rootPageID, generation, err := readRootMetadata(file)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	nextPageID := uint64((info.Size()-metadataSize)/pageSize) + 1
	if rootPageID >= nextPageID {
		_ = file.Close()
		return nil, fmt.Errorf("%w: root page %d is not allocated", errInvalidPageID, rootPageID)
	}

	return &pageManager{
		file:       file,
		nextPageID: nextPageID,
		rootPageID: rootPageID,
		generation: generation,
	}, nil
}

func (pm *pageManager) close() error {
	return pm.file.Close()
}

func (pm *pageManager) allocatePage() (uint64, error) {
	pageID := pm.nextPageID
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

	pm.nextPageID++
	return pageID, nil
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
	if err := pm.file.Sync(); err != nil {
		return err
	}

	nextGeneration := pm.generation + 1
	slot := int(nextGeneration % metadataSlotCount)
	if err := writeRootMetadataSlot(pm.file, slot, rootMetadata{
		generation: nextGeneration,
		rootPageID: rootPageID,
		pageCount:  pm.nextPageID - 1,
	}); err != nil {
		return err
	}
	if err := pm.file.Sync(); err != nil {
		return err
	}

	pm.rootPageID = rootPageID
	pm.generation = nextGeneration
	return nil
}

func (pm *pageManager) validateExistingPageID(pageID uint64) error {
	if pageID == 0 || pageID >= pm.nextPageID {
		return fmt.Errorf("%w: %d", errInvalidPageID, pageID)
	}
	return nil
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
	generation uint64
	rootPageID uint64
	pageCount  uint64
}

func readRootMetadata(file *os.File) (uint64, uint64, error) {
	var best rootMetadata
	found := false
	for slot := 0; slot < metadataSlotCount; slot++ {
		metadata, err := readRootMetadataSlot(file, slot)
		if err != nil {
			if errors.Is(err, errInvalidPageData) {
				continue
			}
			return 0, 0, err
		}
		if !found || metadata.generation > best.generation {
			best = metadata
			found = true
		}
	}
	if !found {
		return 0, 0, nil
	}
	return best.rootPageID, best.generation, nil
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
	if version := binary.LittleEndian.Uint16(page[metadataVersionOffset:]); version != metadataVersion {
		return rootMetadata{}, errInvalidPageData
	}

	wantChecksum := binary.LittleEndian.Uint32(page[metadataChecksumOffset:])
	clearChecksum(page)
	if gotChecksum := crc32.ChecksumIEEE(page[:metadataChecksumEnd]); gotChecksum != wantChecksum {
		return rootMetadata{}, errInvalidPageData
	}

	metadata := rootMetadata{
		generation: binary.LittleEndian.Uint64(page[metadataGenerationOffset:]),
		rootPageID: binary.LittleEndian.Uint64(page[metadataRootPageOffset:]),
		pageCount:  binary.LittleEndian.Uint64(page[metadataPageCountOffset:]),
	}
	if metadata.rootPageID == 0 || metadata.rootPageID > metadata.pageCount {
		return rootMetadata{}, errInvalidPageData
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
	checksum := crc32.ChecksumIEEE(page[:metadataChecksumEnd])
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

func clearChecksum(page []byte) {
	for i := metadataChecksumOffset; i < metadataChecksumEnd; i++ {
		page[i] = 0
	}
}
