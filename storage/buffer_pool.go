package storage

type Page struct {
	ID       uint32
	Data     []byte
	dirty    bool
	pinCount int
}

type BufferPool struct {
	// TODO: pager    *Pager
	// TODO: capacity int
	// TODO: frames   map[uint32]*Page   — pages currently loaded in memory
	// TODO: lruList  — doubly linked list tracking eviction order (container/list)
}

// NewBufferPool creates a buffer pool of the given capacity backed by the provided pager.
// The pool keeps recently-used pages in memory to avoid reading from disk on every access.
// Callers must pin pages via FetchPage before use and unpin them when done.
func NewBufferPool(pager *Pager, capacity int) *BufferPool {
	// TODO: initialise frames map and lru list
	// TODO: store pager and capacity on the struct
	panic("not implemented")
}

// FetchPage returns the page with the given ID, loading it from disk if it is not already
// in memory. If the pool is at capacity, the least-recently-used unpinned page is evicted;
// if that page is dirty, it is flushed to disk first. The returned page has its pin count
// incremented so the pool will not evict it while a caller holds a reference.
func (bp *BufferPool) FetchPage(pageID uint32) (*Page, error) {
	// TODO: if page already in frames: move to front of LRU, increment pinCount, return it
	// TODO: if pool is full: find the LRU page whose pinCount == 0 (panic/error if all are pinned)
	//         if that page is dirty, flush it to disk via pager.WritePage before evicting
	// TODO: load the page from pager.ReadPage, add to frames, set pinCount=1, return it
	panic("not implemented")
}

// UnpinPage signals that the caller is done with a page, decrementing its pin count
// and making it eligible for eviction. If dirty is true, the page is marked as modified
// so it will be flushed to disk before it is evicted from the pool.
func (bp *BufferPool) UnpinPage(pageID uint32, dirty bool) {
	// TODO: decrement pinCount for the page (never go below 0)
	// TODO: if dirty == true, mark the page dirty
	// TODO: move page to back of LRU list — it is now a candidate for eviction
}

// FlushAll writes every dirty page currently in the pool to disk via the pager and
// marks them clean. This is called during checkpointing and when the database is closing
// to ensure no modified data is left only in memory.
func (bp *BufferPool) FlushAll() error {
	// TODO: for every dirty page in frames: call pager.WritePage, then mark it clean
	panic("not implemented")
}
