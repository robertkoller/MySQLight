# `buffer_pool.go` — LRU Page Cache

Reading from disk on every tree traversal is too slow, and `Pager.ReadPage`
allocates a new buffer each call. The buffer pool keeps recently used pages in
memory, hands out shared references to them, and writes them back to disk lazily.
It sits between the [BTree](btree.md) and the [Pager](pager.md), and the tree
talks **only** to the pool — never the pager directly.

---

## Types

```go
type Page struct {
    ID       uint32
    Data     []byte   // the 4096-byte page contents (shared, mutated in place)
    dirty    bool     // modified in memory but not yet written to disk
    pinCount int      // number of callers currently using this page
}

type BufferPool struct {
    pager    *Pager
    capacity int                          // max pages held in memory
    frames   map[uint32]*Page             // pageID -> cached page
    lruList  *list.List                   // eviction order (front = MRU, back = LRU)
    lruMap   map[uint32]*list.Element     // pageID -> its node in lruList
}
```

`Data` is **shared**: a caller that mutates it mutates the cached copy directly.
That is the point — but it means callers must signal "I changed this" via
`UnpinPage(id, true)` so the page is eventually flushed.

---

## Functions

### `NewBufferPool(pager, capacity) *BufferPool`
Allocates the maps and the LRU list; stores the pager and capacity.

### `FetchPage(pageID) (*Page, error)`
Returns the requested page, pinned (`pinCount`++).
- **Cache hit:** move it to the front of the LRU list, increment the pin count,
  return it.
- **Cache full** (`len >= capacity`): walk the LRU list from the back for an
  **unpinned** victim. If every page is pinned, return an error. If the victim is
  dirty, flush it via `pager.WritePage`, then remove it from the list and both maps.
- **Cache miss:** load via `pager.ReadPage`, wrap in a `Page` with `pinCount = 1`,
  insert at the front of the LRU list and into the maps.

### `UnpinPage(pageID, dirty) error`
Signals the caller is done.
- Returns an error if the page isn't in the pool (callers that may have freed the
  page intentionally ignore this).
- Decrements `pinCount` (guarded so it never goes below 0).
- If `dirty`, sets the dirty flag. The flag is **sticky** — cleared only by a
  flush, never by `dirty == false`.

> Note: `UnpinPage` does not reorder the LRU list; recency is tracked by
> `FetchPage` (move-to-front on access). Eviction picks the least-recently-*fetched*
> unpinned page.

### `FlushAll() error`
Writes every dirty page to disk via the pager and marks it clean. Used at
checkpoint and on close so no modified data is left only in memory.

### `AllocatePage() (uint32, error)`
Delegates to `pager.AllocatePage` (which itself prefers the freelist). The caller
then `FetchPage`es the returned ID and overwrites it with a fresh node header.
This works for both brand-new pages (the pager zeroes them on disk before
returning) and recycled pages (they already exist on disk).

### `FreePage(pageID) error`
Drops the page from the cache **without flushing** (it's dead), then calls
`pager.FreePage` to push it onto the freelist. Dropping first is essential — see
the gotcha in [freelist.md](freelist.md). Safe to call on a still-pinned page: the
frame is removed, so a later `UnpinPage` simply no-ops.

### `fetchNode(id) (*Page, *Node, error)`
Convenience wrapper: `FetchPage` then `decodeNode(id, page.Data)`. The returned
`Node` aliases the cached page buffer, so node mutations edit the cache in place.
This is the tree's main entry point into the pool.

---

## LRU & pin semantics

- Front of `lruList` = most recently used; back = least recently used.
- Eviction only ever removes an **unpinned** page (a pinned page is in active use).
- Capacity must comfortably exceed the maximum number of pages pinned at once.
  The deepest pin nesting is in `rebalanceInternal`, which holds a node + parent +
  up to two siblings per level while recursing up the path — keep capacity well
  above `tree height × 4`.

---

## Aliasing gotcha (relevant to the tree)

Code that does `node.data = makeNewLeafHeader()` swaps the slice and breaks the
alias to the cached buffer; it must copy back into `page.Data` before unpinning.
The split paths do exactly this (build a fresh buffer, then `copy(page.Data, …)`),
and the delete-path rebuild helpers (`rebuildLeaf`, `rebuildInternal`,
`compactLeaf`) `copy` into `n.data` in place — so the cache never holds a stale
slice.

---

## Integration

- **Below:** calls `Pager.ReadPage`/`WritePage`/`AllocatePage`/`FreePage`.
- **Above:** the [BTree](btree.md) reads and writes every node through
  `fetchNode`/`FetchPage` + `UnpinPage`, allocates via `AllocatePage`, and frees
  via `FreePage`. Every `FetchPage` is balanced by exactly one `UnpinPage`.
