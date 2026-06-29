# MySQLight Storage Engine

Phase 1 of MySQLight. Turns a single OS file into a durable, page-based B+ tree
that stores arbitrary `[]byte` key/value pairs and survives process restarts.

This is the overview. Each source file in `storage/` also has its own detailed doc
(linked in the file map below).

---

## Layering

```
        ┌─────────────┐
        │    BTree     │   btree.go  — tree logic: split, get, scan, delete/rebalance
        └──────┬──────┘
               │
        ┌──────▼──────┐
        │ BufferPool  │   buffer_pool.go  — LRU cache, pin/unpin, allocate/free, flush
        └──────┬──────┘
               │
        ┌──────▼──────────────────────────────────────────┐
        │                    Pager                          │  pager.go
        │   owns *os.File + header (page 0) + the freelist  │
        └───────────────────────────────────────────────────┘
                              │
                        database file on disk
```

The B+ tree is the core data structure — every table and every index is its own
B+ tree. The buffer pool sits between the tree and the pager so traversals don't
hit disk on every access. The pager owns the file. A **page** is the unit of
everything: 4096 bytes (`PageSize`), matching the OS page size; reads and writes
are always whole pages, and page 0 is the database header.

Keys and values are opaque `[]byte` — the tree never interprets them, only compares
them with `bytes.Compare`. Type-aware comparison is the executor's job in a later
phase.

| File | Doc | Responsibility |
|------|-----|----------------|
| `pager.go` | [pager.md](pager.md) | Raw file I/O by page ID; owns the DB header; **and** the freelist. |
| `buffer_pool.go` | [buffer-pool.md](buffer-pool.md) | In-memory LRU page cache with pin/unpin, lazy write-back, allocate/free wrappers. |
| `btree_node.go` | [btree-node.md](btree-node.md) | Byte layout of a page as leaf or internal node, plus read/write/rebuild accessors. |
| `btree.go` | [btree.md](btree.md) | The B+ tree: insert, split, get, scan, delete (borrow/merge/rotate), iterator. |
| *(no `freelist.go`)* | [freelist.md](freelist.md) | The freelist is an intrusive linked list living in the pager. |

`storage_test.go` covers every public method (pager, pool, insert/get, durability,
delete, scan, freelist).

---

## Pager (`pager.go`)

Treats the file as an array of fixed-size pages and reads/writes raw byte slices by
page ID. It knows nothing about what's inside a page — only page 0, the header, has
meaning to it.

```
file on disk:
[ page 0 ][ page 1 ][ page 2 ][ page 3 ] ...
  header    node      node      node
```

**Page 0 — database header:**

| Offset | Size | Field |
|--------|------|-------|
| 0–8 | 9 B | Magic bytes (`MYSQLIGHT`) |
| 9–10 | 2 B | Page size (always 4096) |
| 11–14 | 4 B | Page count |
| 15–18 | 4 B | Catalog root page ID |
| 19–26 | 8 B | Last WAL LSN |
| 27–30 | 4 B | Freelist head page ID (0 = empty) |

```go
Open(path string) (*Pager, error)
ReadPage(pageID uint32) ([]byte, error)
WritePage(pageID uint32, data []byte) error
AllocatePage() (uint32, error)   // reuse a freed page, else extend the file
FreePage(pageID uint32) error    // push onto the freelist
PageCount() uint32
Close() error                    // sync + close
```

`pageCount` (offset 11) and `freeListHead` (offset 27) are mirrored to the header
whenever they change. **Pages are never shrunk** — freed pages go on the freelist
and are reused by future `AllocatePage` calls.

---

## Freelist (in the pager — Design B)

There is no separate freelist module. The freelist is a singly-linked **stack
threaded through the free pages themselves**:

- `freeListHead` in the header names the first free page (0 = empty).
- Each free page stores, in its first 4 bytes, the id of the *next* free page.

