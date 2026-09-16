package btree

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

var (
	errInvalidPageID   = errors.New("invalid page id")
	errInvalidPageData = errors.New("invalid page data")
)

type pageManager struct {
	file       *os.File
	nextPageID uint64
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
	if info.Size()%pageSize != 0 {
		_ = file.Close()
		return nil, fmt.Errorf("%w: file size %d is not page aligned", errInvalidPageData, info.Size())
	}

	return &pageManager{
		file:       file,
		nextPageID: uint64(info.Size()/pageSize) + 1,
	}, nil
}

func (pm *pageManager) close() error {
	return pm.file.Close()
}

func (pm *pageManager) allocatePage() (uint64, error) {
	pageID := pm.nextPageID
	offset, err := pageOffset(pageID)
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
	offset, err := pageOffset(pageID)
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
	offset, err := pageOffset(pageID)
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

func (pm *pageManager) validateExistingPageID(pageID uint64) error {
	if pageID == 0 || pageID >= pm.nextPageID {
		return fmt.Errorf("%w: %d", errInvalidPageID, pageID)
	}
	return nil
}

func pageOffset(pageID uint64) (int64, error) {
	if pageID == 0 {
		return 0, fmt.Errorf("%w: %d", errInvalidPageID, pageID)
	}
	if pageID > uint64(math.MaxInt64/pageSize)+1 {
		return 0, fmt.Errorf("%w: %d overflows file offset", errInvalidPageID, pageID)
	}
	return int64((pageID - 1) * pageSize), nil
}
