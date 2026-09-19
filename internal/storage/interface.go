package storage

// PageAccess is the narrow contract required by a page-backed tree. The tree
// supplies encoded pages; storage only allocates, reads, writes, and releases
// page IDs.
type PageAccess interface {
	ReadPage(pageID uint64) ([]byte, error)
	WritePage(pageID uint64, page []byte) error
	AllocatePage() (uint64, error)
	ReleasePage(pageID uint64) error
}
