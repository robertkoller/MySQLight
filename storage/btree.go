package storage

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
)

var ErrDuplicateKey = errors.New("duplicate key")
var ErrEntryTooLarge = errors.New("Entry too large to fit into page")
var ErrNotFound = errors.New("entry not found")

const entryCap = ((PageSize - slotStart) / 3) // so that when we split the page in half we dont overflow with 4000 bytes entries

// Iterator is returned by Scan for range queries.
type Iterator interface {
	Next() (key, value []byte, err error) // returns io.EOF when exhausted
	Close() error
}

var _ = io.EOF // ensure io is used

type BTree struct {
	pager      *Pager
	rootPageID uint32
}

// NewBTree initialises a B+ tree backed by the given pager. If rootPageID is zero,
// a new page is allocated and formatted as an empty leaf node to serve as the initial root.
// Otherwise the tree opens at the existing root page.
func NewBTree(pager *Pager, rootPageID uint32) (*BTree, error) {
	if rootPageID == uint32(0) {
		id, err := pager.AllocatePage()
		if err != nil {
			return nil, err
		}
		bytes := makeNewLeafHeader()
		if err := pager.WritePage(id, bytes); err != nil {
			return nil, err
		}
		return &BTree{pager: pager, rootPageID: id}, nil

	}

	return &BTree{pager: pager, rootPageID: rootPageID}, nil
}

// Insert adds a key-value pair to the tree, maintaining sorted order within leaf pages.
// If inserting into the target leaf causes it to overflow, the leaf is split and the median
// key is pushed up to the parent. Splits propagate upward recursively; if the root itself
// splits, a new root page is allocated to keep the tree balanced.
func (t *BTree) Insert(key, value []byte) error {
	if len(key)+len(value)+8 > entryCap {
		return ErrEntryTooLarge
	}
	leafPageID, path, err := t.findLeaf(key)
	if err != nil {
		return err
	}
	node, err := t.decodeNodeNum(leafPageID)
	if err != nil {
		return err
	}

	insert := binarySearchKeys(node, key, true)
	if insert < int(node.keyCount()) && bytes.Equal(node.leafKey(insert), key) {
		return ErrDuplicateKey
	}

	needed := (len(key) + len(value) + 8)
	available := int(node.findFreeSpace()) - (slotStart + int(node.keyCount())*8)
	if needed > available {
		if err := t.splitLeaf(node, leafPageID, path); err != nil {
			return err
		}
		// Gotta recall this just in case the leaf changed
		leafPageID, path, err = t.findLeaf(key) // TODO: Make this more efficient my not traversing fully every time
		if err != nil {
			return err
		}
		node, err = t.decodeNodeNum(leafPageID)
		if err != nil {
			return err
		}
	}

	insert = binarySearchKeys(node, key, true)
	if insert < int(node.keyCount()) && bytes.Equal(node.leafKey(insert), key) {
		return ErrDuplicateKey
	}

	// This shouldnt be reachable here
	// but we recheck anyway because if that invariant is ever violated because of future code change
	// it would silently corrupt so we add this safety check for future code improvements
	needed = (len(key) + len(value) + 8)
	available = int(node.findFreeSpace()) - (slotStart + int(node.keyCount())*8)
	if needed > available {
		return ErrEntryTooLarge
	}

	node.insertLeafEntry(key, value, insert)

	if err := t.pager.WritePage(leafPageID, node.data); err != nil {
		return err
	}
	return nil
}

