package btree

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

var ErrClosed = errors.New("database is closed")

// Entry is a key-value pair returned by a range scan.
type Entry struct {
	Key   []byte
	Value []byte
}

// Mutation is one operation in an atomic KV batch.
type Mutation struct {
	Key    []byte
	Value  []byte
	Delete bool
}

// KV is a durable key-value store backed by copy-on-write B+Tree pages.
type KV struct {
	mu        sync.Mutex
	path      string
	pm        *pageManager
	tree      pageTree
	pages     map[uint64]BNode
	closed    bool
	uncertain bool
	hooks     kvHooks
}

type kvHooks struct {
	beforePageWrite func(uint64) error
	syncData        func() error
	beforeMetadata  func() error
	syncMetadata    func() error
}

// Open opens or creates a durable key-value store at path.
func Open(path string) (*KV, error) {
	if path == "" {
		return nil, fmt.Errorf("database path is empty")
	}
	_, statErr := os.Stat(path)
	created := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !created {
		return nil, statErr
	}

	pm, err := openPageManager(path)
	if err != nil {
		return nil, err
	}
	if created {
		if err := pm.file.Sync(); err != nil {
			_ = pm.close()
			return nil, err
		}
		if err := syncParentDirectory(path); err != nil {
			_ = pm.close()
			return nil, err
		}
	}

	pages, err := recoverCommittedPages(pm)
	if err != nil {
		_ = pm.close()
		return nil, err
	}
	kv := &KV{path: path, pm: pm, pages: pages}
	kv.tree.root = pm.rootPage()
	kv.installHooks()
	return kv, nil
}

// Close closes the database file.
func (kv *KV) Close() error {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	if kv.closed {
		return nil
	}
	kv.closed = true
	return kv.pm.close()
}

// Get returns a copy of the value for key and reports whether it exists.
func (kv *KV) Get(key []byte) ([]byte, bool, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	if kv.closed {
		return nil, false, ErrClosed
	}
	return kv.tree.getValue(key)
}

// Range returns copies of entries whose keys are between start and end,
// inclusive, ordered by ascending key. Empty bounds are ordinary keys, not
// unbounded-range markers.
func (kv *KV) Range(start, end []byte) ([]Entry, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	if kv.closed {
		return nil, ErrClosed
	}

	entries, err := kv.tree.rangeValues(start, end)
	if err != nil {
		return nil, err
	}
	result := make([]Entry, len(entries))
	for i, entry := range entries {
		result[i] = Entry{Key: entry.key, Value: entry.value}
	}
	return result, nil
}

// Set durably stores value for key.
func (kv *KV) Set(key, value []byte) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	if kv.closed {
		return ErrClosed
	}
	return kv.update(func() (bool, error) {
		return true, kv.tree.insert(key, value)
	})
}

// Delete durably removes key and reports whether it existed.
func (kv *KV) Delete(key []byte) (bool, error) {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	if kv.closed {
		return false, ErrClosed
	}
	deleted := false
	err := kv.update(func() (bool, error) {
		var err error
		deleted, err = kv.tree.delete(key)
		return deleted, err
	})
	return deleted, err
}

// ApplyBatch applies all mutations with one copy-on-write commit. If any
// mutation fails, no new root is published.
func (kv *KV) ApplyBatch(mutations []Mutation) error {
	kv.mu.Lock()
	defer kv.mu.Unlock()
	if kv.closed {
		return ErrClosed
	}
	return kv.update(func() (bool, error) {
		changed := false
		for _, mutation := range mutations {
			if mutation.Delete {
				deleted, err := kv.tree.delete(mutation.Key)
				if err != nil {
					return false, err
				}
				changed = changed || deleted
				continue
			}
			if err := kv.tree.insert(mutation.Key, mutation.Value); err != nil {
				return false, err
			}
			changed = true
		}
		return changed, nil
	})
}

