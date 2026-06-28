package storage

import (
	"container/list"
	"errors"
)

type Page struct {
	ID       uint32
	Data     []byte
	dirty    bool
	pinCount int
}

type BufferPool struct {
	// frames   pages currently loaded in memory
	// lruList  doubly linked list tracking eviction order
	pager    *Pager
	capacity int
	frames   map[uint32]*Page
	lruList  *list.List
	lruMap   map[uint32]*list.Element
}

// NewBufferPool creates a buffer pool of the given capacity backed by the provided pager.
// The pool keeps recently-used pages in memory to avoid reading from disk on every access.
// Callers must pin pages via FetchPage before use and unpin them when done.
func NewBufferPool(pager *Pager, capacity int) *BufferPool {
	frames := make(map[uint32]*Page)
	lruList := list.New()
	lruMap := make(map[uint32]*list.Element)

	return &BufferPool{pager: pager, capacity: capacity, frames: frames, lruList: lruList, lruMap: lruMap}
}

// FetchPage returns the page with the given ID, loading it from disk if it is not already
// in memory. If the pool is at capacity, the least-recently-used unpinned page is evicted;
// if that page is dirty, it is flushed to disk first. The returned page has its pin count
// incremented so the pool will not evict it while a caller holds a reference.
func (bp *BufferPool) FetchPage(pageID uint32) (*Page, error) {
	page, ok := bp.frames[pageID]
	if ok {
		elem := bp.lruMap[pageID]
		bp.lruList.MoveToFront(elem)
		page.pinCount++
		return page, nil
	}

	// This cleans up the cache if it is full
	if len(bp.lruMap) >= bp.capacity {
		element := bp.lruList.Back()
		for bp.frames[element.Value.(uint32)].pinCount != 0 {
			element = element.Prev()
			if element == nil {
				return nil, errors.New("All pages are pinned")
			}
		}

		value := element.Value.(uint32)
		evicted := bp.frames[value]

		if evicted.dirty {

			if err := bp.pager.WritePage(value, evicted.Data); err != nil {
				return nil, err
			}
		}

		bp.lruList.Remove(element)
		delete(bp.lruMap, value)
		delete(bp.frames, value)

	}

	loadedPage, err := bp.pager.ReadPage(pageID)
	if err != nil {
		return nil, err
	}

	formattedPage := &Page{ID: pageID, Data: loadedPage, dirty: false, pinCount: 1}
	bp.frames[pageID] = formattedPage
	bp.lruList.PushFront(pageID)
	bp.lruMap[pageID] = bp.lruList.Front()

	return formattedPage, nil
}

// UnpinPage signals that the caller is done with a page, decrementing its pin count
// and making it eligible for eviction. If dirty is true, the page is marked as modified
// so it will be flushed to disk before it is evicted from the pool.
func (bp *BufferPool) UnpinPage(pageID uint32, dirty bool) error {
	page, ok := bp.frames[pageID]
	if !ok {
		return errors.New("Page not found to unpin")
	}
	if page.pinCount > 0 {
		page.pinCount--
	}

	if dirty {
		page.dirty = true
	}

	return nil
}

// FlushAll writes every dirty page currently in the pool to disk via the pager and
// marks them clean. This is called during checkpointing and when the database is closing
// to ensure no modified data is left only in memory.
func (bp *BufferPool) FlushAll() error {
	for _, page := range bp.frames {
		if page.dirty {
			if err := bp.pager.WritePage(page.ID, page.Data); err != nil {
				return err
			}
			page.dirty = false
		}
	}
	return nil
}

// This will be changed later with freelist
func (bp *BufferPool) AllocatePage() (uint32, error) {
	return bp.pager.AllocatePage()
}

// Fetches and creates a node object representing a certain page
func (bp *BufferPool) fetchNode(id uint32) (*Page, *Node, error) {
	page, err := bp.FetchPage(id)
	if err != nil {
		return page, nil, err
	}

	node, err := decodeNode(id, page.Data)
	return page, node, err

}
