# `btree_node.go` — Page Layout & Node Accessors

A B+ tree node is exactly one 4096-byte page. This file defines how the raw bytes
of a page are interpreted as either a **leaf** or an **internal** node, and the
read/write helpers the tree uses. It does not contain any tree logic (no traversal,
no splitting) — only the per-page byte format.

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
    data     []byte    // the raw page bytes (length PageSize)
}
```

`Node` is a thin wrapper over the page bytes; all accessors read/write directly
into `data`. Integers are big-endian throughout.

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

**Slot entry (8 bytes), kept in sorted key order:**

| Bytes | Field |
|-------|-------|
| 0–1 | keyOffset (uint16) |
| 2–3 | keyLen (uint16) |
| 4–5 | valueOffset (uint16) |
| 6–7 | valueLen (uint16) |

The slot array grows **up** from offset 9; key/value bytes grow **down** from
4096. They meet when the page is full.

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

**Key slot entry (4 bytes):** `keyOffset (uint16), keyLen (uint16)`.

For `n` keys there are always `n+1` child pointers. Child `i` holds all keys `x`
with `key[i-1] ≤ x < key[i]`. Internal nodes store **no values** and **no
right-sibling** — they are pure routers.

> **Dynamic slot base.** The key slot array starts at `5 + (keyCount+1)*4`. Adding
> a child pointer shifts this base right by 4 bytes, so `internalKey` recomputes
> the base from the live `keyCount` on every call, and `insertInternalEntry` must
> shift existing key slots to keep them aligned with the new base.

---

## Header / counter accessors

| Method | What it does |
|--------|--------------|
| `decodeNode(pageID, data) (*Node, error)` | Wraps raw bytes in a `Node`; errors if byte 0 isn't a known node type. |
| `keyCount() uint16` | Reads the key count at offset 1. |
| `findFreeSpace() uint16` | Reads `freeSpacePtr` at offset 3. |
| `newFreeSpace(ptr)` | Writes `freeSpacePtr`. |
| `incrementKeyCount()` | `keyCount++`. |
| `isLeaf() bool` | `nodeType == NodeLeaf`. |
| `makeNewLeafHeader() []byte` | Fresh empty leaf page: type byte, `keyCount = 0`, `freeSpacePtr = PageSize`. |

> **Missing helper:** there is no `makeNewInternalHeader()` yet. `splitInternal`
> needs it — same as `makeNewLeafHeader` but with `NodeInternal` and no sibling
> field.

---

## Leaf accessors

### `insertLeafEntry(key, value []byte, slotIndex int)`
1. Pack the value at `freeSpacePtr - len(value)`, then the key just below it;
   update `freeSpacePtr` to the key's offset.
2. Build the 8-byte slot (key/value offsets + lengths).
3. Shift slots `[slotIndex..]` right by 8 bytes (Go's `copy` is memmove-safe for
   the overlap) and write the new slot at `slotIndex`.
4. `incrementKeyCount()`.

Overflow is **not** checked here — the caller (`BTree.Insert`) checks before
calling and splits if needed.

### `leafKey(i) []byte` / `leafValue(i) []byte`
Read slot `i` (at `9 + i*8`) and return the key (offset/len at +0/+2) or value
(offset/len at +4/+6) bytes from within the page.

### `rightSibling() uint32` / `setRightSibling(pageID)`
Get/set the 4-byte sibling pointer at offset 5. Leaf-only; on an internal node
offset 5 is `child[0]`, so don't call these on internal nodes.

---

## Internal accessors

### `insertInternalEntry(key []byte, slotIndex int, rightChildID uint32)`
Inserts a separator key at `slotIndex` and its right child at child position
`slotIndex+1`. Because adding a child pointer grows the child region by 4 bytes,
the key slots must move to stay aligned with the dynamic base. The net effect:

1. Pack the key bytes at `freeSpacePtr - len(key)`.
2. Shift key slots `[slotIndex..keyCount-1]` right by 8 (4 for the new child
   pointer + 4 for the new key-slot gap).
3. Shift key slots `[0..slotIndex-1]` right by 4 (child pointer growth only).
4. Shift child pointers `[slotIndex+1..keyCount]` right by 4 to open the gap.
5. Write `rightChildID` at child position `slotIndex+1`.
6. Write the new key slot at `newSlotsBase + slotIndex*4`, where
   `newSlotsBase = 5 + (keyCount+2)*4`.
7. `incrementKeyCount()`.

Verified for front, middle, and end (append) insertions. Like the leaf version,
overflow is the caller's responsibility.

### `internalKey(i) []byte`
Computes the slot base `5 + (keyCount+1)*4`, reads slot `i`'s offset/len at
`base + i*4`, and returns the key bytes.

### `childPageID(i) uint32`
Reads the uint32 child pointer at `5 + i*4`.

---

## Overflow math (used by the caller, documented here for reference)

**Leaf** — free gap before insert and cost of one entry:
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

## Stale comments to ignore

The doc comments above `rightSibling` and `childPageID` say "offset 3"; the code
correctly uses offset 5. The code is right, the comments are wrong.

---

## Integration

`btree_node.go` is pure byte-layout plumbing. It is consumed entirely by
[`btree.go`](btree.md); it does no I/O and never touches the pager or buffer pool.
