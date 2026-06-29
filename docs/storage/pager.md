# `pager.go` — Raw Page I/O

The pager is the lowest layer of the storage engine. It treats the database file
as a flat array of fixed-size **pages** (4096 bytes) and exposes read/write/
allocate/free operations addressed by page ID. It knows nothing about B+ trees or
what lives inside a page — only page 0, the header, has meaning to it.

```
file on disk:
[ page 0 ][ page 1 ][ page 2 ][ page 3 ] ...
  header    node      node      node
```

---

## Constants

| Name | Value | Meaning |
|------|-------|---------|
| `PageSize` | `4096` | Bytes per page. Matches OS virtual-memory pages. |
| `magicByte` | `"MYSQLIGHT"` | 9-byte signature at the start of page 0; identifies a valid DB file. |

---

## Header layout (page 0)

| Offset | Size | Field |
|--------|------|-------|
| 0–8 | 9 B | Magic bytes (`MYSQLIGHT`) |
| 9–10 | 2 B | Page size (uint16) |
| 11–14 | 4 B | Page count (uint32) |
| 15–18 | 4 B | Catalog root page ID (uint32) |
| 19–26 | 8 B | Last checkpointed WAL LSN (uint64) |
| 27–30 | 4 B | **Freelist head** page ID (uint32, 0 = empty) |

All multi-byte integers are **big-endian**, consistently across the whole engine.

---

## Type

```go
type Pager struct {
    pages        *os.File   // the open database file
    freeListHead uint32     // head of the free-page chain (0 = none)
    pageCount    uint32     // number of pages currently in the file (incl. header)
}
```

`pageCount` is the in-memory authority for how many pages exist; it is mirrored
into the header at offset 11 whenever it changes. `freeListHead` is mirrored at
offset 27.

---

## Functions

### `createHeader(pageCount, catalogID, walLSN, freeListHead) []byte`
Builds a fresh 4096-byte header page with all five fields written at their
offsets. Returns the full page.

### `Open(path string) (*Pager, error)`
Opens the file (creating it if absent, mode `0644`).
- **New file (size 0):** writes a fresh header with `pageCount = 1` (the header
  page itself) and `freeListHead = 0`.
- **Existing file:** reads page 0, verifies the magic bytes (error if mismatched),
  and loads `pageCount` and `freeListHead` from the header.

### `ReadPage(pageID uint32) ([]byte, error)`
Bounds-checks `pageID < pageCount`, seeks to `int64(pageID) * PageSize`, and reads
exactly `PageSize` bytes into a **freshly allocated** slice. Each call allocates a
new buffer — this is why the buffer pool exists (to avoid re-reading and
re-allocating hot pages).

### `WritePage(pageID uint32, data []byte) error`
Bounds-checks `pageID`, rejects `data` whose length isn't exactly `PageSize`
(a partial write would corrupt the fixed-size layout), seeks to the page offset
and writes. The caller is responsible for forming a complete page first.

### `AllocatePage() (uint32, error)`
Hands out a page, preferring reuse over growth:
- **Freelist non-empty (`freeListHead != 0`):** pop the head — read the freed
  page's first 4 bytes to find the next free page, set `freeListHead` to it,
  persist, and return the popped ID.
- **Freelist empty:** extend the file — return the current `pageCount` as the new
  ID, write a zeroed page there (so it's immediately readable), increment
  `pageCount`, and persist the count.

Does not write node content — that's the caller's job (it overwrites the page with
a fresh header).

### `FreePage(pageID uint32) error`
Pushes the page onto the freelist stack: writes the current `freeListHead` into
the freed page's first 4 bytes (its "next" link), sets `freeListHead = pageID`,
and persists the head. See [freelist.md](freelist.md) for the full mechanism and
the buffer-pool drop that must accompany it.

### `writeFreeListHead() error`
Persists `freeListHead` to header offset 27. Called by `AllocatePage`/`FreePage`.

### `PageCount() uint32`
Returns the current page count (including page 0).

### `Close() error`
`Sync()` then `Close()` the file, so OS-buffered writes are durably flushed before
the handle is released.

---

## Integration

- **Above:** the [BufferPool](buffer-pool.md) is the caller of `ReadPage`/
  `WritePage`. The [BTree](btree.md) goes through the pool, never the pager
  directly.
- **Allocation/free:** `AllocatePage` and `FreePage` *are* the freelist — there is
  no separate freelist module. The pool's `AllocatePage`/`FreePage` wrap these
  (the latter also dropping the cached frame).
- The pager is the only component that touches the `*os.File`.

> Minor note: I/O uses `Seek` + `Read`/`Write` on a shared file offset, which is
> fine single-threaded; `ReadAt`/`WriteAt` would be cleaner and concurrency-safe
> if that ever matters.