func (kv *KV) update(change func() (bool, error)) error {
	if kv.uncertain {
		if err := kv.repair(); err != nil {
			return fmt.Errorf("repair metadata: %w", err)
		}
	}

	state := kv.pm.snapshot()
	oldRoot := kv.tree.root
	oldPages := kv.pages
	info, err := kv.pm.file.Stat()
	if err != nil {
		return err
	}
	originalSize := info.Size()
	working := clonePageMap(oldPages)
	pending := make(map[uint64]BNode)
	var callbackErr error

	kv.tree.get = func(pageID uint64) BNode {
		page, ok := working[pageID]
		if !ok && callbackErr == nil {
			callbackErr = fmt.Errorf("page %d is not loaded", pageID)
		}
		return page
	}
	kv.tree.new = func(node BNode) uint64 {
		if callbackErr != nil {
			return 0
		}
		pageID, err := kv.pm.reservePage()
		if err != nil {
			callbackErr = err
			return 0
		}
		page := BNode(append([]byte(nil), node...))
		pending[pageID] = page
		working[pageID] = page
		return pageID
	}
	kv.tree.del = func(pageID uint64) {
		if callbackErr != nil {
			return
		}
		if err := kv.pm.freePage(pageID); err != nil {
			callbackErr = err
		}
	}

	changed, changeErr := change()
	if changeErr != nil || callbackErr != nil {
		kv.rollback(state, oldRoot, oldPages, originalSize)
		return errors.Join(changeErr, callbackErr)
	}
	if !changed {
		kv.rollback(state, oldRoot, oldPages, originalSize)
		return nil
	}

	pageIDs := make([]uint64, 0, len(pending))
	for pageID := range pending {
		pageIDs = append(pageIDs, pageID)
	}
	sort.Slice(pageIDs, func(i, j int) bool { return pageIDs[i] < pageIDs[j] })
	for _, pageID := range pageIDs {
		if kv.hooks.beforePageWrite != nil {
			if err := kv.hooks.beforePageWrite(pageID); err != nil {
				kv.rollback(state, oldRoot, oldPages, originalSize)
				return err
			}
		}
		if err := kv.pm.writePage(pageID, pending[pageID]); err != nil {
			kv.rollback(state, oldRoot, oldPages, originalSize)
			return err
		}
	}
	if err := kv.hooks.syncData(); err != nil {
		kv.rollback(state, oldRoot, oldPages, originalSize)
		return err
	}
	if err := kv.pm.publishRoot(kv.tree.root, kv.hooks.beforeMetadata, kv.hooks.syncMetadata); err != nil {
		kv.pm.restore(state)
		kv.tree.root = oldRoot
		kv.pages = oldPages
		kv.uncertain = true
		kv.bindTree()
		return err
	}

	kv.pages = reachablePageMap(working, kv.tree.root)
	kv.bindTree()
	return nil
}

func (kv *KV) rollback(
	state pageManagerState,
	root uint64,
	pages map[uint64]BNode,
	originalSize int64,
) {
	kv.pm.restore(state)
	kv.tree.root = root
	kv.pages = pages
	_ = kv.pm.file.Truncate(originalSize)
	kv.bindTree()
}

func (kv *KV) repair() error {
	// Recover on the owned descriptor so no competing opener can enter here.
	info, err := kv.pm.file.Stat()
	if err != nil {
		return err
	}
	if info.Size() < metadataSize || (info.Size()-metadataSize)%pageSize != 0 {
		return fmt.Errorf("%w: file size %d is not page aligned", errInvalidPageData, info.Size())
	}
	pm := &pageManager{file: kv.pm.file, nextPageID: uint64((info.Size()-metadataSize)/pageSize) + 1}
	pages, err := recoverCommittedPages(pm)
	if err != nil {
		return err
	}
	kv.pm = pm
	kv.pages = pages
	kv.tree.root = pm.rootPage()
	kv.uncertain = false
	kv.installHooks()
	return nil
}

func (kv *KV) installHooks() {
	kv.hooks = kvHooks{
		syncData:     kv.pm.file.Sync,
		syncMetadata: kv.pm.file.Sync,
	}
	kv.bindTree()
}

func (kv *KV) bindTree() {
	kv.tree.get = func(pageID uint64) BNode { return kv.pages[pageID] }
	kv.tree.new = func(BNode) uint64 { panic("page allocation outside update") }
	kv.tree.del = func(uint64) { panic("page release outside update") }
}

