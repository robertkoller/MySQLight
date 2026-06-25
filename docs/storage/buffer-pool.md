# `buffer_pool.go` — LRU Page Cache

Reading from disk on every tree traversal is too slow, and `Pager.ReadPage`
allocates a new buffer each call. The buffer pool keeps recently used pages in
memory, hands out shared references to them, and writes them back to disk lazily.

It sits between the [BTree](btree.md) and the [Pager](pager.md) in the intended
design. (Today the tree bypasses it — see *Integration*.)

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

`Data` is **shared**: when a caller mutates it, it mutates the cached copy
directly. That is the whole point — but it means callers must signal "I changed
this" via `UnpinPage(id, true)` so the page is eventually flushed.

---

## Functions

### `NewBufferPool(pager *Pager, capacity int) *BufferPool`
Allocates the maps and the LRU list and stores the pager and capacity.

### `FetchPage(pageID uint32) (*Page, error)`
Returns the requested page, pinned (`pinCount` incremented).
- **Cache hit:** move it to the front of the LRU list (most recently used),
  increment the pin count, return it.
- **Cache full** (`len >= capacity`): walk the LRU list from the back looking for
  an **unpinned** page. If every page is pinned, return an error. Otherwise, if
  the victim is dirty, flush it via `pager.WritePage`, then remove it from the
  list and both maps.
- **Cache miss:** load the page via `pager.ReadPage`, wrap it in a `Page` with
  `pinCount = 1`, insert at the front of the LRU list and into the maps.

### `UnpinPage(pageID uint32, dirty bool) error`
Signals the caller is done with a page.
- Errors if the page isn't in the pool.
- Decrements `pinCount` (guarded so it never goes below 0).
- If `dirty` is true, marks the page dirty. The dirty flag is sticky — it is only
  cleared by a flush, never by `dirty == false`, so a page stays dirty until
  written.
- When `pinCount` reaches 0, the page is moved to the back of the LRU list,
  making it the next eviction candidate.

### `FlushAll() error`
Writes every dirty page to disk via the pager and marks it clean. Used at
checkpoint and on close so no modified data is left only in memory.

---

## LRU & pin semantics

- Front of `lruList` = most recently used; back = least recently used.
- Eviction only ever removes an **unpinned** page (a pinned page is in active use
  and must not disappear from under its holder).
- A page becomes an eviction candidate the moment its pin count hits 0
  (`UnpinPage` moves it to the back).

---

## Open issues (as of this writing)

- **`FlushAll` pin detection is buggy.** It sets `hasPins = page.pinCount > 0`
  inside the loop, overwriting it each iteration, so the final value reflects only
  the last dirty page (and map iteration order is random). It should OR-accumulate:
  `hasPins = hasPins || page.pinCount > 0`. Separately, decide whether flushing a
  still-pinned page is an error at all, since the data is written regardless.
- **No `NewPage` path.** Once the tree uses the pool, allocating a brand-new page
  can't go through `FetchPage` (that calls `ReadPage` on a page the file doesn't
  hold yet → EOF). The pool will need a method that allocates via the pager and
  installs a fresh, pinned, dirty frame without reading disk.

---

## Integration

- **Below:** calls `Pager.ReadPage`/`WritePage`.
- **Above:** intended to be the only thing the [BTree](btree.md) talks to for
  reading/writing nodes. The swap from "pager-direct" to "pool" changes the
  tree's contract:
  - `ReadPage` (copy) → `FetchPage` (shared, pinned).
  - explicit `WritePage` → mutate in place + `UnpinPage(id, true)`.
  - **Gotcha:** code that does `node.data = makeNewLeafHeader()` (reassigning the
    slice) breaks the alias to the cached buffer. Under the pool, such code must
    rewrite the existing buffer in place instead of swapping the slice, or the
    cache will flush stale bytes.
