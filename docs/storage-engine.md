# MySQLight Storage Engine

Phase 1 of MySQLight. Responsible for reading and writing data durably to disk via a B+ tree backed by a page-based file format.

---

## Overview

```
BTree
  └── BufferPool   (page cache, LRU eviction)
        └── Pager  (raw file I/O)
```

The B+ tree is the core data structure. Every table and every index is its own B+ tree. The buffer pool sits between the tree and the pager to avoid hitting disk on every access. The pager owns the file.

---

## Pager (`pager.go`)

The pager treats the database file as an array of fixed-size **pages** (4096 bytes each). It knows nothing about what is inside a page — it just reads and writes raw byte slices by page ID.

```
file on disk:
[ page 0 ][ page 1 ][ page 2 ][ page 3 ] ...
  header    btree     btree     freelist
```

**Page 0 — database header:**

| Offset | Size | Field |
|--------|------|-------|
| 0–8 | 9 B | Magic bytes (`MYSQLIGHT`) |
| 9–10 | 2 B | Page size (always 4096) |
| 11–14 | 4 B | Page count |
| 15–18 | 4 B | Catalog root page ID |
| 19–26 | 8 B | Last WAL LSN |

**API:**

```go
Open(path string) (*Pager, error)         // open or create the db file
ReadPage(pageID uint32) ([]byte, error)   // read 4096 bytes from disk
WritePage(pageID uint32, data []byte) error
AllocatePage() (uint32, error)            // extend file by one page
FreePage(pageID uint32) error             // hand page to freelist
PageCount() uint32
Close() error                             // sync + close file
```

`AllocatePage` increments the page count and persists it to the header immediately. Pages are never shrunk — freed pages go to the freelist and are reused by future `AllocatePage` calls.

---

## Buffer Pool (`buffer_pool.go`)

Reading from disk on every B+ tree traversal would be too slow. The buffer pool keeps recently used pages in memory using an LRU cache.

**How it works:**
- Fixed capacity (e.g. 64 pages)
- `FetchPage` returns a page from memory if cached, otherwise loads it from the pager
- When the cache is full, the least-recently-used **unpinned** page is evicted. If that page is dirty, it is flushed to disk first
- Callers increment the pin count by fetching a page and decrement it by calling `UnpinPage`. A pinned page can never be evicted

```go
FetchPage(pageID uint32) (*Page, error)
UnpinPage(pageID uint32, dirty bool) error
FlushAll() error
```

Marking a page dirty via `UnpinPage(id, true)` means "I modified this page — write it to disk before evicting it."

---

## B+ Tree Node Layout (`btree_node.go`)

Every B+ tree node is exactly one page (4096 bytes). There are two node types: **leaf** and **internal**.

### Leaf Node

Stores actual key-value pairs. Leaf nodes are linked in sorted order via right-sibling pointers so range scans can walk them without backtracking.

```
[0]      nodeType       1 B   (0x01 = leaf)
[1–2]    keyCount       2 B   uint16
[3–4]    freeSpacePtr   2 B   uint16 — starts at 4096, decrements as data is written
[5–8]    rightSibling   4 B   uint32 page ID (0 = no sibling)
[9–...]  slot array     keyCount × 8 bytes
[  free space  ]
[key/value data grows ← from end of page]
```

**Slot entry (8 bytes):**

| Offset within slot | Field |
|--------------------|-------|
| 0–1 | keyOffset (uint16) — position of key bytes in page |
| 2–3 | keyLen (uint16) |
| 4–5 | valueOffset (uint16) — position of value bytes in page |
| 6–7 | valueLen (uint16) |

Slot entries are kept in sorted key order. Key and value bytes are packed from the right side of the page inward — `freeSpacePtr` tracks where the next write goes.

**Inserting a key-value pair into a leaf:**
1. Decrement `freeSpacePtr` by `len(value)`, write value bytes there → `valueOffset`
2. Decrement `freeSpacePtr` by `len(key)`, write key bytes there → `keyOffset`
3. Find the sorted insertion index in the slot array
4. Shift existing slots right by 8 bytes to make room
5. Write the new 8-byte slot at the insertion index
6. Increment `keyCount`, write updated `freeSpacePtr`

**Overflow check before inserting:**
```
needed   = len(key) + len(value) + 8
available = freeSpacePtr - (9 + (keyCount+1)*8)
```
If `needed > available`, the page is full and must be split.

### Internal Node

Stores separator keys and child page IDs. No values — only used for routing.

