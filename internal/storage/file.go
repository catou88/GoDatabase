package storage

import (
	"io"
	"os"
)

// File is the capability required by fixed-page storage. Keeping this
// interface separate from FilePages allows tests and future storage backends
// to inject short-I/O and synchronization failures.
type File interface {
	io.ReaderAt
	io.WriterAt
	Stat() (os.FileInfo, error)
}

var _ File = (*os.File)(nil)
