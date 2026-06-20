package storage

import "io"

// Iterator is returned by Scan for range queries.
type Iterator interface {
	Next() (key, value []byte, err error) // returns io.EOF when exhausted
	Close() error
}

var _ = io.EOF // ensure io is used

type BTree struct {
	// TODO: pager      *Pager
	// TODO: rootPageID uint32
}

// NewBTree initialises a B+ tree backed by the given pager. If rootPageID is zero,
// a new page is allocated and formatted as an empty leaf node to serve as the initial root.
// Otherwise the tree opens at the existing root page.
func NewBTree(pager *Pager, rootPageID uint32) (*BTree, error) {
	// TODO: if rootPageID == 0, allocate a new page via pager.AllocatePage
	//         and initialise it as an empty leaf node (see btree_node.go)
	// TODO: return a BTree pointing at that root
	panic("not implemented")
}

// Insert adds a key-value pair to the tree, maintaining sorted order within leaf pages.
// If inserting into the target leaf causes it to overflow, the leaf is split and the median
// key is pushed up to the parent. Splits propagate upward recursively; if the root itself
// splits, a new root page is allocated to keep the tree balanced.
func (t *BTree) Insert(key, value []byte) error {
	// TODO: call findLeaf(key) to walk internal nodes down to the correct leaf
	// TODO: insert the key-value pair into the leaf (keep keys sorted)
	// TODO: if the leaf overflows: splitLeaf → push median key up to parent
	// TODO: if the parent overflows: splitInternal → recurse up
	// TODO: if the root splits: allocate a new root page, make it an internal node
	panic("not implemented")
}

// Delete removes the entry with the given key from the tree. If the key is not found,
// it returns ErrNotFound. After removal, if the leaf falls below half capacity, it tries
// to borrow an entry from an adjacent sibling. If the sibling is too small to lend, the
// two nodes are merged and the separator key is removed from the parent, which may trigger
// further merges up the tree.
func (t *BTree) Delete(key []byte) error {
	// TODO: find the leaf containing key; return ErrNotFound if absent
	// TODO: remove the key-value pair from the leaf
	// TODO: if the leaf underflows (fewer than half capacity):
	//         try to redistribute (borrow) from an adjacent sibling
	//         if sibling is too small to lend: merge, remove separator from parent, recurse up
	panic("not implemented")
}

// Get traverses internal nodes using key comparisons to reach the correct leaf page,
// then binary-searches the leaf for the key and returns the associated value.
// Returns ErrNotFound if the key does not exist in the tree.
func (t *BTree) Get(key []byte) ([]byte, error) {
	// TODO: traverse internal nodes using key comparisons to reach the leaf
	// TODO: binary search the leaf page for key
	// TODO: return the value, or ErrNotFound if key is absent
	panic("not implemented")
}

// Scan returns an iterator that yields key-value pairs in sorted order from start to end.
// It finds the leaf containing start (or the leftmost leaf if start is nil), then walks
// the right-sibling pointer chain. The iterator returns io.EOF once it passes end or
// exhausts all leaf pages.
func (t *BTree) Scan(start, end []byte) Iterator {
	// TODO: traverse to the leaf containing start (or the leftmost leaf if start == nil)
	// TODO: return an iterator that reads entries in order, following right-sibling pointers
	// TODO: iterator.Next() returns io.EOF once it passes end (or exhausts all leaves)
	panic("not implemented")
}
