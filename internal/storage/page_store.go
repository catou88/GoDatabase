// Package storage owns fixed-size page I/O primitives used by the database
// engine. It does not interpret B+Tree nodes or application keys.
package storage

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
)

const DefaultPageSize = 4096

var (
	ErrInvalidPageID = errors.New("invalid page id")
	ErrInvalidPage   = errors.New("invalid page")
)

// PageAccess is the narrow contract required by a page-backed tree. The tree
// supplies encoded pages; storage only allocates, reads, writes, and releases
// page IDs.
type PageAccess interface {
	ReadPage(pageID uint64) ([]byte, error)
	WritePage(pageID uint64, page []byte) error
	AllocatePage() (uint64, error)
	ReleasePage(pageID uint64) error
}

// FilePages provides fixed-size page I/O over a file. Page IDs start at one;
// page zero is reserved for metadata by the database format.
type FilePages struct {
	file     *os.File
	pageSize int
	pages    uint64
}

// OpenFilePages opens a page file without interpreting its contents. Existing
// bytes are preserved, allowing callers to layer metadata and recovery rules
// above this component.
func OpenFilePages(file *os.File, pageSize int) (*FilePages, error) {
	if file == nil {
		return nil, errors.New("storage: nil file")
	}
	if pageSize <= 0 {
		return nil, fmt.Errorf("storage: invalid page size %d", pageSize)
	}
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if info.Size()%int64(pageSize) != 0 {
		return nil, fmt.Errorf("storage: file size %d is not page aligned", info.Size())
	}
	return &FilePages{file: file, pageSize: pageSize, pages: uint64(info.Size() / int64(pageSize))}, nil
}

func (p *FilePages) pageOffset(pageID uint64) (int64, error) {
	if pageID == 0 || pageID > uint64(math.MaxInt64/int64(p.pageSize)) {
		return 0, fmt.Errorf("%w: %d", ErrInvalidPageID, pageID)
	}
	return int64((pageID - 1) * uint64(p.pageSize)), nil
}

// ReadPage returns a copy of a complete page.
func (p *FilePages) ReadPage(pageID uint64) ([]byte, error) {
	if pageID == 0 || pageID > p.pages {
		return nil, fmt.Errorf("%w: %d", ErrInvalidPageID, pageID)
	}
	offset, err := p.pageOffset(pageID)
	if err != nil {
		return nil, err
	}
	page := make([]byte, p.pageSize)
	n, err := p.file.ReadAt(page, offset)
	if err != nil {
		return nil, err
	}
	if n != len(page) {
		return nil, io.ErrUnexpectedEOF
	}
	return page, nil
}

// WritePage writes one complete existing page and rejects short writes.
func (p *FilePages) WritePage(pageID uint64, page []byte) error {
	if pageID == 0 || pageID > p.pages {
		return fmt.Errorf("%w: %d", ErrInvalidPageID, pageID)
	}
	if len(page) != p.pageSize {
		return fmt.Errorf("%w: size %d, want %d", ErrInvalidPage, len(page), p.pageSize)
	}
	offset, err := p.pageOffset(pageID)
	if err != nil {
		return err
	}
	n, err := p.file.WriteAt(page, offset)
	if err != nil {
		return err
	}
	if n != len(page) {
		return io.ErrShortWrite
	}
	return nil
}

// AllocatePage appends a zero-filled page and returns its one-based ID.
func (p *FilePages) AllocatePage() (uint64, error) {
	pageID := p.pages + 1
	offset, err := p.pageOffset(pageID)
	if err != nil {
		return 0, err
	}
	page := make([]byte, p.pageSize)
	n, err := p.file.WriteAt(page, offset)
	if err != nil {
		return 0, err
	}
	if n != len(page) {
		return 0, io.ErrShortWrite
	}
	p.pages = pageID
	return pageID, nil
}

// ReleasePage is intentionally a boundary-only operation. Free-list policy is
// owned by the database storage coordinator and is not guessed by this file
// primitive.
func (p *FilePages) ReleasePage(pageID uint64) error {
	if pageID == 0 || pageID > p.pages {
		return fmt.Errorf("%w: %d", ErrInvalidPageID, pageID)
	}
	return nil
}

func (p *FilePages) PageSize() int { return p.pageSize }
