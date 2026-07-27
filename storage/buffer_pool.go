package storage

import (
	"container/list"
	"errors"

	"github.com/robertkoller/MySQLight/wal"
)

type Page struct {
	ID       uint32
	Data     []byte
	dirty    bool
	pinCount int
}

type BufferPool struct {
	pager          *Pager
	capacity       int
	frames         map[uint32]*Page
	lruList        *list.List
	lruMap         map[uint32]*list.Element
	walFile        *wal.WAL
	currentTxnID   uint64
	cleanSnapshots map[uint32][]byte // page data as of last clean flush — used as before-image
	beforeImages   map[uint32][]byte // before-image captured when page first becomes dirty
	dirtyTxn       map[uint32]uint64 // which transaction dirtied each page
}

// NewBufferPool creates a buffer pool of the given capacity backed by the provided pager.
// The pool keeps recently-used pages in memory to avoid reading from disk on every access.
// Callers must pin pages via FetchPage before use and unpin them when done.
func NewBufferPool(pager *Pager, capacity int) *BufferPool {
	return &BufferPool{
		pager:          pager,
		capacity:       capacity,
		frames:         make(map[uint32]*Page),
		lruList:        list.New(),
		lruMap:         make(map[uint32]*list.Element),
		cleanSnapshots: make(map[uint32][]byte),
		beforeImages:   make(map[uint32][]byte),
		dirtyTxn:       make(map[uint32]uint64),
	}
}

// SetWAL wires a WAL writer into the buffer pool. Once set, the pool writes a WAL UPDATE
// record (with before and after images) before flushing any dirty page to disk.
func (bp *BufferPool) SetWAL(walFile *wal.WAL) {
	bp.walFile = walFile
}

// SetTxnID records which transaction is currently performing writes. The pool uses this to
// associate dirty pages with the correct transaction ID in WAL records and RollbackTxn.
func (bp *BufferPool) SetTxnID(txnID uint64) {
	bp.currentTxnID = txnID
}

// writeWALUpdate writes a WAL UPDATE record before flushing a dirty page to disk.
// It is a no-op when no WAL is configured.
func (bp *BufferPool) writeWALUpdate(pageID uint32, afterImage []byte) {
	if bp.walFile == nil {
		return
	}
	beforeImage := bp.beforeImages[pageID]
	if beforeImage == nil {
		beforeImage = bp.cleanSnapshots[pageID]
	}
	if beforeImage == nil {
		return
	}
	txnID := bp.dirtyTxn[pageID]
	bp.walFile.WriteRecord(&wal.Record{
		TxnID:       txnID,
		Type:        wal.RecordUpdate,
		PageID:      pageID,
		BeforeImage: beforeImage,
		AfterImage:  afterImage,
	})
}

// FetchPage returns the page with the given ID, loading it from disk if it is not already
// in memory. If the pool is at capacity, the least-recently-used unpinned page is evicted;
// if that page is dirty, its WAL record is written and it is flushed to disk first. The
// returned page has its pin count incremented so the pool will not evict it while in use.
func (bp *BufferPool) FetchPage(pageID uint32) (*Page, error) {
	page, ok := bp.frames[pageID]
	if ok {
		elem := bp.lruMap[pageID]
		bp.lruList.MoveToFront(elem)
		page.pinCount++
		return page, nil
	}

	if len(bp.lruMap) >= bp.capacity {
		element := bp.lruList.Back()
		for bp.frames[element.Value.(uint32)].pinCount != 0 {
			element = element.Prev()
			if element == nil {
				return nil, errors.New("All pages are pinned")
			}
		}

		evictedID := element.Value.(uint32)
		evicted := bp.frames[evictedID]

		if evicted.dirty {
			bp.writeWALUpdate(evictedID, evicted.Data)
			if err := bp.pager.WritePage(evictedID, evicted.Data); err != nil {
				return nil, err
			}
		}

		bp.lruList.Remove(element)
		delete(bp.lruMap, evictedID)
		delete(bp.frames, evictedID)
		delete(bp.beforeImages, evictedID)
		delete(bp.dirtyTxn, evictedID)
		delete(bp.cleanSnapshots, evictedID)
	}

	loadedPage, err := bp.pager.ReadPage(pageID)
	if err != nil {
		return nil, err
	}

	formattedPage := &Page{ID: pageID, Data: loadedPage, dirty: false, pinCount: 1}
	bp.frames[pageID] = formattedPage
	bp.lruList.PushFront(pageID)
	bp.lruMap[pageID] = bp.lruList.Front()

	// Capture clean snapshot when loading from disk — this becomes the before-image
	// if this page is later dirtied by the current transaction.
	snapshot := make([]byte, len(loadedPage))
	copy(snapshot, loadedPage)
	bp.cleanSnapshots[pageID] = snapshot

	return formattedPage, nil
}