func loadCommittedPages(pm *pageManager) (map[uint64]BNode, error) {
	pages := make(map[uint64]BNode)
	if pm.rootPage() == 0 {
		return pages, nil
	}
	leafDepth := -1
	visiting := make(map[uint64]bool)
	type keyBound struct {
		key []byte
		set bool
	}
	var walk func(uint64, int, keyBound, keyBound, bool) error
	walk = func(pageID uint64, depth int, lower, upper keyBound, root bool) error {
		if pageID == 0 || pageID > pm.committedPageCount {
			return fmt.Errorf("invalid child page id %d", pageID)
		}
		if visiting[pageID] {
			return fmt.Errorf("B+Tree contains a page cycle at %d", pageID)
		}
		if _, loaded := pages[pageID]; loaded {
			return fmt.Errorf("B+Tree references page %d more than once", pageID)
		}
		visiting[pageID] = true
		page, err := pm.readPage(pageID)
		if err != nil {
			return fmt.Errorf("read page %d: %w", pageID, err)
		}
		node := BNode(page)
		if err := validateBNode(node); err != nil {
			return fmt.Errorf("validate page %d: %w", pageID, err)
		}
		if node.nkeys() == 0 {
			return fmt.Errorf("page %d has no entries", pageID)
		}
		if int(node.nbytes()) > pageSize {
			return fmt.Errorf("page %d exceeds page size", pageID)
		}
		if root && len(node.getKey(0)) != 0 {
			return fmt.Errorf("root page %d is missing the lower-bound sentinel", pageID)
		}
		if root && node.btype() == nodeTypeInternal && node.nkeys() < 2 {
			return fmt.Errorf("internal root page %d has fewer than two children", pageID)
		}
		firstKey := node.getKey(0)
		lastKey := node.getKey(node.nkeys() - 1)
		if lower.set && !bytes.Equal(firstKey, lower.key) {
			return fmt.Errorf("page %d lower bound %q does not match parent key %q", pageID, firstKey, lower.key)
		}
		if upper.set && bytes.Compare(lastKey, upper.key) >= 0 {
			return fmt.Errorf("page %d key %q crosses upper bound %q", pageID, lastKey, upper.key)
		}
		pages[pageID] = node
		if node.btype() == nodeTypeLeaf {
			if leafDepth == -1 {
				leafDepth = depth
			} else if leafDepth != depth {
				return fmt.Errorf("leaf depth %d differs from %d", depth, leafDepth)
			}
		} else {
			for i := uint16(0); i < node.nkeys(); i++ {
				childID := node.getPtr(i)
				childLower := keyBound{key: node.getKey(i), set: true}
				childUpper := upper
				if i+1 < node.nkeys() {
					childUpper = keyBound{key: node.getKey(i + 1), set: true}
				}
				if err := walk(childID, depth+1, childLower, childUpper, false); err != nil {
					return err
				}
			}
			for i := uint16(0); i < node.nkeys(); i++ {
				child := pages[node.getPtr(i)]
				if int(child.nbytes()) > pageSize/4 {
					continue
				}
				if i > 0 && encodedMergeSize(pages[node.getPtr(i-1)], child) <= pageSize {
					return fmt.Errorf("page %d child %d is underfull and mergeable with its left sibling", pageID, node.getPtr(i))
				}
				if i+1 < node.nkeys() && encodedMergeSize(child, pages[node.getPtr(i+1)]) <= pageSize {
					return fmt.Errorf("page %d child %d is underfull and mergeable with its right sibling", pageID, node.getPtr(i))
				}
			}
		}
		visiting[pageID] = false
		return nil
	}
	if err := walk(pm.rootPage(), 0, keyBound{}, keyBound{}, true); err != nil {
		return nil, err
	}
	return pages, nil
}

func encodedMergeSize(left, right BNode) int {
	return pageHeaderSize + int(left.nkeys()+right.nkeys())*(pagePtrSize+pageOffsetSize) +
		int(left.getOffset(left.nkeys())) + int(right.getOffset(right.nkeys()))
}

