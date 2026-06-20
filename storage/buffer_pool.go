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

func NewBufferPool(pager *Pager, capacity int) *BufferPool {
	// TODO: initialise frames map and lru list
	// TODO: store pager and capacity on the struct
	panic("not implemented")
}

func (bp *BufferPool) FetchPage(pageID uint32) (*Page, error) {
	// TODO: if page already in frames: move to front of LRU, increment pinCount, return it
	// TODO: if pool is full: find the LRU page whose pinCount == 0 (panic/error if all are pinned)
	//         if that page is dirty, flush it to disk via pager.WritePage before evicting
	// TODO: load the page from pager.ReadPage, add to frames, set pinCount=1, return it
	panic("not implemented")
}

func (bp *BufferPool) UnpinPage(pageID uint32, dirty bool) {
	// TODO: decrement pinCount for the page (never go below 0)
	// TODO: if dirty == true, mark the page dirty
	// TODO: move page to back of LRU list — it is now a candidate for eviction
}

func (bp *BufferPool) FlushAll() error {
	// TODO: for every dirty page in frames: call pager.WritePage, then mark it clean
	panic("not implemented")
}
