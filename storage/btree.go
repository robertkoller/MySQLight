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

func NewBTree(pager *Pager, rootPageID uint32) (*BTree, error) {
	// TODO: if rootPageID == 0, allocate a new page via pager.AllocatePage
	//         and initialise it as an empty leaf node (see btree_node.go)
	// TODO: return a BTree pointing at that root
	panic("not implemented")
}

func (t *BTree) Insert(key, value []byte) error {
	// TODO: call findLeaf(key) to walk internal nodes down to the correct leaf
	// TODO: insert the key-value pair into the leaf (keep keys sorted)
	// TODO: if the leaf overflows: splitLeaf → push median key up to parent
	// TODO: if the parent overflows: splitInternal → recurse up
	// TODO: if the root splits: allocate a new root page, make it an internal node
	panic("not implemented")
}

func (t *BTree) Delete(key []byte) error {
	// TODO: find the leaf containing key; return ErrNotFound if absent
	// TODO: remove the key-value pair from the leaf
	// TODO: if the leaf underflows (fewer than half capacity):
	//         try to redistribute (borrow) from an adjacent sibling
	//         if sibling is too small to lend: merge, remove separator from parent, recurse up
	panic("not implemented")
}

func (t *BTree) Get(key []byte) ([]byte, error) {
	// TODO: traverse internal nodes using key comparisons to reach the leaf
	// TODO: binary search the leaf page for key
	// TODO: return the value, or ErrNotFound if key is absent
	panic("not implemented")
}

func (t *BTree) Scan(start, end []byte) Iterator {
	// TODO: traverse to the leaf containing start (or the leftmost leaf if start == nil)
	// TODO: return an iterator that reads entries in order, following right-sibling pointers
	// TODO: iterator.Next() returns io.EOF once it passes end (or exhausts all leaves)
	panic("not implemented")
}
