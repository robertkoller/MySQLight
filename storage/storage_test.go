package storage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"path/filepath"
	"testing"
)

// helpers

func bkey(i int) []byte {
	b := make([]byte, 8)
	binary.BigEndian.PutUint64(b, uint64(i))
	return b
}

func bval(i int) []byte {
	return []byte(fmt.Sprintf("value-%d-payload-payload", i))
}

// newTree opens a fresh database in a temp dir and returns the pager, pool and an empty tree.
func newTree(t *testing.T, capacity int) (*Pager, *BufferPool, *BTree) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	pager, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	pool := NewBufferPool(pager, capacity)
	tree, err := NewBTree(pool, 0)
	if err != nil {
		t.Fatalf("NewBTree: %v", err)
	}
	return pager, pool, tree
}

// assertNoPins verifies every cached page has been unpinned — catches pin leaks.
func assertNoPins(t *testing.T, pool *BufferPool) {
	t.Helper()
	for id, page := range pool.frames {
		if page.pinCount != 0 {
			t.Errorf("page %d left pinned (pinCount=%d)", id, page.pinCount)
		}
	}
}

// Pager

func TestPagerAllocateReadWrite(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pager.db")
	pager, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if pager.PageCount() != 1 {
		t.Fatalf("fresh pager PageCount = %d, want 1 (header)", pager.PageCount())
	}

	id, err := pager.AllocatePage()
	if err != nil {
		t.Fatalf("AllocatePage: %v", err)
	}
	if id != 1 {
		t.Fatalf("first allocated page id = %d, want 1", id)
	}

	data := make([]byte, PageSize)
	copy(data, []byte("hello pager"))
	if err := pager.WritePage(id, data); err != nil {
		t.Fatalf("WritePage: %v", err)
	}

	got, err := pager.ReadPage(id)
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Fatalf("ReadPage mismatch")
	}

	// out of range
	if _, err := pager.ReadPage(99); err == nil {
		t.Fatalf("ReadPage out of range should error")
	}
	if err := pager.WritePage(id, []byte("too short")); err == nil {
		t.Fatalf("WritePage wrong size should error")
	}
}

func TestPagerDurability(t *testing.T) {
	path := filepath.Join(t.TempDir(), "durable.db")
	pager, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	id, _ := pager.AllocatePage()
	id2, _ := pager.AllocatePage()
	want := make([]byte, PageSize)
	copy(want, []byte("persisted"))
	if err := pager.WritePage(id2, want); err != nil {
		t.Fatalf("WritePage: %v", err)
	}
	pageCountBefore := pager.PageCount()
	if err := pager.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()
	if reopened.PageCount() != pageCountBefore {
		t.Fatalf("PageCount after reopen = %d, want %d", reopened.PageCount(), pageCountBefore)
	}
	got, err := reopened.ReadPage(id2)
	if err != nil {
		t.Fatalf("ReadPage after reopen: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("data did not survive reopen")
	}
	_ = id
}

func TestPagerBadMagic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.db")
	pager, _ := Open(path)
	// clobber the magic bytes
	bad := make([]byte, PageSize)
	copy(bad, []byte("NOTMYSQL!"))
	pager.WritePage(0, bad)
	pager.Close()

	if _, err := Open(path); err == nil {
		t.Fatalf("Open should reject a file with bad magic bytes")
	}
}

// BufferPool

func TestBufferPoolCachesAndPins(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bp.db")
	pager, _ := Open(path)
	defer pager.Close()
	pool := NewBufferPool(pager, 4)

	id, _ := pool.AllocatePage()
	first, err := pool.FetchPage(id)
	if err != nil {
		t.Fatalf("FetchPage: %v", err)
	}
	// second fetch of the same page returns the same cached object and bumps the pin
	second, _ := pool.FetchPage(id)
	if first != second {
		t.Fatalf("FetchPage did not return cached page object")
	}
	if first.pinCount != 2 {
		t.Fatalf("pinCount = %d, want 2 after two fetches", first.pinCount)
	}
	pool.UnpinPage(id, false)
	pool.UnpinPage(id, false)
	if first.pinCount != 0 {
		t.Fatalf("pinCount = %d, want 0 after two unpins", first.pinCount)
	}
}

