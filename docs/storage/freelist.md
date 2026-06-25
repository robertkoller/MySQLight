# `freelist.go` — Free Page Tracking

**Status: stub.** The struct and all three methods currently `panic("not implemented")`.
This doc describes how it is meant to work and how it slots into the engine.

When a page is deleted (e.g. a B+ tree node emptied by `Delete`), the file is not
shrunk — truncating a file is expensive and fragments offsets. Instead the freed
page's ID is recorded so a later allocation can hand it back out. The freelist is
that record, and it is persisted so freed pages survive a restart.

---

## Intended type

```go
type Freelist struct {
    pager        *Pager     // to extend the file when the free list is empty
    freePages    []uint32   // in-memory list of reusable page IDs
    headerPageID uint32     // the page where the free list is serialised
}
```

---

## Intended functions

### `NewFreelist(pager *Pager, headerPageID uint32) (*Freelist, error)`
- Read the freelist header page (its location comes from the DB header, page 0).
- If it already holds a serialised list of free page IDs, decode them into
  `freePages` so they can be reused immediately.
- Return the initialised freelist.

### `Allocate() (uint32, error)`
- If `freePages` is non-empty: pop the last entry and return it (no file I/O).
- Otherwise: delegate to `pager.AllocatePage()` to extend the file.

This makes the freelist the preferred allocation path, with the pager as the
fallback for genuine growth.

### `Free(pageID uint32) error`
- Append `pageID` to `freePages`.
- Persist the updated list to the freelist header page so it survives a restart.

---

## Persistence sketch

A simple on-disk format for the freelist page:

```
[ count: 4 B uint32 ][ pageID 0: 4 B ][ pageID 1: 4 B ] ...
```

If the number of free pages can exceed what fits in one page, the freelist pages
chain via a "next freelist page" pointer (a stretch concern; a single page holds
~1023 IDs, which is plenty for early development).

---

## Integration

- **Pager:** `Pager.FreePage` (also a stub) should delegate to `Freelist.Free`,
  and the engine's allocation path should call `Freelist.Allocate` instead of
  `Pager.AllocatePage` directly once this exists.
- **BTree:** `Delete` (not yet built) is the producer of freed pages — when a node
  is merged away it should be handed to the freelist.
- **Header:** page 0 stores where the freelist header page lives so it can be
  found on startup.
