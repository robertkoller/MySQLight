package storage

import (
	"os"
	"testing"
)

func TestPager(t *testing.T) {
	// TODO: create a temp file path, defer os.Remove
	// TODO: open a Pager, allocate a page, write known bytes, close
	// TODO: reopen the Pager, read the page back, assert bytes match
	// TODO: assert PageCount() is correct after reopening
	t.Skip("not implemented")
	defer os.Remove("test_pager.db")
}

func TestBufferPool(t *testing.T) {
	// TODO: open a Pager, create a BufferPool with capacity 4
	// TODO: fetch more pages than capacity to force LRU eviction
	// TODO: assert evicted dirty pages were flushed to disk
	// TODO: pin a page, fill the pool, assert the pinned page is never evicted
	t.Skip("not implemented")
}

func TestBTreeInsertGet(t *testing.T) {
	// TODO: create a BTree over a temp pager
	// TODO: insert 10,000 key-value pairs in random order
	// TODO: call Get for every key and assert the value matches
	t.Skip("not implemented")
}

func TestBTreeDurability(t *testing.T) {
	// TODO: insert rows, close the pager, reopen it, assert all rows still exist
	t.Skip("not implemented")
}

func TestBTreeDelete(t *testing.T) {
	// TODO: insert N rows, delete half, assert deleted keys return ErrNotFound
	// TODO: assert remaining keys still return correct values
	t.Skip("not implemented")
}

func TestBTreeScan(t *testing.T) {
	// TODO: insert rows with sequential keys, scan a subrange, assert correct results in order
	t.Skip("not implemented")
}