func TestBufferPoolEvictsDirty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bpdirty.db")
	pager, _ := Open(path)
	defer pager.Close()
	pool := NewBufferPool(pager, 3)

	// allocate the victim and write through the pool, then unpin dirty
	victim, _ := pool.AllocatePage()
	page, _ := pool.FetchPage(victim)
	copy(page.Data, []byte("dirty-data"))
	pool.UnpinPage(victim, true)

	// fetch enough other pages to force the victim out
	for i := 0; i < 5; i++ {
		id, _ := pool.AllocatePage()
		p, _ := pool.FetchPage(id)
		pool.UnpinPage(id, false)
		_ = p
	}

	if _, ok := pool.frames[victim]; ok {
		t.Fatalf("victim should have been evicted")
	}
	// the dirty victim must have been flushed to disk on eviction
	got, err := pager.ReadPage(victim)
	if err != nil {
		t.Fatalf("ReadPage: %v", err)
	}
	if !bytes.HasPrefix(got, []byte("dirty-data")) {
		t.Fatalf("dirty page was not flushed before eviction")
	}
}

func TestBufferPoolPinnedNeverEvicted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bppin.db")
	pager, _ := Open(path)
	defer pager.Close()
	pool := NewBufferPool(pager, 2)

	pinned, _ := pool.AllocatePage()
	if _, err := pool.FetchPage(pinned); err != nil { // stays pinned
		t.Fatalf("FetchPage: %v", err)
	}

	a, _ := pool.AllocatePage()
	pool.FetchPage(a)
	pool.UnpinPage(a, false)
	b, _ := pool.AllocatePage()
	pool.FetchPage(b)
	pool.UnpinPage(b, false)

	if _, ok := pool.frames[pinned]; !ok {
		t.Fatalf("pinned page was evicted")
	}
}

func TestBufferPoolAllPinnedErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bpfull.db")
	pager, _ := Open(path)
	defer pager.Close()
	pool := NewBufferPool(pager, 2)

	a, _ := pool.AllocatePage()
	pool.FetchPage(a)
	b, _ := pool.AllocatePage()
	pool.FetchPage(b)
	c, _ := pool.AllocatePage()
	if _, err := pool.FetchPage(c); err == nil {
		t.Fatalf("FetchPage should error when all pages are pinned")
	}
}

func TestBufferPoolFlushAll(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bpflush.db")
	pager, _ := Open(path)
	defer pager.Close()
	pool := NewBufferPool(pager, 8)

	id, _ := pool.AllocatePage()
	page, _ := pool.FetchPage(id)
	copy(page.Data, []byte("flushme"))
	pool.UnpinPage(id, true)

	if err := pool.FlushAll(); err != nil {
		t.Fatalf("FlushAll: %v", err)
	}
	got, _ := pager.ReadPage(id)
	if !bytes.HasPrefix(got, []byte("flushme")) {
		t.Fatalf("FlushAll did not write dirty page")
	}
}

// BTree Insert / Get

func TestBTreeInsertGet(t *testing.T) {
	_, pool, tree := newTree(t, 128)

	const n = 5000
	// insert in a scrambled order to exercise splits
	for i := 0; i < n; i++ {
		k := (i*2654435761 + 12345) % n
		if err := tree.Insert(bkey(k), bval(k)); err != nil {
			// the scramble can repeat; skip dup collisions deterministically below instead
			t.Fatalf("insert %d: %v", k, err)
		}
	}
	for i := 0; i < n; i++ {
		got, err := tree.Get(bkey(i))
		if err != nil {
			t.Fatalf("Get %d: %v", i, err)
		}
		if !bytes.Equal(got, bval(i)) {
			t.Fatalf("Get %d = %q, want %q", i, got, bval(i))
		}
	}
	assertNoPins(t, pool)
}

