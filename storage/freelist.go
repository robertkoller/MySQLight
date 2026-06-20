package storage

// Freelist tracks pages that have been freed and can be reused.
// It is persisted in a reserved page range at a known location in the file.

type Freelist struct {
	// TODO: pager      *Pager
	// TODO: freePages  []uint32 — in-memory list of reusable page IDs
	// TODO: headerPageID uint32  — the page where the freelist is persisted
}

func NewFreelist(pager *Pager, headerPageID uint32) (*Freelist, error) {
	// TODO: read headerPageID from the database header (page 0)
	// TODO: if the freelist page exists, decode the list of free page IDs from it
	// TODO: return an initialised Freelist
	panic("not implemented")
}

func (fl *Freelist) Allocate() (uint32, error) {
	// TODO: if freePages is non-empty: pop the last entry and return it
	// TODO: otherwise: call pager.AllocatePage() to extend the file
	panic("not implemented")
}

func (fl *Freelist) Free(pageID uint32) error {
	// TODO: append pageID to freePages
	// TODO: persist the updated list to the freelist header page
	panic("not implemented")
}
