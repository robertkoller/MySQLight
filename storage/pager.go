package storage

import (
	"encoding/binary"
	"errors"
	"io"
	"os"
)

const PageSize = 4096
const magicByte = "MYSQLIGHT"

// Header layout for page 0 (byte offsets):
//   0–8   magic bytes ("MYSQLIGHT")
//   9–10  page size (uint16)
//   11–14 page count (uint32)
//   15–18 catalog root page ID (uint32)
//   19–26 last checkpointed WAL LSN (uint64)
//   27-30 freeListHead (uint32)

type Pager struct {
	pages        *os.File
	freeListHead uint32
	pageCount    uint32
}

// createHeader builds a fresh PageSize-byte slice formatted as the database header page.
// It writes the magic string at offset 0, the page size at offset 9, the initial page count
// at offset 11, the catalog root page ID at offset 15, and the WAL LSN at offset 19.
// All multi-byte integers are encoded in big-endian byte order.
func createHeader(pageCount uint32, cataLogID uint32, WAL_LSN uint64, freeListHead uint32) []byte {
	header := make([]byte, PageSize)

	copy(header[0:], []byte(magicByte))

	binary.BigEndian.PutUint16(header[9:], PageSize)
	binary.BigEndian.PutUint32(header[11:], pageCount)
	binary.BigEndian.PutUint32(header[15:], cataLogID)
	binary.BigEndian.PutUint64(header[19:], WAL_LSN)
	binary.BigEndian.PutUint32(header[27:], freeListHead)

	return header

}

// Open opens the database file at the given path, creating it if it does not exist.
// If the file is new (size zero), Open writes a fresh header page containing the magic
// bytes, the page size, and an initial page count of one. If the file already exists,
// Open reads page zero, verifies the magic bytes to confirm this is a valid MySQLight
// database, and loads the stored page count into the Pager so it knows how many pages
// are on disk.
func Open(path string) (*Pager, error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		return nil, err
	}

	if info.Size() == 0 {
		if _, err = file.Write(createHeader(1, 0, 0, 0)); err != nil {
			return nil, err
		}
		return &Pager{pages: file, pageCount: 1}, nil
	}

	buffer := make([]byte, PageSize)
	if _, err = io.ReadFull(file, buffer); err != nil {
		return nil, err
	}

	if string(buffer[0:9]) != magicByte {
		return nil, errors.New("invalid database file: bad magic bytes")
	}

	pageCount := binary.BigEndian.Uint32(buffer[11:])
	freeListPage := binary.BigEndian.Uint32(buffer[27:])
	return &Pager{pages: file, freeListHead: freeListPage, pageCount: pageCount}, nil
}

// ReadPage reads exactly PageSize bytes from the page identified by pageID and returns
// them as a fresh slice. It returns an error if pageID is out of range or if the read
// fails before filling the buffer.
func (p *Pager) ReadPage(pageID uint32) ([]byte, error) {
	if pageID >= p.pageCount {
		return nil, errors.New("PageID is bigger than the number of stored pages.")
	}

	buffer := make([]byte, PageSize)
	p.pages.Seek(int64(pageID)*PageSize, 0)

	if _, err := io.ReadFull(p.pages, buffer); err != nil {
		return nil, err
	}
	return buffer, nil
}

// WritePage writes exactly PageSize bytes from data to the page identified by pageID.
// It returns an error if pageID is out of range or if data is not exactly PageSize bytes,
// since writing a partial page would corrupt the fixed-size page layout on disk.
// The caller is responsible for ensuring the data is fully formed before calling WritePage.
func (p *Pager) WritePage(pageID uint32, data []byte) error {
	if pageID >= p.pageCount {
		return errors.New("PageID does not exist")
	}

	if len(data) != PageSize {
		return errors.New("Invalid length of data")
	}

	p.pages.Seek(int64(pageID)*PageSize, 0)
	if _, err := p.pages.Write(data); err != nil {
		return err
	}
	return nil

}

// AllocatePage reserves a new page at the end of the database file and returns its page ID.
// It increments the in-memory page count and persists the updated count to the header on disk
// so that the new page survives a restart. AllocatePage does not write any content to the new
// page — that is the caller's responsibility.
func (p *Pager) AllocatePage() (uint32, error) {
	if p.freeListHead != 0 {
		id := p.freeListHead
		buffer, err := p.ReadPage(id)
		if err != nil {
			return 0, err
		}

		p.freeListHead = binary.BigEndian.Uint32(buffer[0:])
		if err := p.writeFreeListHead(); err != nil {
			return 0, err
		}
		return id, nil
	}
	newID := p.pageCount

	// This section makes it so that the file is immediately readable
	buffer := make([]byte, PageSize)
	p.pages.Seek(int64(newID)*PageSize, 0)
	if _, err := p.pages.Write(buffer); err != nil {
		return 0, err
	}

	// this section just updates the header
	p.pageCount++
	p.pages.Seek(11, 0)
	count := make([]byte, 4)
	binary.BigEndian.PutUint32(count, p.pageCount)
	if _, err := p.pages.Write(count); err != nil {
		return 0, err
	}

	return newID, nil

}

// FreePage marks a page as no longer in use so it can be reclaimed by a future AllocatePage call.
// Rather than shrinking the file, freed pages are tracked in the freelist (freelist.go) and
// handed out again when new pages are needed, which avoids fragmentation and expensive file truncation.
func (p *Pager) FreePage(pageID uint32) error {
	buffer := make([]byte, PageSize)
	binary.BigEndian.PutUint32(buffer[0:], p.freeListHead)

	if err := p.WritePage(pageID, buffer); err != nil {
		return err
	}

	p.freeListHead = pageID
	return p.writeFreeListHead()
}

// PageCount returns the number of pages currently allocated in the database file,
// including page zero which holds the header.
func (p *Pager) PageCount() uint32 {
	return p.pageCount
}

// Close flushes all pending writes to disk and closes the underlying file handle.
// It calls Sync before Close to guarantee that any data the OS has buffered is
// durably written to storage, preventing data loss if the process exits immediately after.
func (p *Pager) Close() error {
	if err := p.pages.Sync(); err != nil {
		return err
	}
	if err := p.pages.Close(); err != nil {
		return err
	}
	return nil
}

func (p *Pager) writeFreeListHead() error {
	p.pages.Seek(27, 0)
	bytes := make([]byte, 4)
	binary.BigEndian.PutUint32(bytes, p.freeListHead)
	_, err := p.pages.Write(bytes)
	return err
}
