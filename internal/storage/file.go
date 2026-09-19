package storage

import (
	"io"
	"os"
)

// File is the file capability required by FilePages. It keeps page storage
// independent from the concrete file implementation used by the composition
// root and tests.
type File interface {
	io.ReaderAt
	io.WriterAt
	Stat() (os.FileInfo, error)
}

var _ File = (*os.File)(nil)
