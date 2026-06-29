# `btree_node.go` — Page Layout & Node Accessors

A B+ tree node is exactly one 4096-byte page. This file defines how the raw bytes
of a page are interpreted as either a **leaf** or an **internal** node, and the
read/write helpers the tree uses. It contains no tree logic (no traversal, no
splitting/merging decisions) — only the per-page byte format and primitives.

---

## Types & constants

```go
type NodeType uint8
const (
    NodeLeaf     NodeType = 0x01
    NodeInternal NodeType = 0x02
)

const slotStart = 9   // byte offset where a LEAF's slot array begins

type Node struct {
    pageID   uint32
    nodeType NodeType
    data     []byte    // the raw page bytes (length PageSize), aliases the cached page
}
```

`Node` is a thin wrapper over the page bytes; accessors read/write directly into
`data`. Integers are big-endian throughout.

---

## Leaf node layout

```
[0]      nodeType      1 B   (0x01)
[1–2]    keyCount      2 B   uint16
[3–4]    freeSpacePtr  2 B   uint16  — start of the key/value data region; begins at 4096
[5–8]    rightSibling  4 B   uint32  — page ID of next leaf in sorted order (0 = none)
[9–...]  slot array    keyCount × 8 B
[ ...free gap... ]
[ key/value data grows ← downward from the end of the page ]
```

**Slot entry (8 bytes), kept in sorted key order:** `keyOffset, keyLen,
valueOffset, valueLen` (each uint16). The slot array grows **up** from offset 9;
key/value bytes grow **down** from 4096. They meet when the page is full.

---

## Internal node layout

```
[0]      nodeType      1 B   (0x02)
[1–2]    keyCount      2 B   uint16
[3–4]    freeSpacePtr  2 B   uint16
[5–...]  child IDs     (keyCount+1) × 4 B   uint32
[...]    key slot array keyCount × 4 B
[ key bytes grow ← downward from the end of the page ]
```

**Key slot entry (4 bytes):** `keyOffset, keyLen` (uint16). For `n` keys there are
always `n+1` child pointers; child `i` holds keys `x` with `key[i-1] ≤ x < key[i]`.
Internal nodes store **no values** and **no right-sibling** — pure routers.

> **Dynamic slot base.** The key slot array starts at `5 + (keyCount+1)*4`. Adding
> a child pointer shifts this base right by 4 bytes, so `internalKey` recomputes
> the base from the live `keyCount` on every call, and `insertInternalEntry` shifts
> existing key slots to keep them aligned.

---

## Headers & counters

| Method | What it does |
|--------|--------------|
| `makeNewLeafHeader() []byte` | Fresh empty leaf page: type byte, `keyCount = 0`, `freeSpacePtr = PageSize`. |
| `makeNewInternalHeader() []byte` | Fresh empty internal page: type byte, `keyCount = 0`, `freeSpacePtr = PageSize`. |
| `decodeNode(pageID, data) (*Node, error)` | Wraps raw bytes in a `Node`; errors if byte 0 isn't a known node type. |
| `keyCount() uint16` / `incrementKeyCount()` / `decrementKeyCount()` | Read/adjust the key count at offset 1. |
| `findFreeSpace() uint16` / `newFreeSpace(ptr)` | Read/write `freeSpacePtr` at offset 3. |
| `isLeaf() bool` | `nodeType == NodeLeaf`. |
| `rightSibling() uint32` / `setRightSibling(id)` | Get/set the 4-byte sibling pointer at offset 5. **Leaf-only** — on an internal node offset 5 is `child[0]`. |

---

## Leaf accessors

- **`insertLeafEntry(key, value, slotIndex)`** — pack value then key at the
  descending `freeSpacePtr`, build the 8-byte slot, shift slots `[slotIndex..]`
  right by 8, write the new slot, `keyCount++`. Overflow is the caller's check.
- **`deleteLeafEntry(slotIndex)`** — remove the slot by shifting the slots above it
  down by 8, `keyCount--`. **Does not reclaim** the key/value bytes (the data
  region's `freeSpacePtr` only moves down) — that dead space is reclaimed by a
  later rebuild/compaction. This is *the* subtlety behind the delete-path helpers
  below.
- **`leafKey(i) []byte` / `leafValue(i) []byte`** — read slot `i` and return the
  bytes from within the page (a slice into `data`, so copy before the page can be
  unpinned).
- **`leafLiveBytes() int`** — `slotStart + keyCount*8 + Σ(keyLen+valueLen)`. The
  *live* footprint (ignores dead bytes), used by `Delete` to detect underflow and
  to fit-check merges.

---

## Internal accessors

- **`insertInternalEntry(key, slotIndex, rightChildID)`** — inserts a separator at
  `slotIndex` and its right child at child position `slotIndex+1`, shifting key
  slots and child pointers to keep the dynamic base aligned. Verified for front,
  middle, and end insertions.
- **`deleteInternalEntry(separatorKeyIndex, childIndex)`** — rebuilds the node from
  its keys/children minus the named separator key and child pointer (compacting in
  the process).
- **`replaceInternalKey(i, newKey)`** — replaces separator `i` by **rebuilding** the
  node. It does *not* write in place: the data region only grows downward, so
  repeated in-place replacements (one per leaf-borrow and rotation) would leak the
  old key bytes and eventually march `freeSpacePtr` into the child/slot region.
  Rebuilding compacts on every call.
- **`internalKey(i) []byte`** — slot base `5 + (keyCount+1)*4`, read slot `i`.
- **`childPageID(i) uint32`** — uint32 child pointer at `5 + i*4`.
- **`internalLiveBytes() int`** — `5 + (keyCount+1)*4 + keyCount*4 + Σ keyLen`. The
  internal analogue of `leafLiveBytes`, used to detect internal underflow.

---

## Rebuild / compaction helpers

Because `deleteLeafEntry` and in-place key replacement leave dead bytes, the
delete path rebuilds pages from their live contents to reclaim space and keep the
physical layout consistent with `leafLiveBytes`/`internalLiveBytes`.

- **`rebuildLeaf(keys, values)`** — rewrites the leaf to hold exactly the given
  pairs, packed with no dead space; preserves the right-sibling pointer. Used by
  leaf merges (combine two nodes' live entries into the survivor).
- **`compactLeaf()`** — `rebuildLeaf` from the node's *own* live entries. Used
  before a borrow appends an entry, so the append can't overflow a fragmented page.
- **`rebuildInternal(keys, children)`** — rewrites the internal node to hold exactly
  the given separators and `len(keys)+1` children. Used by internal rotations and
  merges (and by `deleteInternalEntry`/`replaceInternalKey`).

---

## Overflow math (used by the caller, documented here for reference)

**Leaf** — free gap and cost of one entry:
```
freeGap = freeSpacePtr - (slotStart + keyCount*8)
needed  = len(key) + len(value) + 8        // key + value + one 8-byte slot
split if needed > freeGap
```

**Internal:**
```
freeGap = freeSpacePtr - (5 + (keyCount+1)*4 + keyCount*4)
needed  = len(key) + 4 + 4                  // key + one 4-byte slot + one child ptr
split if needed > freeGap
```

Do this arithmetic in `int`, not `uint16` — a nearly-full node makes the
subtraction go negative, which `uint16` would wrap into a huge value and skip a
needed split.

---

## Integration

`btree_node.go` is pure byte-layout plumbing, consumed entirely by
[`btree.go`](btree.md); it does no I/O and never touches the pager or buffer pool.