```
[0]      nodeType       1 B   (0x02 = internal)
[1–2]    keyCount       2 B   uint16
[3–4]    freeSpacePtr   2 B   uint16
[5–...]  child IDs      (keyCount+1) × 4 bytes
[...]    key slot array keyCount × 4 bytes (keyOffset uint16, keyLen uint16)
[key bytes grow ← from end of page]
```

For `n` keys there are always `n+1` child page IDs. A child at position `i` holds all keys satisfying `key[i-1] ≤ x < key[i]`.

**Important — key slot base is dynamic:** The key slot array starts at `5 + (keyCount+1)*4`, which shifts right by 4 bytes every time a child pointer is added. `internalKey(i)` recomputes this base from the live `keyCount` on every call. As a consequence, `insertInternalEntry` must physically shift all existing key slots right by 4 bytes before writing the new child pointer, so that subsequent reads via `internalKey` land in the correct positions.

**Inserting a key into an internal node** (`insertInternalEntry(key, slotIndex, rightChildID)`):
1. Pack key bytes at `freeSpacePtr - len(key)`, update `freeSpacePtr`
2. Shift key slots `[slotIndex .. keyCount-1]` right by 8 bytes (4 for new child pointer + 4 for new key slot gap)
3. Shift key slots `[0 .. slotIndex-1]` right by 4 bytes (4 for new child pointer only)
4. Shift child pointers `[slotIndex+1 .. keyCount]` right by 4 bytes to open a gap
5. Write `rightChildID` at child position `slotIndex+1`
6. Write the new 4-byte key slot `(keyOffset, keyLen)` at `newSlotsBase + slotIndex*4`, where `newSlotsBase = 5 + (keyCount+2)*4`
7. Increment `keyCount`

**Overflow check before inserting into an internal node:**
```
needed    = len(key) + 4 + 4          (key bytes + key slot + child pointer)
available = freeSpacePtr - (5 + (keyCount+2)*4 + (keyCount+1)*4)
```
If `needed > available`, the node is full and must be split before inserting.

---

## B+ Tree (`btree.go`)

```go
type BTree struct {
    pager      *Pager
    rootPageID uint32
}
```

### Insert

```
findLeaf(key) → leafPageID, path
if leaf full → splitLeaf → pushUp
insertLeafEntry(key, value, slotIdx) into leaf
WritePage(leafPageID)
```

**`findLeaf`** walks from the root down to the correct leaf. At each internal node it scans keys left to right and follows the first child whose separator key is greater than the search key (or the last child if none is). It accumulates the path of internal node page IDs visited — these are needed by `pushUp` to propagate splits upward.

**`splitLeaf`** is called when a leaf is full:
1. Extract all key-value pairs into memory
2. Allocate a new right page
3. Re-initialize both pages as empty leaves
4. Re-insert left half into original page, right half into new page
5. Set original leaf's `rightSibling` to new page ID
6. Call `pushUp(keys[midpoint], rightPageID, path)` to insert the separator into the parent

**`pushUp`** inserts a separator key and new right child into the parent:
- If `path` is empty, the root was split — allocate a new root internal node with two children and update `t.rootPageID`
- Otherwise, insert into `path[last]`. If that parent is also full, call `splitInternal` and recurse up with `path[:last]`

### Get

Walk internal nodes using `internalKey(i)` comparisons down to the leaf. Binary search the leaf's slot array for the key. Return the value or `ErrNotFound`.

### Delete

Find the leaf containing the key, remove its slot entry. If the leaf falls below half capacity (underflow), try to borrow an entry from an adjacent sibling. If the sibling is also too small to lend, merge the two leaves and remove the separator key from the parent, which may trigger further merges upward.

### Scan

Find the leaf containing `start` (or the leftmost leaf if `start` is nil) via `findLeaf`. Return an iterator that walks right-sibling pointers, yielding key-value pairs in order until it passes `end` or runs out of leaves.

---

## What's Left in Phase 1

| Component | Status |
|-----------|--------|
| Pager (ReadPage, WritePage, AllocatePage, Close) | Done |
| FreePage / Freelist | Not started |
| Buffer Pool (FetchPage, UnpinPage, FlushAll) | Done |
| Pager (ReadPage, WritePage, AllocatePage, Close) | Done |
| FreePage / Freelist | Not started |
| Buffer Pool (FetchPage, UnpinPage, FlushAll) | Done |
| Node read accessors (leafKey, leafValue, internalKey, childPageID) | Done |
| insertLeafEntry | Done |
| insertInternalEntry | In progress |
| BTree.Insert (single page, no split) | Done |
| splitLeaf | In progress |
| pushUp / new root creation | In progress |
| splitInternal | Not started |
| BTree.Get | Not started |
| BTree.Delete | Not started |
| BTree.Scan + Iterator | Not started |
