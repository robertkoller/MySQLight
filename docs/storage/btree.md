# `btree.go` — The B+ Tree

The core data structure. Every table and every index in MySQLight is one of
these. It stores sorted `[]byte` key/value pairs across pages, splitting nodes as
they fill so the tree stays balanced, and (when finished) supports point lookups,
range scans, and deletes.

It builds on the page format in [`btree_node.go`](btree-node.md) and currently
does its I/O directly through the [Pager](pager.md) (the [BufferPool](buffer-pool.md)
is intended to sit in between but is not yet wired in).

---

## Types

```go
type BTree struct {
    pager      *Pager
    rootPageID uint32
}

type Iterator interface {
    Next() (key, value []byte, err error)  // io.EOF when exhausted
    Close() error
}
```

Keys are compared with `bytes.Compare` — the tree is type-agnostic.

---

## `NewBTree(pager, rootPageID) (*BTree, error)`
- `rootPageID == 0`: allocate a page, format it as an empty leaf
  (`makeNewLeafHeader`), write it, and use it as the root.
- otherwise: open the tree at the existing root.

---

## Insert path

### `Insert(key, value) error`
1. `findLeaf(key)` → target leaf page ID + the path of internal pages above it.
2. Compute the leaf overflow check (`needed` vs `available`, in `int`).
3. If it would overflow, `splitLeaf`, then **re-run `findLeaf`** — the split may
   have moved the target key into the new right leaf (and may even have grown the
   root), so the original leaf reference can be stale.
4. `binarySearchKeys(node, key, true)` finds the sorted slot index.
5. `insertLeafEntry` writes the entry; the page is written back.

### `splitLeaf(node, leafPageID, path) error`
Splits a full leaf into two:
1. Save the current `rightSibling` before reformatting.
2. Allocate a new page for the right half.
3. Copy all key/value pairs into memory.
4. Reformat the original page and re-insert the left half `[0, midpoint)`.
5. Build the right half `[midpoint, count)` on the new page; set its right sibling
   to the saved value, and set the original leaf's right sibling to the new page.
6. Write both pages.
7. `pushUp(keys[midpoint], newPageID, path)` — the median key is **copied** up
   (it still lives in the right leaf, because leaves hold the real data).

### `splitInternal(node, nodePageID, newKey, newChild, path) error` — *in progress*
Splits a full internal node while inserting a new separator. Unlike a leaf split,
the median is **moved** up, not copied — internal keys are only routers.

Intended algorithm (collect → insert → split):
1. Allocate the right page.
2. Collect existing keys (`internalKey`) and the `n+1` children (`childPageID`)
   into memory **with `append`** (not index assignment into nil slices).
3. `index = binarySearchKeys(node, newKey, false)`.
4. Splice into fresh slices (avoid append-aliasing): keys become
   `keys[:index] + newKey + keys[index:]`; children become
   `children[:index+1] + newChild + children[index+1:]`.
5. `m = len(keys)/2`. `keys[m]` is pushed up and belongs to **neither** half.
   - left = `keys[0..m-1]`, `children[0..m]` (rebuilt in place on `nodePageID`)
   - right = `keys[m+1..]`, `children[m+1..]` (new page)
   Rebuild each via `makeNewInternalHeader`, write `children[0]` at offset 5, then
   `insertInternalEntry` the rest.
6. Write both pages, then `pushUp(keys[m], rightPageID, path[:len-1])`.

> **Current status:** not finished/compiling. The collection loop assigns into
> nil slices (needs `append`), the splice is reversed and has an aliasing bug, the
> median/rebuild/write/return steps are missing, and `makeNewInternalHeader` does
> not exist yet.

### `pushUp(medianKey, rightPageID, path) error`
Inserts a separator + new right child one level up.
- **`path` empty (root split):** allocate a new root internal node with two
  children (old root + new right page) and one key (`medianKey`), and update
  `rootPageID`. The tree grows one level.
- **otherwise:** load `path[last]`. If it has room, find the slot with
  `binarySearchKeys(parent, medianKey, false)`, `insertInternalEntry`, write it.
  If it is full, hand off to `splitInternal`, which inserts and recurses upward.

> **Current status:** the root-split branch and the has-room branch are correct.
> The full-parent branch still calls `splitLeaf` on an internal node (wrong layout)
> — it should call `splitInternal`.

---

## Search helpers

### `findLeaf(key)` / `findLeafRecursive(pageNum, path, key)`
Walks from the root to the leaf that should contain `key`. At each internal node
it scans keys left to right and descends into the first child whose separator is
greater than `key` (or the last child if none is), accumulating the path of
internal page IDs. Equality with a separator routes **right**, which is correct:
the separator equals the first key of the right subtree's leaf.

### `binarySearchKeys(node, key, leaf bool) int`
Binary search returning the insertion index (`low`) where `key` belongs, or the
index of an exact match. `leaf == true` compares via `leafKey`; `false` via
`internalKey`. Relies on `bytes.Compare` returning exactly -1/0/1.

### `decodeNodeNum(pageNum) (*Node, error)`
Reads a page through the pager and wraps it as a `Node`. This is the single choke
point that will switch to `BufferPool.FetchPage` when the pool is wired in.

---

## Not yet implemented

| Method | Plan |
|--------|------|
| `Get(key)` | Reuse `findLeaf` + `binarySearchKeys(.., true)`; return the value or `ErrNotFound`. (Needs an exported `ErrNotFound`.) |
| `Scan(start, end)` | Find the start leaf, return an iterator that walks the right-sibling chain yielding entries until it passes `end`. |
| `Delete(key)` | Remove the leaf entry; on underflow borrow from a sibling, else merge and remove the separator from the parent, cascading upward. Frees emptied pages to the [Freelist](freelist.md). |

---

## Known open items (insert path)

- `splitInternal` unfinished (see above) and `pushUp` still calls `splitLeaf` for
  full internal parents.
- `makeNewInternalHeader` missing.
- Duplicate keys are not rejected — `Insert` will store a second entry for a key
  that already exists. Primary keys are meant to be unique.
- The buffer pool is not used; every traversal hits the pager directly.

---

## Integration

- **Below:** [`btree_node.go`](btree-node.md) for page layout; [Pager](pager.md)
  (eventually [BufferPool](buffer-pool.md)) for I/O; [Freelist](freelist.md) for
  page reuse once `Delete` exists.
- **Above:** the catalog and executor (later phases) treat each table/index as a
  `BTree` of serialized rows keyed by primary key.
