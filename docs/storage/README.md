# Storage Engine — File Map & Integration

Phase 1 of MySQLight. This package turns a single OS file into a durable,
page-based B+ tree that stores arbitrary `[]byte` key/value pairs and survives
process restarts.

This folder documents each source file in `storage/` individually. For the
original high-level design narrative see [`../storage-engine.md`](../storage-engine.md).

| File | Doc | Responsibility |
|------|-----|----------------|
| `pager.go` | [pager.md](pager.md) | Raw file I/O. Reads/writes fixed 4096-byte pages by ID; owns the DB header. |
| `freelist.go` | [freelist.md](freelist.md) | Tracks freed pages for reuse so the file does not grow forever. |
| `buffer_pool.go` | [buffer-pool.md](buffer-pool.md) | In-memory LRU cache of pages with pin/unpin and lazy write-back. |
| `btree_node.go` | [btree-node.md](btree-node.md) | The byte layout of a single page as a leaf or internal node, plus read/write accessors. |
| `btree.go` | [btree.md](btree.md) | The B+ tree itself: insert, split, search, (planned) get/scan/delete. |

`storage_test.go` is intentionally not documented here.

---

## Layering

```
        ┌─────────────┐
        │    BTree     │   btree.go  — tree logic, splits, search
        └──────┬──────┘
               │  (intended)        (intended) ┌────────────┐
        ┌──────▼──────┐  ───────────────────── │  Freelist  │  freelist.go
        │ BufferPool  │  buffer_pool.go         └─────┬──────┘
        └──────┬──────┘                               │
               │                                      │
        ┌──────▼──────────────────────────────────────▼──┐
        │                    Pager                        │  pager.go
        │        (owns the *os.File, header page 0)       │
        └─────────────────────────────────────────────────┘
                              │
                        database file on disk
```

A page is the unit of everything: 4096 bytes (`PageSize`), matching the OS page
size. Reads and writes are always whole pages. Page 0 is the database header.

Each node of the B+ tree occupies exactly one page. Keys and values are opaque
`[]byte` — the tree never interprets them, it only compares them with
`bytes.Compare`. Type-aware comparison is the executor's job in a later phase.

---

## Intended data flow (a single `Insert`)

1. `BTree.findLeaf` walks the tree from the root, reading each node through the
   pager (intended: through the buffer pool) and following child pointers until
   it reaches a leaf. It records the path of internal pages visited.
2. If the target leaf has room, the new entry is written into it and the page is
   written back.
3. If the leaf is full, `splitLeaf` divides it into two pages and `pushUp`
   inserts a separator key into the parent. Splits cascade upward; if the root
   splits, a new root is allocated and the tree grows one level taller.

---

## Current status (as of this writing)

| Area | State |
|------|-------|
| Pager: Open / ReadPage / WritePage / AllocatePage / PageCount / Close | Working |
| Pager: FreePage | Stub (delegates to Freelist, not yet built) |
| Freelist | Stub — struct and methods are TODO |
| BufferPool: FetchPage / UnpinPage / FlushAll | Implemented; see doc for open issues |
| BufferPool wired into BTree | **No** — the tree currently calls the pager directly |
| Node layout + accessors (`btree_node.go`) | Working, including `insertInternalEntry` |
| `makeNewInternalHeader` helper | **Missing** — needed by `splitInternal` |
| BTree.Insert + splitLeaf + pushUp (leaf path) | Working |
| BTree.splitInternal | **In progress / not compiling** |
| BTree.Get / Delete / Scan | Stubs |

See each file's doc for the per-method detail and the specific open items.

---

## Key design rules

- **Keys are `[]byte`, compared with `bytes.Compare`.** The tree is type-agnostic.
- **One node == one page (4096 B).** Never split a node across pages.
- **Leaf splits *copy* the median up; internal splits *move* it up.** This is the
  single most important invariant when reading `splitLeaf` vs `splitInternal`.
- **Every `FetchPage` must be balanced by exactly one `UnpinPage`** once the tree
  is wired through the buffer pool.
- **Pages are never shrunk.** Freed pages go to the freelist and are reused.