// We dont want pages to overflow so in case it does we split it into 2 pages here
func (t *BTree) splitLeaf(node *Node, leafPageID uint32, path []uint32) error {
	currRightSibling := node.rightSibling()
	pageID, err := t.pager.AllocatePage()
	if err != nil {
		return err
	}

	// Get Keys and values into memmory
	var keys [][]byte
	var values [][]byte
	for i := 0; i < int(node.keyCount()); i++ {
		keys = append(keys, node.leafKey(i))
		values = append(values, node.leafValue(i))
	}

	count := node.keyCount()
	// midpoint := count / 2
	// this gets us fancy midpoint by size
	total := 0
	sizes := make([]int, count)
	for i := 0; i < int(count); i++ {
		sizes[i] = len(keys[i]) + len(values[i]) + 8
		total += sizes[i]
	}

	midpoint := 0
	acc := 0
	for midpoint < int(count) && acc+sizes[midpoint] < total/2 {
		acc += sizes[midpoint]
		midpoint++
	}

	if midpoint == 0 {
		midpoint = 1
	}
	if midpoint == int(count) {
		midpoint = int(count) - 1
	}

	//left half
	node.data = makeNewLeafHeader()
	for i := 0; i < int(midpoint); i++ {
		node.insertLeafEntry(keys[i], values[i], i)
	}

	// This might be the right half or something idk ;)
	rightHalf := makeNewLeafHeader()
	rightNode, err := decodeNode(pageID, rightHalf)
	if err != nil {
		return err
	}
	rightNode.setRightSibling(currRightSibling)
	for i := midpoint; i < int(count); i++ {
		rightNode.insertLeafEntry(keys[i], values[i], int(i-midpoint))
	}

	// set the new page as a child of the oldone
	node.setRightSibling(pageID)

	// write both pages
	if err := t.pager.WritePage(leafPageID, node.data); err != nil {
		return err
	}
	if err := t.pager.WritePage(pageID, rightHalf); err != nil {
		return err
	}

	return t.pushUp(keys[midpoint], pageID, path)
}

func (t *BTree) splitInternal(node *Node, nodePageID uint32, newKey []byte, newChild uint32, path []uint32) error {
	rightPageID, err := t.pager.AllocatePage()
	if err != nil {
		return err
	}
	oldCount := int(node.keyCount())
	var oldKeys [][]byte
	var oldChildren []uint32
	for i := 0; i < oldCount; i++ {
		oldKeys = append(oldKeys, node.internalKey(i))
		oldChildren = append(oldChildren, node.childPageID(i))
	}
	oldChildren = append(oldChildren, node.childPageID(oldCount)) // add one extra cuz we got n+1 childs

	index := binarySearchKeys(node, newKey, false)
	keys := make([][]byte, 0, oldCount+1)
	keys = append(keys, oldKeys[:index]...)
	keys = append(keys, newKey)
	keys = append(keys, oldKeys[index:]...)

	children := make([]uint32, 0, oldCount+2)
	children = append(children, oldChildren[:index+1]...)
	children = append(children, newChild)
	children = append(children, oldChildren[index+1:]...)

	count := len(keys)
	// fancy smancy midpoint seperation
	//midpoint := count / 2

	total := 0
	sizes := make([]int, count)
	for i := 0; i < count; i++ {
		sizes[i] = len(keys[i]) + 4 + 4
		total += sizes[i]
	}
	midpoint := 0
	acc := 0
	for midpoint < count && acc+sizes[midpoint] <= total/2 {
		acc += sizes[midpoint]
		midpoint++
	}
	if midpoint == 0 {
		midpoint = 1
	}
	if midpoint >= count-1 {
		midpoint = count - 2
	}

	// leftchild
	node.data = makeNewInternalHeader()
	binary.BigEndian.PutUint32(node.data[5:], children[0])
	for i := 0; i < midpoint; i++ {
		node.insertInternalEntry(keys[i], i, children[i+1])
	}

	rightHalf := makeNewInternalHeader()
	rightNode, err := decodeNode(rightPageID, rightHalf)
	if err != nil {
		return err
	}
	binary.BigEndian.PutUint32(rightHalf[5:], children[midpoint+1])
	for i := midpoint + 1; i < count; i++ {
		rightNode.insertInternalEntry(keys[i], i-(midpoint+1), children[i+1])
	}

	// write both pages
	if err := t.pager.WritePage(nodePageID, node.data); err != nil {
		return err
	}
	if err := t.pager.WritePage(rightPageID, rightHalf); err != nil {
		return err
	}

	return t.pushUp(keys[midpoint], rightPageID, path[:len(path)-1])

}