func TestBTreeInsertErrors(t *testing.T) {
	_, _, tree := newTree(t, 16)

	if err := tree.Insert(bkey(1), bval(1)); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if err := tree.Insert(bkey(1), bval(99)); err != ErrDuplicateKey {
		t.Fatalf("duplicate insert err = %v, want ErrDuplicateKey", err)
	}
	if _, err := tree.Get(bkey(404)); err != ErrNotFound {
		t.Fatalf("Get missing err = %v, want ErrNotFound", err)
	}
	huge := make([]byte, PageSize)
	if err := tree.Insert(bkey(2), huge); err != ErrEntryTooLarge {
		t.Fatalf("oversized insert err = %v, want ErrEntryTooLarge", err)
	}
}

func TestBTreeDurability(t *testing.T) {
	path := filepath.Join(t.TempDir(), "treedurable.db")
	pager, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	pool := NewBufferPool(pager, 64)
	tree, _ := NewBTree(pool, 0)

	const n = 3000
	for i := 0; i < n; i++ {
		if err := tree.Insert(bkey(i), bval(i)); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	rootID := tree.rootPageID // root can change across splits; capture for reopen
	if err := pool.FlushAll(); err != nil {
		t.Fatalf("FlushAll: %v", err)
	}
	if err := pager.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	pager2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer pager2.Close()
	pool2 := NewBufferPool(pager2, 64)
	tree2, err := NewBTree(pool2, rootID)
	if err != nil {
		t.Fatalf("NewBTree reopen: %v", err)
	}
	for i := 0; i < n; i++ {
		got, err := tree2.Get(bkey(i))
		if err != nil {
			t.Fatalf("Get %d after reopen: %v", i, err)
		}
		if !bytes.Equal(got, bval(i)) {
			t.Fatalf("Get %d after reopen = %q, want %q", i, got, bval(i))
		}
	}
	assertNoPins(t, pool2)
}

// BTree Delete

func TestBTreeDelete(t *testing.T) {
	_, pool, tree := newTree(t, 128)

	const n = 4000
	for i := 0; i < n; i++ {
		if err := tree.Insert(bkey(i), bval(i)); err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
	}
	// delete every odd key
	for i := 1; i < n; i += 2 {
		if err := tree.Delete(bkey(i)); err != nil {
			t.Fatalf("delete %d: %v", i, err)
		}
	}
	for i := 0; i < n; i++ {
		got, err := tree.Get(bkey(i))
		if i%2 == 0 {
			if err != nil || !bytes.Equal(got, bval(i)) {
				t.Fatalf("even key %d should remain: err=%v", i, err)
			}
		} else {
			if err != ErrNotFound {
				t.Fatalf("odd key %d should be gone: err=%v", i, err)
			}
		}
	}
	// deleting a missing key returns ErrNotFound
	if err := tree.Delete(bkey(1)); err != ErrNotFound {
		t.Fatalf("re-delete err = %v, want ErrNotFound", err)
	}
	assertNoPins(t, pool)
}

func TestBTreeDeleteAll(t *testing.T) {
	_, pool, tree := newTree(t, 128)

	const n = 2000
	for i := 0; i < n; i++ {
		tree.Insert(bkey(i), bval(i))
	}
	for i := 0; i < n; i++ {
		if err := tree.Delete(bkey(i)); err != nil {
			t.Fatalf("delete %d: %v", i, err)
		}
	}
	for i := 0; i < n; i++ {
		if _, err := tree.Get(bkey(i)); err != ErrNotFound {
			t.Fatalf("key %d should be gone after delete-all: %v", i, err)
		}
	}
	// tree is empty but usable: a fresh insert/get round-trips
	if err := tree.Insert(bkey(42), bval(42)); err != nil {
		t.Fatalf("insert into emptied tree: %v", err)
	}
	if got, err := tree.Get(bkey(42)); err != nil || !bytes.Equal(got, bval(42)) {
		t.Fatalf("get from emptied tree: err=%v", err)
	}
	assertNoPins(t, pool)
}

// BTree Scan

func scanAll(t *testing.T, tree *BTree, start, end []byte) [][2][]byte {
	t.Helper()
	it, err := tree.Scan(start, end)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	defer it.Close()
	var out [][2][]byte
	for {
		k, v, err := it.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Next: %v", err)
		}
		out = append(out, [2][]byte{k, v})
	}
	return out
}