// UnpinPage signals that the caller is done with a page, decrementing its pin count
// and making it eligible for eviction. If dirty is true, the page is marked as modified
// so it will be flushed to disk before eviction. The first time a page transitions from
// clean to dirty, its current clean snapshot is recorded as the WAL before-image.
func (bp *BufferPool) UnpinPage(pageID uint32, dirty bool) error {
	page, ok := bp.frames[pageID]
	if !ok {
		return errors.New("Page not found to unpin")
	}
	if page.pinCount > 0 {
		page.pinCount--
	}

	if dirty && !page.dirty {
		// First time this page is dirtied — record before-image and owning transaction.
		if cleanSnapshot, exists := bp.cleanSnapshots[pageID]; exists {
			beforeImage := make([]byte, len(cleanSnapshot))
			copy(beforeImage, cleanSnapshot)
			bp.beforeImages[pageID] = beforeImage
		}
		bp.dirtyTxn[pageID] = bp.currentTxnID
		page.dirty = true
	} else if dirty {
		page.dirty = true
	}

	return nil
}

// FlushAll writes every dirty page currently in the pool to disk via the pager, writing
// a WAL record before each flush, and marks them clean. Called during checkpointing and
// on clean database close.
func (bp *BufferPool) FlushAll() error {
	for _, page := range bp.frames {
		if page.dirty {
			bp.writeWALUpdate(page.ID, page.Data)
			if err := bp.pager.WritePage(page.ID, page.Data); err != nil {
				return err
			}
			page.dirty = false
			// Update clean snapshot to the committed state.
			snapshot := make([]byte, len(page.Data))
			copy(snapshot, page.Data)
			bp.cleanSnapshots[page.ID] = snapshot
			delete(bp.beforeImages, page.ID)
			delete(bp.dirtyTxn, page.ID)
		}
	}
	return nil
}

// RollbackTxn restores the before-image for every page dirtied by the given transaction,
// undoing all its in-memory changes. Pages still in cache are restored in place and marked
// clean. Pages that were evicted to disk have their before-image written back via the pager.
func (bp *BufferPool) RollbackTxn(txnID uint64) error {
	for pageID, pageTxnID := range bp.dirtyTxn {
		if pageTxnID != txnID {
			continue
		}
		beforeImage, ok := bp.beforeImages[pageID]
		if !ok {
			continue
		}
		if page, inCache := bp.frames[pageID]; inCache {
			copy(page.Data, beforeImage)
			page.dirty = false
			snapshot := make([]byte, len(beforeImage))
			copy(snapshot, beforeImage)
			bp.cleanSnapshots[pageID] = snapshot
		} else {
			if err := bp.pager.WritePage(pageID, beforeImage); err != nil {
				return err
			}
		}
		delete(bp.beforeImages, pageID)
		delete(bp.dirtyTxn, pageID)
	}
	return nil
}

// AllocatePage allocates a new page via the pager.
func (bp *BufferPool) AllocatePage() (uint32, error) {
	return bp.pager.AllocatePage()
}

// fetchNode fetches a page and decodes it as a B+ tree node.
func (bp *BufferPool) fetchNode(id uint32) (*Page, *Node, error) {
	page, err := bp.FetchPage(id)
	if err != nil {
		return page, nil, err
	}
	node, err := decodeNode(id, page.Data)
	return page, node, err
}

// FreePage removes a page from the pool and releases it to the pager's free list.
func (bp *BufferPool) FreePage(pageID uint32) error {
	if element, ok := bp.lruMap[pageID]; ok {
		bp.lruList.Remove(element)
		delete(bp.lruMap, pageID)
		delete(bp.frames, pageID)
		delete(bp.beforeImages, pageID)
		delete(bp.dirtyTxn, pageID)
		delete(bp.cleanSnapshots, pageID)
	}
	return bp.pager.FreePage(pageID)
}