func (t *BTree) pushUp(medianKey []byte, rightPageID uint32, path []uint32) error {
	if len(path) == 0 {
		// root leaf was split
		rootID, err := t.pager.AllocatePage()
		if err != nil {
			return err
		}

		root := make([]byte, PageSize)
		root[0] = byte(NodeInternal)
		binary.BigEndian.PutUint16(root[1:], 1)
		binary.BigEndian.PutUint32(root[5:], t.rootPageID)
		binary.BigEndian.PutUint32(root[9:], rightPageID)

		keyOffset := uint16(PageSize) - uint16(len(medianKey))
		binary.BigEndian.PutUint16(root[13:], keyOffset)
		binary.BigEndian.PutUint16(root[15:], uint16(len(medianKey)))

		copy(root[keyOffset:], medianKey)

		binary.BigEndian.PutUint16(root[3:], uint16(PageSize)-uint16(len(medianKey)))

		if err := t.pager.WritePage(rootID, root); err != nil {
			return err
		}
		t.rootPageID = rootID

	} else {
		parentID := path[len(path)-1]
		parentNode, err := t.decodeNodeNum(parentID)
		if err != nil {
			return err
		}

		needed := (len(medianKey) + 4 + 4)
		available := int(parentNode.findFreeSpace()) - (5 + (int(parentNode.keyCount())+1)*4 + int(parentNode.keyCount())*4)
		if needed > available {
			return t.splitInternal(parentNode, parentID, medianKey, rightPageID, path)
		} else {
			index := binarySearchKeys(parentNode, medianKey, false)
			parentNode.insertInternalEntry(medianKey, index, rightPageID)
			if err := t.pager.WritePage(parentID, parentNode.data); err != nil {
				return err
			}
		}

	}

	return nil
}

// finds the leaf for inserting given a specific key
func (t *BTree) findLeaf(key []byte) (leafPageID uint32, path []uint32, err error) {
	return t.findLeafRecursive(t.rootPageID, []uint32{}, key)
}

// recursively traverses through the tree to find where to insert the key
func (t *BTree) findLeafRecursive(pageNum uint32, curr []uint32, key []byte) (leafPageID uint32, path []uint32, err error) {
	node, err := t.decodeNodeNum(pageNum)
	if err != nil {
		return pageNum, curr, err
	}
	if node.nodeType == NodeLeaf {
		return pageNum, curr, nil
	} else {
		insert := binarySearchKeys(node, key, false)
		childID := node.childPageID(insert)
		curr = append(curr, pageNum)
		return t.findLeafRecursive(childID, curr, key)
	}
}

// searches for where to insert a key
// if leaf = false its internal so we look for what child to descend into
func binarySearchKeys(node *Node, key []byte, leaf bool) int {
	low := 0
	high := int(node.keyCount()) - 1
	var comparison int
	for low <= high {
		mid := low + (high-low)/2
		if leaf {
			comparison = bytes.Compare(node.leafKey(mid), key)
		} else {
			comparison = bytes.Compare(node.internalKey(mid), key)
		}

		if leaf {
			if comparison < 0 {
				low = mid + 1
			} else {
				high = mid - 1
			}
		}
		if !leaf {
			if comparison <= 0 {
				low = mid + 1
			} else {
				high = mid - 1
			}
		}
	}

	return low

}

// Gives you the node given the page number
func (t *BTree) decodeNodeNum(pageNum uint32) (*Node, error) {
	page, err := t.pager.ReadPage(pageNum)
	if err != nil {
		return nil, err
	}
	node, err := decodeNode(pageNum, page)
	if err != nil {
		return nil, err
	}
	return node, nil
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