func TestBTreeScan(t *testing.T) {
	_, pool, tree := newTree(t, 128)

	const n = 500
	for i := 0; i < n; i++ {
		tree.Insert(bkey(i), bval(i))
	}

	// full scan in order
	all := scanAll(t, tree, nil, nil)
	if len(all) != n {
		t.Fatalf("full scan returned %d, want %d", len(all), n)
	}
	for i, kv := range all {
		if !bytes.Equal(kv[0], bkey(i)) || !bytes.Equal(kv[1], bval(i)) {
			t.Fatalf("full scan out of order at %d: key=%v", i, kv[0])
		}
	}

	// bounded scan [100, 200) — exclusive upper bound
	sub := scanAll(t, tree, bkey(100), bkey(200))
	if len(sub) != 100 {
		t.Fatalf("bounded scan returned %d, want 100", len(sub))
	}
	if !bytes.Equal(sub[0][0], bkey(100)) || !bytes.Equal(sub[len(sub)-1][0], bkey(199)) {
		t.Fatalf("bounded scan endpoints wrong: first=%v last=%v", sub[0][0], sub[len(sub)-1][0])
	}

	// start past the end yields nothing
	empty := scanAll(t, tree, bkey(99999), nil)
	if len(empty) != 0 {
		t.Fatalf("scan past end returned %d, want 0", len(empty))
	}

	// start in a gap lands on the next present key
	tree.Delete(bkey(250))
	gap := scanAll(t, tree, bkey(250), bkey(253))
	if len(gap) != 2 || !bytes.Equal(gap[0][0], bkey(251)) {
		t.Fatalf("scan from deleted key wrong: %d entries, first=%v", len(gap), gap[0][0])
	}

	assertNoPins(t, pool)
}

func TestBTreeScanEmptyTreeAndEarlyClose(t *testing.T) {
	_, pool, tree := newTree(t, 16)

	// scan of an empty tree returns EOF immediately
	it, err := tree.Scan(nil, nil)
	if err != nil {
		t.Fatalf("Scan: %v", err)
	}
	if _, _, err := it.Next(); err != io.EOF {
		t.Fatalf("empty scan Next = %v, want io.EOF", err)
	}
	it.Close()

	for i := 0; i < 300; i++ {
		tree.Insert(bkey(i), bval(i))
	}
	// stop early then Close — must release the held pin (no leaks)
	it2, _ := tree.Scan(nil, nil)
	it2.Next()
	it2.Next()
	if err := it2.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Close is idempotent
	if err := it2.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
	assertNoPins(t, pool)
}

// Freelist

