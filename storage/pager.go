package storage

import (
	"errors"
	"os"
)

const PageSize = 4096

// Header layout for page 0 (byte offsets):
//   0–8   magic bytes ("MYSQLIGHT")
//   9–10  page size (uint16)
//   11–14 page count (uint32)
//   15–18 catalog root page ID (uint32)
//   19–26 last checkpointed WAL LSN (uint64)

type Pager struct {
	file      *os.File
	pageCount uint32
}

func Open(path string) (*Pager, error) {
	// TODO: os.OpenFile with O_RDWR|O_CREATE
	// TODO: if new file (size == 0): write a fresh header page with magic bytes, PageSize, pageCount=1
	// TODO: if existing file: read page 0, check magic bytes, read pageCount into p.pageCount
	panic("not implemented")
}

func (p *Pager) ReadPage(pageID uint32) ([]byte, error) {
	if pageID >= p.pageCount {
		return nil, errors.New("PageID is bigger than the number of stored pages.")
	}
	// TODO: return error if pageID >= p.pageCount
	// TODO: seek to int64(pageID) * PageSize
	// TODO: read exactly PageSize bytes into a fresh buffer and return it
	panic("not implemented")
}

func (p *Pager) WritePage(pageID uint32, data []byte) error {
	// TODO: return error if pageID >= p.pageCount or len(data) != PageSize
	// TODO: seek to int64(pageID) * PageSize
	// TODO: write all PageSize bytes
	panic("not implemented")
}

func (p *Pager) AllocatePage() (uint32, error) {
	// TODO: newID = p.pageCount; p.pageCount++
	// TODO: update the pageCount field in the header (page 0) on disk
	// TODO: return newID — the caller is responsible for writing valid content to it
	panic("not implemented")
}

func (p *Pager) FreePage(pageID uint32) error {
	// TODO: delegate to the Freelist (freelist.go)
	panic("not implemented")
}

func (p *Pager) PageCount() uint32 {
	return p.pageCount
}

func (p *Pager) Close() error {
	// TODO: sync the file (f.Sync()) then close it
	panic("not implemented")
}