func recoverCommittedPages(pm *pageManager) (map[uint64]BNode, error) {
	candidates := make([]rootMetadata, 0, metadataSlotCount)
	for slot := 0; slot < metadataSlotCount; slot++ {
		metadata, err := readRootMetadataSlot(pm.file, slot)
		if err != nil {
			if errors.Is(err, errInvalidPageData) {
				continue
			}
			return nil, err
		}
		candidates = append(candidates, metadata)
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("%w: no valid committed metadata", errInvalidPageData)
	}
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].generation > candidates[j].generation
	})

	var recoveryErr error
	for _, metadata := range candidates {
		if metadata.rootPageID >= pm.nextPageID {
			recoveryErr = errors.Join(recoveryErr, fmt.Errorf(
				"generation %d root page %d is not allocated",
				metadata.generation,
				metadata.rootPageID,
			))
			continue
		}
		pm.restore(pageManagerState{
			nextPageID:         pm.nextPageID,
			committedPageCount: metadata.pageCount,
			rootPageID:         metadata.rootPageID,
			generation:         metadata.generation,
			freePageIDs:        metadata.freePageIDs,
			retiredPageIDs:     metadata.retiredPageIDs,
			freeListPageIDs:    metadata.freeListPageIDs,
		})
		pages, err := loadCommittedPages(pm)
		if err == nil {
			if err := reconcilePageOwnership(pm, pages); err != nil {
				recoveryErr = errors.Join(recoveryErr, fmt.Errorf(
					"generation %d: %w",
					metadata.generation,
					err,
				))
				continue
			}
			return pages, nil
		}
		recoveryErr = errors.Join(recoveryErr, fmt.Errorf(
			"generation %d: %w",
			metadata.generation,
			err,
		))
	}
	return nil, fmt.Errorf("no valid committed B+Tree: %w", recoveryErr)
}

func reconcilePageOwnership(pm *pageManager, livePages map[uint64]BNode) error {
	owners := make(map[uint64]string, len(livePages)+len(pm.freePageIDs)+len(pm.retiredPageIDs)+len(pm.freeListPageIDs))
	claim := func(pageID uint64, owner string) error {
		if pageID == 0 || pageID >= pm.nextPageID {
			return fmt.Errorf("%s contains invalid page id %d", owner, pageID)
		}
		if previous, exists := owners[pageID]; exists {
			return fmt.Errorf("page %d is owned by both %s and %s", pageID, previous, owner)
		}
		owners[pageID] = owner
		return nil
	}
	for pageID := range livePages {
		if err := claim(pageID, "live tree"); err != nil {
			return err
		}
	}
	for _, pageID := range pm.freePageIDs {
		if err := claim(pageID, "reusable free list"); err != nil {
			return err
		}
	}
	for _, pageID := range pm.retiredPageIDs {
		if err := claim(pageID, "protected free list"); err != nil {
			return err
		}
	}
	for _, pageID := range pm.freeListPageIDs {
		if err := claim(pageID, "free-list structure"); err != nil {
			return err
		}
	}
	for pageID := uint64(1); pageID < pm.nextPageID; pageID++ {
		if _, accountedFor := owners[pageID]; accountedFor {
			continue
		}
		pm.pendingFreePageIDs = append(pm.pendingFreePageIDs, pageID)
	}
	return nil
}

func reachablePageMap(pages map[uint64]BNode, root uint64) map[uint64]BNode {
	reachable := make(map[uint64]BNode)
	var walk func(uint64)
	walk = func(pageID uint64) {
		if pageID == 0 {
			return
		}
		node := pages[pageID]
		reachable[pageID] = node
		if node.btype() == nodeTypeInternal {
			for i := uint16(0); i < node.nkeys(); i++ {
				walk(node.getPtr(i))
			}
		}
	}
	walk(root)
	return reachable
}

func clonePageMap(src map[uint64]BNode) map[uint64]BNode {
	dst := make(map[uint64]BNode, len(src))
	for pageID, page := range src {
		dst[pageID] = page
	}
	return dst
}

func syncParentDirectory(path string) error {
	directory, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	syncErr := directory.Sync()
	return errors.Join(syncErr, directory.Close())
}