func TestPagerFreelistReuse(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fl.db")
	pager, _ := Open(path)
	defer pager.Close()

	a, _ := pager.AllocatePage() // 1
	b, _ := pager.AllocatePage() // 2
	c, _ := pager.AllocatePage() // 3
	if a != 1 || b != 2 || c != 3 {
		t.Fatalf("unexpected ids: %d %d %d", a, b, c)
	}
	countBefore := pager.PageCount()

	if err := pager.FreePage(b); err != nil {
		t.Fatalf("FreePage: %v", err)
	}
	if err := pager.FreePage(c); err != nil {
		t.Fatalf("FreePage: %v", err)
	}

	// LIFO: last freed (c) comes back first, then b — without growing the file
	if got, _ := pager.AllocatePage(); got != c {
		t.Fatalf("reuse 1 = %d, want %d", got, c)
	}
	if got, _ := pager.AllocatePage(); got != b {
		t.Fatalf("reuse 2 = %d, want %d", got, b)
	}
	if pager.PageCount() != countBefore {
		t.Fatalf("PageCount grew to %d while reusing freed pages (want %d)", pager.PageCount(), countBefore)
	}

	// freelist empty again -> next allocate extends the file
	if got, _ := pager.AllocatePage(); got != countBefore {
		t.Fatalf("post-freelist allocate = %d, want %d", got, countBefore)
	}
	if pager.PageCount() != countBefore+1 {
		t.Fatalf("PageCount = %d, want %d", pager.PageCount(), countBefore+1)
	}
}

func TestPagerFreelistDurable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fldur.db")
	pager, _ := Open(path)
	pager.AllocatePage()         // 1
	b, _ := pager.AllocatePage() // 2
	c, _ := pager.AllocatePage() // 3
	pager.FreePage(b)
	pager.FreePage(c) // head = c, c -> b
	if err := pager.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	re, err := Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer re.Close()
	// the freelist head survived the reopen
	if got, _ := re.AllocatePage(); got != c {
		t.Fatalf("after reopen reuse = %d, want %d", got, c)
	}
	if got, _ := re.AllocatePage(); got != b {
		t.Fatalf("after reopen reuse 2 = %d, want %d", got, b)
	}
}

func TestBufferPoolFreePageDiscards(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fldisc.db")
	pager, _ := Open(path)
	defer pager.Close()
	pool := NewBufferPool(pager, 8)

	id, _ := pool.AllocatePage()
	page, _ := pool.FetchPage(id)
	copy(page.Data, []byte("DEADBEEF"))
	pool.UnpinPage(id, true) // now dirty in the pool

	if err := pool.FreePage(id); err != nil {
		t.Fatalf("FreePage: %v", err)
	}
	if _, ok := pool.frames[id]; ok {
		t.Fatalf("freed page is still cached in the pool")
	}
	// FlushAll must NOT write the discarded dirty bytes over the freelist link
	if err := pool.FlushAll(); err != nil {
		t.Fatalf("FlushAll: %v", err)
	}
	got, _ := pager.ReadPage(id)
	if bytes.HasPrefix(got, []byte("DEADBEEF")) {
		t.Fatalf("discarded dirty page was flushed over the freelist link")
	}
}

func TestBTreeFreelistRecycling(t *testing.T) {
	pager, pool, tree := newTree(t, 128)

	const n = 3000
	for i := 0; i < n; i++ {
		tree.Insert(bkey(i), bval(i))
	}
	peak := pager.PageCount()

	for i := 0; i < n; i++ {
		if err := tree.Delete(bkey(i)); err != nil {
			t.Fatalf("delete %d: %v", i, err)
		}
	}
	// deletes only free pages; the file never shrinks and never grows
	if pager.PageCount() != peak {
		t.Fatalf("PageCount changed during deletes: %d -> %d", peak, pager.PageCount())
	}

	// reinserting reuses freed pages from the freelist instead of growing the file.
	// Without a freelist this would allocate ~peak brand-new pages (roughly doubling the
	// file); with it, the count stays within a couple of pages of the peak.
	for i := 0; i < n; i++ {
		tree.Insert(bkey(i), bval(i))
	}
	if pager.PageCount() > peak+4 {
		t.Fatalf("file grew despite the freelist (reuse not happening): peak=%d now=%d", peak, pager.PageCount())
	}

	for i := 0; i < n; i++ {
		got, err := tree.Get(bkey(i))
		if err != nil || !bytes.Equal(got, bval(i)) {
			t.Fatalf("key %d wrong after recycle: err=%v", i, err)
		}
	}
	assertNoPins(t, pool)
}
