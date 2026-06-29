# Free Page Tracking (Design B — intrusive linked list)

When a B+ tree node is emptied by `Delete` (merged away, or an old root after a
collapse), the file is **not** shrunk — truncating fragments offsets and is
expensive. Instead the freed page's ID is recorded so a later `AllocatePage` can
hand it back out. That record is the **freelist**, and it is persisted so freed
pages survive a restart.

There is **no `freelist.go`** and no separate `Freelist` type. The freelist is a
singly-linked stack threaded *through the free pages themselves*, implemented
entirely in [`pager.go`](pager.md) plus one method in
[`buffer_pool.go`](buffer-pool.md).

---

## The idea

- The DB header (page 0) stores a single `freeListHead` page ID at **offset 27**.
  `0` means the list is empty (page 0 is the header, never freeable, so 0 is a
  safe sentinel).
- Each free page stores, in its **first 4 bytes**, the ID of the *next* free page.
- So the list is: `header.freeListHead → page → page → … → 0`, with every "next"
  pointer living inside the free page it points from. No separate storage, no
  capacity cap, all operations O(1).

```
header.freeListHead = 7
   page 7:  [next = 4] …garbage…
   page 4:  [next = 9] …
   page 9:  [next = 0] …      (0 = end of chain)
```

`freeListHead` and the in-page "next" are **different pointers**: the head names
the first free page; that page's first 4 bytes name the second, etc.

---

## Free = push onto the stack (`Pager.FreePage`)

```
write current freeListHead into pageID's first 4 bytes   // new node's "next" = old head
freeListHead = pageID                                     // head = new node
persist freeListHead to the header (offset 27)
```

The freed page becomes the new head; it links to whatever used to be the head.

## Allocate = pop the stack (`Pager.AllocatePage`)

```
if freeListHead != 0:
    id   = freeListHead
    next = id's first 4 bytes        // read the link we stored on free
    freeListHead = next              // advance head past the page we're handing out
    persist freeListHead
    return id
else:
    extend the file (bump pageCount, persist it) and return the new last page
```

Reusing the head requires *reading the page* — the "what's next" info lives only
inside it. The freelist is the preferred path; growing the file is the fallback.

Trace: free A (head→A, A→0), free B (head→B, B→A), allocate → B (head→A),
allocate → A (head→0). A LIFO stack.

---

## The buffer-pool gotcha (`BufferPool.FreePage`)

`Pager.FreePage` writes the link straight to disk, but the page being freed may
still be cached in the buffer pool — possibly **dirty**. If it isn't dropped, a
later `FlushAll`/eviction would write its stale bytes over the link and corrupt
the chain. So callers free **through the pool**, which discards the cached frame
(no flush) and then calls the pager:

```go
func (bp *BufferPool) FreePage(pageID uint32) error {
    if elem, ok := bp.lruMap[pageID]; ok {
        bp.lruList.Remove(elem)
        delete(bp.lruMap, pageID)
        delete(bp.frames, pageID)   // discard — do NOT flush, the page is dead
    }
    return bp.pager.FreePage(pageID)
}
```

The tree always calls `t.pool.FreePage(...)`, never `pager.FreePage` directly.

---

## How the tree uses it

`rebalanceLeaf` and `rebalanceInternal` ([`btree.go`](btree.md)) free pages at
three spots, each at a **tail position** (the page is never read again in that
frame):

- a leaf merged into its left sibling → free the now-empty leaf,
- a right sibling merged into the survivor → free the right sibling,
- a root that collapsed to a single child → free the old root.

A freed page is often still **pinned** at that moment (e.g. a dead leaf is owned
by `Delete`, unpinned after the call returns). Because `pool.FreePage` removes it
from `frames`, the later `UnpinPage` simply doesn't find it and no-ops — the
discarded dirty bytes are never written. Reuse is clean too: a recycled ID is no
longer in `frames`, so the next `FetchPage` misses, reads the link page from
disk, and the caller overwrites it with a fresh node header.

---

## Persistence

`freeListHead` is mirrored to the header (offset 27) on every change and read
back in `Pager.Open`, so the whole chain survives a restart — the links live in
the pages on disk, and the head lives in the header.
