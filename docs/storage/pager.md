# `pager.go` — Raw Page I/O

The pager is the lowest layer of the storage engine. It treats the database file
as a flat array of fixed-size **pages** (4096 bytes) and exposes read/write/
allocate operations addressed by page ID. It knows nothing about B+ trees or
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

All multi-byte integers are **big-endian**, consistently across the whole engine.

---

## Type

```go
type Pager struct {
    pages     *os.File   // the open database file
    pageCount uint32      // number of pages currently in the file (incl. header)
}
```

`pageCount` is the in-memory authority for how many pages exist; it is mirrored
into the header at offset 11 whenever it changes.

---

## Functions

### `createHeader(pageCount, catalogID, walLSN) []byte`
Builds a fresh 4096-byte header page: writes the magic string, page size, page
count, catalog root ID, and WAL LSN at their offsets. Returns the full page.

### `Open(path string) (*Pager, error)`
Opens the file (creating it if absent, mode `0644`).
- **New file (size 0):** writes a fresh header with `pageCount = 1` (the header
  page itself) and returns a `Pager` with `pageCount = 1`.
- **Existing file:** reads page 0, verifies the magic bytes (error if mismatched),
  and loads `pageCount` from the header.

### `ReadPage(pageID uint32) ([]byte, error)`
Bounds-checks `pageID < pageCount`, seeks to `pageID * PageSize`, and reads
exactly `PageSize` bytes into a **freshly allocated** slice. Each call allocates
a new buffer — this is why the buffer pool exists (to avoid re-reading and
re-allocating hot pages).

> **Open issue:** the seek offset is computed as `int64(pageID*PageSize)`, which
> multiplies in `uint32` and overflows past ~4 GB. `WritePage` does it correctly
> as `int64(pageID)*PageSize`. `ReadPage` should match.

### `WritePage(pageID uint32, data []byte) error`
Bounds-checks `pageID`, rejects `data` whose length isn't exactly `PageSize`
(a partial write would corrupt the fixed-size layout), seeks to the page offset
and writes. The caller is responsible for forming a complete page first.

### `AllocatePage() (uint32, error)`
Reserves the next page at the end of the file: returns the current `pageCount`
as the new ID, increments `pageCount`, and persists the new count to the header
(offset 11).

- It does **not** write any content to the new page — that's the caller's job.
- It does **not** physically extend the file; the file only grows when the new
  page is actually written. Therefore a caller must `WritePage` a freshly
  allocated page before any `ReadPage`, or the read will hit EOF.

### `FreePage(pageID uint32) error` — stub
Intended to hand the page to the [Freelist](freelist.md) for reuse rather than
shrinking the file. Not yet implemented.

### `PageCount() uint32`
Returns the current page count (including page 0).

### `Close() error`
`Sync()` then `Close()` the file, so OS-buffered writes are durably flushed
before the handle is released.

---

## Integration

- **Above:** the [BufferPool](buffer-pool.md) is the intended sole caller of
  `ReadPage`/`WritePage`. (Today the [BTree](btree.md) also calls the pager
  directly.)
- **Allocation:** `AllocatePage` will become the fallback path for the
  [Freelist](freelist.md) — the freelist hands out recycled IDs first and only
  calls `AllocatePage` when it's empty.
- The pager is the only component that touches the `*os.File`.