`FreePage` is a push (write the old head into the page, set head = page);
`AllocatePage` is a pop (read the page's first 4 bytes to find the new head). O(1),
no separate storage, no capacity cap, survives restart. The buffer pool's
`FreePage` first **drops the freed page from the cache without flushing** — else a
later flush would write stale bytes over the link and corrupt the chain. Full
detail in [freelist.md](freelist.md).

---

## Buffer Pool (`buffer_pool.go`)

An LRU cache that keeps recently used pages in memory and writes them back lazily.
The tree talks **only** to the pool, never the pager directly.

- Fixed capacity. `FetchPage` returns a cached page or loads it from the pager.
- When full, the least-recently-used **unpinned** page is evicted (flushed first if
  dirty). A pinned page can never be evicted.
- Callers pin via `FetchPage` and release via `UnpinPage(id, dirty)`. `dirty == true`
  means "I modified this — flush before evicting." The dirty flag is sticky.

```go
FetchPage(pageID uint32) (*Page, error)
UnpinPage(pageID uint32, dirty bool) error
FlushAll() error
AllocatePage() (uint32, error)   // wraps pager (freelist-aware)
FreePage(pageID uint32) error    // drop from cache (no flush) + pager.FreePage
fetchNode(id uint32) (*Page, *Node, error)
```

`Page.Data` is shared and mutated in place; capacity must comfortably exceed the
max pages pinned at once (deepest is `rebalanceInternal`, ~4 per level up the path).

---

## B+ Tree Node Layout (`btree_node.go`)

Every node is exactly one page. Two node types: **leaf** and **internal**.

### Leaf node — stores key/value pairs; linked in sorted order via right-sibling

```
[0]      nodeType       1 B   (0x01 = leaf)
[1–2]    keyCount       2 B   uint16
[3–4]    freeSpacePtr   2 B   uint16 — starts at 4096, decrements as data is written
[5–8]    rightSibling   4 B   uint32 page ID (0 = no sibling)
[9–...]  slot array     keyCount × 8 bytes
[  free gap  ]
[ key/value data grows ← from end of page ]
```

**Slot entry (8 bytes), in sorted key order:** `keyOffset, keyLen, valueOffset,
valueLen` (each uint16). Slot array grows up from offset 9; data grows down from
4096; they meet when full.

### Internal node — stores separator keys + child pointers; no values

```
[0]      nodeType       1 B   (0x02 = internal)
[1–2]    keyCount       2 B   uint16
[3–4]    freeSpacePtr   2 B   uint16
[5–...]  child IDs      (keyCount+1) × 4 bytes
[...]    key slot array keyCount × 4 bytes (keyOffset, keyLen uint16)
[ key bytes grow ← from end of page ]
```

For `n` keys there are `n+1` children; child `i` holds keys `x` with
`key[i-1] ≤ x < key[i]`. The key slot base `5 + (keyCount+1)*4` is **dynamic** —
adding a child shifts it, so accessors recompute it from the live `keyCount`.

Because `deleteLeafEntry` and in-place key replacement don't reclaim bytes (the
free-space pointer only moves down), the delete path **rebuilds/compacts** pages
(`rebuildLeaf`, `compactLeaf`, `rebuildInternal`) so physical usage tracks live
bytes. Overflow math and per-accessor detail are in [btree-node.md](btree-node.md).

---

## B+ Tree (`btree.go`)

```go
type BTree struct {
    pool       *BufferPool
    rootPageID uint32   // changes when the root splits or collapses
}
```

Pin discipline underpins everything: every `fetchNode`/`FetchPage` is balanced by
exactly one `UnpinPage`.

### Insert
`findLeaf(key)` → leaf + path. If the leaf would overflow, `splitLeaf` then re-find
the target; otherwise `insertLeafEntry` and unpin dirty. A split **copies** the
median up to the parent via `pushUp`; if the parent is full it `splitInternal`s
(which **moves** its median up) and recurses; a root split allocates a new root.

### Get
`findLeaf` → binary-search the leaf → return a copy of the value or `ErrNotFound`.

### Delete
Remove the leaf entry. If the leaf is the root or still ≥ half full, done.
Otherwise `rebalanceLeaf` — **borrow** from a sibling that can spare an entry,
else **merge** if the two fit in one page (freeing the emptied page), else a
**forced borrow** from the necessarily-large sibling. A merge removes a separator
from the parent, which may underflow → `rebalanceInternal` (rotation through the
parent, or merge that pulls the separator down), recursing upward; a root that
collapses to one child drops a level (old root freed).

### Scan
`Scan(start, end)` positions a cursor on the start leaf (leftmost if `start == nil`)
and returns a `*leafIterator`. `Next()` walks the right-sibling chain yielding
copied key/value pairs in order until it passes `end` (exclusive) or runs out of
leaves (`io.EOF`); the iterator holds exactly one leaf pinned and `Close()` (idempotent)
releases it.

---

## Phase 1 Status — complete

| Component | Status |
|-----------|--------|
| Pager (Open, ReadPage, WritePage, AllocatePage, FreePage, PageCount, Close) | Done |
| Freelist (intrusive linked list in the pager; pool drops the freed frame) | Done |
| Buffer Pool (FetchPage, UnpinPage, FlushAll, AllocatePage, FreePage, fetchNode) | Done |
| Buffer pool wired into the tree (no direct pager access from BTree) | Done |
| Node layout + accessors; rebuild/compaction helpers | Done |
| BTree.Insert + splitLeaf + splitInternal + pushUp | Done |
| BTree.Get | Done |
| BTree.Delete (borrow, merge, internal rotation, recursive merge, root collapse) | Done |
| BTree.Scan + iterator | Done |
| Tests (pager, buffer pool, insert/get, durability, delete, scan, freelist) | Done |

### Not in Phase 1 (later work)
- Persisting a tree's root page id so it can be reopened without the caller
  remembering it — the **catalog's** job in Phase 3.
- Crash recovery / mid-operation durability — **WAL**, Phase 4.
- Minor polish: pager I/O is `Seek`-based (could be `ReadAt`/`WriteAt`); the Insert
  split decision uses the on-page free-space pointer rather than live bytes, so a
  heavily delete-then-insert leaf may split slightly early (correct, not optimal).

---

## Key design rules

- **Keys are `[]byte`, compared with `bytes.Compare`.** The tree is type-agnostic.
- **One node == one page (4096 B).** Never split a node across pages.
- **Leaf splits *copy* the median up; internal splits *move* it up.** Conversely,
  leaf merges drop the separator; internal merges *pull it down* into the survivor.
- **Every `FetchPage` is balanced by exactly one `UnpinPage`.**
- **Pages are never shrunk.** Freed pages go on the freelist and are reused.
- **Deletes don't reclaim bytes in place** — the delete path rebuilds/compacts so
  physical usage tracks live bytes.
