# `btree.go` — The B+ Tree

The core data structure. Every table and every index in MySQLight is one of these.
It stores sorted `[]byte` key/value pairs across pages, splitting nodes as they
fill and merging/rotating as they empty so the tree stays balanced, and supports
point lookups, range scans, inserts, and deletes.

It builds on the page format in [`btree_node.go`](btree-node.md) and does all I/O
through the [BufferPool](buffer-pool.md) — every node is reached via
`pool.fetchNode` and released with `pool.UnpinPage`.

---

## Types

```go
type BTree struct {
    pool       *BufferPool
    rootPageID uint32       // changes when the root splits or collapses
}

type Iterator interface {
    Next() (key, value []byte, err error)  // io.EOF when exhausted
    Close() error
}
```

Keys are compared with `bytes.Compare` — the tree is type-agnostic. Errors:
`ErrDuplicateKey`, `ErrNotFound`, `ErrEntryTooLarge`.

### `NewBTree(pool, rootPageID) (*BTree, error)`
- `rootPageID == 0`: allocate a page, format it as an empty leaf, use it as root.
- otherwise: open at the existing root. (Nothing persists the root id yet — the
  caller must remember it across reopens; that becomes the catalog's job in a later
  phase.)

---

## Pin discipline (the invariant behind everything)

Every `fetchNode`/`FetchPage` is balanced by exactly one `UnpinPage`. `findLeaf`
unpins each node as it descends, so it returns an **unpinned** leaf id plus the
path of internal ids above it. Functions that hand a node to a helper document who
owns the unpin (e.g. `Delete` owns the leaf; `rebalanceLeaf` owns the parent and
siblings it fetches).

---

## Insert path

### `Insert(key, value) error`
1. Reject entries larger than `entryCap` (so two can't fail to fit after a split).
2. `findLeaf(key)` → leaf id + path. Reject duplicates.
3. Overflow check (`needed` vs `available`, in `int`). If it would overflow,
   `splitLeaf`, then **re-find** the target leaf (the split may have moved the key
   right or grown the root).
4. `binarySearchKeys(node, key, true)` → slot; `insertLeafEntry`; unpin dirty.

### `splitLeaf` / `splitInternal`
Both split a full node by a **byte-weighted midpoint** (not a count midpoint), so
variable-size entries divide evenly:
- **Leaf:** copy entries out, reformat the original as the left half, build the
  right half on a newly allocated page, fix the right-sibling chain, then
  `pushUp(median, rightPage, path)`. The median is **copied** up (the real entry
  stays in the right leaf).
- **Internal:** splice the incoming `(newKey, newChild)` into the key/child arrays,
  pick the byte-midpoint, rebuild left + right pages. The median is **moved** up
  (internal keys are only routers), via `pushUp(median, rightPage, path[:len-1])`.

### `pushUp(medianKey, rightPageID, path)`
- **`path` empty (root split):** allocate a new internal root with two children and
  the median key; update `rootPageID`; the tree grows one level.
- **otherwise:** load `path[last]`. If it has room, `insertInternalEntry`; if full,
  `splitInternal`, which recurses upward.

---

## Get

### `Get(key) ([]byte, error)`
`findLeaf` → `binarySearchKeys(.., true)`; on an exact match return a **copy** of
the value (the page is unpinned on return), else `ErrNotFound`.

---

## Delete path

### `Delete(key) error`
`findLeaf` → locate the slot (`ErrNotFound` if absent) → `deleteLeafEntry`. If the
leaf is the root, or still ≥ half full (`leafLiveBytes() >= PageSize/2`), done.
Otherwise `rebalanceLeaf`.

### `rebalanceLeaf(leaf, key, path)`
Fetches the parent and both existing siblings, then decides — priority:
1. **Borrow** from a sibling that can spare an entry and stay ≥ half full (pick the
   fuller one). Move one entry across and fix the parent separator with
   `replaceInternalKey`. (Compacts the destination leaf first so the append can't
   overflow a fragmented page.)
2. else **merge** if the two leaves actually fit in one page — rebuild the survivor
   from both nodes' live entries, fix the sibling chain, remove the separator from
   the parent (`deleteInternalEntry`), and `FreePage` the dead leaf.
3. else **forced borrow** from the (necessarily large) sibling — covers the case
   where a sibling is too full to lend yet too full to merge with, which variable-
   size entries make possible.

A merge shrinks the parent, which is then fixed up: if the parent is the root and
collapsed to a single child, promote that child and free the old root; else if the
parent underflows, recurse into `rebalanceInternal`.

### `rebalanceInternal(node, key, path)`
Same shape, but internal mechanics differ:
- **Rotation (borrow):** rotate *through the parent* — the parent separator descends
  into `node`, the sibling's boundary key ascends to replace it, and the sibling's
  boundary child pointer moves across. Pages are rebuilt via `rebuildInternal`.
- **Merge:** the surviving node holds `[left keys] + [parent separator] + [right
  keys]` with all children concatenated; the separator is **pulled down** from the
  parent. Then `deleteInternalEntry` on the parent and recurse / collapse the root,
  exactly like the leaf case.

`node` is caller-owned (the caller unpins it); `rebalanceInternal` unpins only the
parent and siblings it fetches. All `FreePage` calls happen at tail positions, and
freeing a still-pinned dead page is safe because `pool.FreePage` drops it from the
cache (the later `UnpinPage` no-ops). See [freelist.md](freelist.md).

---

## Scan

### `Scan(start, end) (Iterator, error)`
Positions a cursor: `findLeaf(start)` (or the leftmost leaf if `start == nil`),
fetch+**pin** that leaf, and set the start slot via `binarySearchKeys(.., true)`.
Returns a `*leafIterator` that owns the pin until `Close`.

### `leafIterator`
Holds `pool`, the current `pageID`/`node`, the `slot`, the exclusive `end` bound,
and a `closed` flag.
- **`Next()`** — skip exhausted/empty leaves by following `rightSibling` (unpin the
  old leaf, pin the next; `next == 0` → close + `io.EOF`). Read the entry; if
  `end != nil && key >= end`, close + `io.EOF` (bound is exclusive). **Copy** the
  key/value before returning (the page is unpinned on the next sibling cross or
  `Close`), then advance `slot`.
- **`Close()`** — idempotent; unpins the currently-held leaf.

Invariant: exactly one leaf pinned between `Next` calls, released on every exit
path (sibling cross, bound hit, end of chain, `Close`).

---

## Search helpers

- **`findLeaf(key)` / `findLeafRecursive`** — walk root→leaf, descending into the
  first child whose separator exceeds `key` (equality routes right, which is correct
  for a B+ tree), accumulating the internal path. Unpins each node as it descends.
- **`binarySearchKeys(node, key, leaf)`** — returns the insertion index / exact-match
  index. `leaf == true` compares via `leafKey`, else `internalKey`.

---

## Integration

- **Below:** [`btree_node.go`](btree-node.md) for page layout; the
  [BufferPool](buffer-pool.md) for all I/O, allocation, and frees; the freelist
  (in the pager) reclaims merged-away pages.
- **Above:** the catalog and executor (later phases) treat each table/index as a
  `BTree` of serialized rows keyed by primary key.

> Open item: `FreePage` for the freelist is wired in, but `pager.go`'s I/O is still
> `Seek`-based; and the Insert path's split decision uses the leaked free-space
> pointer rather than live bytes, so a heavily delete-then-insert leaf can split
> slightly earlier than necessary (correct, just not optimal).
