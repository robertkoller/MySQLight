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
	Next() (key []byte, value []byte, err error) // returns io.EOF when exhausted
	Close() error
}

var _ = io.EOF // ensure io is used

// BTree is a disk-backed B+ tree reached through the buffer pool; rootPageID is the current
// root and changes whenever the root splits.
type BTree struct {
	pool       *BufferPool
	rootPageID uint32
}

type leafIterator struct {
	pool   *BufferPool
	pageID uint32
	slot   int
	end    []byte
	node   *Node
	closed bool
}

// Compile time check that *leafIterator satisfies Iterator
var _ Iterator = (*leafIterator)(nil)

// (it *leafIterator) is the receiver — like `self`/`this`. Pointer receiver
// because Next mutates state (slot, pageID, closed).
func (it *leafIterator) Next() (key []byte, value []byte, err error) {
	if it.closed {
		return nil, nil, io.EOF
	}

	for it.slot >= int(it.node.keyCount()) {
		next := it.node.rightSibling()
		if next == 0 { // this is the rightmost sibling (end of the line buddy)
			it.Close()
			return nil, nil, io.EOF
		}
		_, newNode, err := it.pool.fetchNode(next)
		if err != nil {
			return nil, nil, err
		}
		it.pool.UnpinPage(it.pageID, false)
		it.node = newNode
		it.pageID = next
		it.slot = 0
	}

	keyCurr := append([]byte(nil), it.node.leafKey(it.slot)...)
	valueCurr := append([]byte(nil), it.node.leafValue(it.slot)...)

	if it.end != nil && bytes.Compare(keyCurr, it.end) >= 0 {
		it.Close()
		return nil, nil, io.EOF
	}

	it.slot++
	return keyCurr, valueCurr, nil
}

// Closes the iterator off
func (it *leafIterator) Close() error {
	if it.closed {
		return nil
	}
	it.pool.UnpinPage(it.pageID, false)
	it.closed = true
	return nil
}

// NewBTree initialises a B+ tree backed by the given buffer pool. If rootPageID is zero,
// a new page is allocated and formatted as an empty leaf node to serve as the initial root.
// Otherwise the tree opens at the existing root page.
func NewBTree(pool *BufferPool, rootPageID uint32) (*BTree, error) {
	if rootPageID == uint32(0) {
		id, err := pool.AllocatePage()
		if err != nil {
			return nil, err
		}
		bytes := makeNewLeafHeader()
		page, err := pool.FetchPage(id)
		if err != nil {
			return nil, err
		}
		copy(page.Data, bytes)
		pool.UnpinPage(id, true)
		return &BTree{pool: pool, rootPageID: id}, nil

	}

	return &BTree{pool: pool, rootPageID: rootPageID}, nil
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
	page, node, err := t.pool.fetchNode(leafPageID)
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
		median, rightPageID, err := t.splitLeaf(page, node, path)
		if err != nil {
			return err
		}
		t.pool.UnpinPage(leafPageID, true)
		if bytes.Compare(key, median) >= 0 {
			leafPageID = rightPageID
		}
		page, node, err = t.pool.fetchNode(leafPageID)
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

	t.pool.UnpinPage(leafPageID, true)
	return nil
}

// Get traverses internal nodes using key comparisons to reach the correct leaf page,
// then binary-searches the leaf for the key and returns the associated value.
// Returns ErrNotFound if the key does not exist in the tree.
func (t *BTree) Get(key []byte) ([]byte, error) {
	leafPageID, _, err := t.findLeaf(key)
	if err != nil {
		return nil, err
	}
	_, node, err := t.pool.fetchNode(leafPageID)
	if err != nil {
		return nil, err
	}

	insert := binarySearchKeys(node, key, true)
	if insert < int(node.keyCount()) && bytes.Equal(node.leafKey(insert), key) {
		value := append([]byte(nil), node.leafValue(insert)...)
		t.pool.UnpinPage(leafPageID, false)
		return value, nil
	}
	t.pool.UnpinPage(leafPageID, false)
	return nil, ErrNotFound
}

// Delete removes the entry with the given key from the tree. If the key is not found,
// it returns ErrNotFound. After removal, if the leaf falls below half capacity, it tries
// to borrow an entry from an adjacent sibling. If the sibling is too small to lend, the
// two nodes are merged and the separator key is removed from the parent, which may trigger
// further merges up the tree.
func (t *BTree) Delete(key []byte) error {
	leafPageID, path, err := t.findLeaf(key)
	if err != nil {
		return err
	}
	_, node, err := t.pool.fetchNode(leafPageID)
	if err != nil {
		return err
	}

	insert := binarySearchKeys(node, key, true)
	if insert >= int(node.keyCount()) || !bytes.Equal(node.leafKey(insert), key) {
		t.pool.UnpinPage(leafPageID, false)
		return ErrNotFound
	}
	node.deleteLeafEntry(insert)

	if len(path) == 0 || node.leafLiveBytes() >= PageSize/2 {
		t.pool.UnpinPage(node.pageID, true)
		return nil
	}
	err = t.rebalanceLeaf(node, key, path)
	t.pool.UnpinPage(leafPageID, true)
	return err
}

// Scan returns an iterator that yields key-value pairs in sorted order from start to end.
// It finds the leaf containing start (or the leftmost leaf if start is nil), then walks
// the right-sibling pointer chain. The iterator returns io.EOF once it passes end or
// exhausts all leaf pages.
func (t *BTree) Scan(start []byte, end []byte) (Iterator, error) {
	startID, _, err := t.findLeaf(start)
	if err != nil {
		return nil, err
	}
	_, startNode, err := t.pool.fetchNode(startID)
	if err != nil {
		return nil, err
	}

	var slot int
	if start == nil {
		slot = 0
	} else {
		slot = binarySearchKeys(startNode, start, true)
	}

	return &leafIterator{pool: t.pool, pageID: startID, node: startNode, slot: slot, end: end, closed: false}, nil
}

// finds the leaf for inserting given a specific key
func (t *BTree) findLeaf(key []byte) (leafPageID uint32, path []uint32, err error) {
	return t.findLeafRecursive(t.rootPageID, []uint32{}, key)
}

// recursively traverses through the tree to find where to insert the key
func (t *BTree) findLeafRecursive(pageNum uint32, curr []uint32, key []byte) (leafPageID uint32, path []uint32, err error) {
	_, node, err := t.pool.fetchNode(pageNum)
	if err != nil {
		return pageNum, curr, err
	}
	if node.nodeType == NodeLeaf {
		t.pool.UnpinPage(pageNum, false)
		return pageNum, curr, nil
	} else {
		insert := binarySearchKeys(node, key, false)
		childID := node.childPageID(insert)
		curr = append(curr, pageNum)
		t.pool.UnpinPage(pageNum, false)
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

// We dont want pages to overflow so in case it does we split it into 2 pages here
func (t *BTree) splitLeaf(page *Page, node *Node, path []uint32) ([]byte, uint32, error) {
	currRightSibling := node.rightSibling()
	pageID, err := t.pool.AllocatePage()
	if err != nil {
		return nil, 0, err
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
		return nil, 0, err
	}
	rightNode.setRightSibling(currRightSibling)
	for i := midpoint; i < int(count); i++ {
		rightNode.insertLeafEntry(keys[i], values[i], int(i-midpoint))
	}

	// set the new page as a child of the oldone
	node.setRightSibling(pageID)

	// write both pages
	// snapshot the median before copy-back overwrites the buffer keys[] points into
	median := append([]byte(nil), keys[midpoint]...)
	copy(page.Data, node.data)
	newPage, err := t.pool.FetchPage(pageID)
	if err != nil {
		return nil, 0, err
	}
	copy(newPage.Data, rightHalf)
	t.pool.UnpinPage(pageID, true)

	if err := t.pushUp(median, pageID, path); err != nil {
		return nil, 0, err
	}
	return median, pageID, nil
}

// splitInternal splits a full internal node when a child split needs to insert a new
// separator (newKey, newChild) into it, pushing the median key up to the parent.
func (t *BTree) splitInternal(page *Page, node *Node, newKey []byte, newChild uint32, path []uint32) error {
	rightPageID, err := t.pool.AllocatePage()
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
	// snapshot the median before copy-back overwrites the buffer keys[] points into
	median := append([]byte(nil), keys[midpoint]...)
	copy(page.Data, node.data)
	newPage, err := t.pool.FetchPage(rightPageID)
	if err != nil {
		return err
	}
	copy(newPage.Data, rightHalf)
	t.pool.UnpinPage(rightPageID, true)

	return t.pushUp(median, rightPageID, path[:len(path)-1])

}

// pushUp inserts the separator medianKey and its new right child into the parent, allocating
// a new root if the split propagated past the top of the tree.
func (t *BTree) pushUp(medianKey []byte, rightPageID uint32, path []uint32) error {
	if len(path) == 0 {
		// root leaf was split
		rootID, err := t.pool.AllocatePage()
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

		newPage, err := t.pool.FetchPage(rootID)
		if err != nil {
			return err
		}
		copy(newPage.Data, root)
		t.pool.UnpinPage(rootID, true)
		t.rootPageID = rootID

	} else {
		parentID := path[len(path)-1]
		parentPage, parentNode, err := t.pool.fetchNode(parentID)
		if err != nil {
			return err
		}

		needed := (len(medianKey) + 4 + 4)
		available := int(parentNode.findFreeSpace()) - (5 + (int(parentNode.keyCount())+1)*4 + int(parentNode.keyCount())*4)
		if needed > available {
			if err := t.splitInternal(parentPage, parentNode, medianKey, rightPageID, path); err != nil {
				t.pool.UnpinPage(parentID, true)
				return err
			}
		} else {
			index := binarySearchKeys(parentNode, medianKey, false)
			parentNode.insertInternalEntry(medianKey, index, rightPageID)
		}
		t.pool.UnpinPage(parentID, true)

	}

	return nil
}

// rebalanceLeaf repairs an underflowing (non-root) leaf by either merging it with an
// adjacent sibling (when they fit in one page) or borrowing a single entry from one.
// It fixes the parent's separator/child pointers. The caller (Delete) owns and unpins
// `leaf`; rebalanceLeaf unpins the parent and sibling it fetches.
func (t *BTree) rebalanceLeaf(leaf *Node, key []byte, path []uint32) error {
	parentID := path[len(path)-1]
	_, parent, err := t.pool.fetchNode(parentID)
	if err != nil {
		return err
	}
	defer t.pool.UnpinPage(parentID, true)

	childIndex := binarySearchKeys(parent, key, false)

	hasLeft := childIndex > 0
	hasRight := childIndex < int(parent.keyCount())
	var left, right *Node
	sizeLeft, sizeRight := 0, 0
	leftDirty := false
	if hasLeft {
		leftParent := parent.childPageID(childIndex - 1)
		_, left, err = t.pool.fetchNode(leftParent)
		if err != nil {
			return err
		}
		last := left.keyCount() - 1
		sizeLeft = len(left.leafKey(int(last))) + len(left.leafValue(int(last))) + 8
		defer func() { t.pool.UnpinPage(leftParent, leftDirty) }()
	}
	rightDirty := false
	if hasRight {
		rightParent := parent.childPageID(childIndex + 1)
		_, right, err = t.pool.fetchNode(rightParent)
		if err != nil {
			return err
		}
		sizeRight = len(right.leafKey(0)) + len(right.leafValue(0)) + 8
		defer func() { t.pool.UnpinPage(rightParent, rightDirty) }()
	}

	canLendLeft := hasLeft && left.leafLiveBytes()-sizeLeft >= PageSize/2
	canLendRight := hasRight && right.leafLiveBytes()-sizeRight >= PageSize/2

	// fancy smancy way to make an increasing integer struct :)
	const (
		actNone = iota
		actLeft
		actRight
	)
	borrowFrom := actNone
	mergeWith := actNone
	if canLendLeft || canLendRight {
		switch {
		case canLendLeft && canLendRight:
			if right.leafLiveBytes() > left.leafLiveBytes() {
				borrowFrom = actRight
			} else {
				borrowFrom = actLeft
			}
		case canLendLeft:
			borrowFrom = actLeft
		default:
			borrowFrom = actRight
		}
	} else {
		mergeLeftFits := hasLeft && leaf.leafLiveBytes()+left.leafLiveBytes()-slotStart <= PageSize
		mergeRightFits := hasRight && leaf.leafLiveBytes()+right.leafLiveBytes()-slotStart <= PageSize
		switch {
		case mergeLeftFits:
			mergeWith = actLeft
		case mergeRightFits:
			mergeWith = actRight
		case hasLeft && (!hasRight || left.leafLiveBytes() >= right.leafLiveBytes()):
			borrowFrom = actLeft
		default:
			borrowFrom = actRight
		}
	}

	if borrowFrom == actRight {
		bufferKey := append([]byte(nil), right.leafKey(0)...)
		bufferValue := append([]byte(nil), right.leafValue(0)...)

		if err := leaf.compactLeaf(); err != nil {
			return err
		}
		right.deleteLeafEntry(0)
		leaf.insertLeafEntry(bufferKey, bufferValue, int(leaf.keyCount()))
		parent.replaceInternalKey(childIndex, right.leafKey(0))
		rightDirty = true
		return nil
	}
	if borrowFrom == actLeft {
		last := left.keyCount() - 1
		bufferKey := append([]byte(nil), left.leafKey(int(last))...)
		bufferValue := append([]byte(nil), left.leafValue(int(last))...)

		if err := leaf.compactLeaf(); err != nil {
			return err
		}
		left.deleteLeafEntry(int(last))
		leaf.insertLeafEntry(bufferKey, bufferValue, 0)
		parent.replaceInternalKey(childIndex-1, bufferKey)
		leftDirty = true
		return nil
	}

	// Merge
	if mergeWith == actLeft {
		var keys, values [][]byte
		for i := 0; i < int(left.keyCount()); i++ {
			keys = append(keys, append([]byte(nil), left.leafKey(i)...))
			values = append(values, append([]byte(nil), left.leafValue(i)...))
		}
		for i := 0; i < int(leaf.keyCount()); i++ {
			keys = append(keys, append([]byte(nil), leaf.leafKey(i)...))
			values = append(values, append([]byte(nil), leaf.leafValue(i)...))
		}
		if err := left.rebuildLeaf(keys, values); err != nil {
			return err
		}
		left.setRightSibling(leaf.rightSibling())
		parent.deleteInternalEntry(childIndex-1, childIndex)
		leftDirty = true
		// TODO: FreePage(leaf) once the freelist exists
	} else {
		var keys, values [][]byte
		for i := 0; i < int(leaf.keyCount()); i++ {
			keys = append(keys, append([]byte(nil), leaf.leafKey(i)...))
			values = append(values, append([]byte(nil), leaf.leafValue(i)...))
		}
		for i := 0; i < int(right.keyCount()); i++ {
			keys = append(keys, append([]byte(nil), right.leafKey(i)...))
			values = append(values, append([]byte(nil), right.leafValue(i)...))
		}
		if err := leaf.rebuildLeaf(keys, values); err != nil {
			return err
		}
		leaf.setRightSibling(right.rightSibling())
		parent.deleteInternalEntry(childIndex, childIndex+1)
		rightDirty = true
		// TODO: FreePage(right) once the freelist exists
	}

	// The merge removed a separator from the parent, which may now underflow.
	if len(path) == 1 {
		// parent is the root: only act if it has collapsed to a single child.
		if parent.keyCount() == 0 {
			t.rootPageID = parent.childPageID(0)
			// TODO: FreePage(old root) once the freelist exists
		}
		return nil
	}
	if parent.internalLiveBytes() < PageSize/2 {
		return t.rebalanceInternal(parent, key, path[:len(path)-1])
	}
	return nil

}

// rebalanceInternal repairs an underflowing (non-root) internal node. It rotates a key
// through the parent from a sibling that can spare one, or, if neither can, merges with a
// sibling and pulls the parent separator down between them. A merge shrinks the parent,
// which may itself underflow (recurse up) or, if it is the root, collapse a level off the
// tree. The caller owns and unpins `node`; this function unpins the parent and the siblings
// it fetches. `path` holds the ancestors above `node`, so path[last] is node's parent.
func (t *BTree) rebalanceInternal(node *Node, key []byte, path []uint32) error {
	parentID := path[len(path)-1]
	_, parent, err := t.pool.fetchNode(parentID)
	if err != nil {
		return err
	}
	defer t.pool.UnpinPage(parentID, true)

	childIndex := binarySearchKeys(parent, key, false)

	hasLeft := childIndex > 0
	hasRight := childIndex < int(parent.keyCount())

	var left, right *Node
	leftDirty := false
	rightDirty := false
	if hasLeft {
		leftID := parent.childPageID(childIndex - 1)
		_, left, err = t.pool.fetchNode(leftID)
		if err != nil {
			return err
		}
		defer func() { t.pool.UnpinPage(leftID, leftDirty) }()
	}
	if hasRight {
		rightID := parent.childPageID(childIndex + 1)
		_, right, err = t.pool.fetchNode(rightID)
		if err != nil {
			return err
		}
		defer func() { t.pool.UnpinPage(rightID, rightDirty) }()
	}

	if !hasLeft && !hasRight {
		return nil
	}

	// A sibling can lend if its at least half full after all of the losses
	canLendLeft := false
	if hasLeft {
		boundary := len(left.internalKey(int(left.keyCount())-1)) + 8
		canLendLeft = left.internalLiveBytes()-boundary >= PageSize/2
	}
	canLendRight := false
	if hasRight {
		boundary := len(right.internalKey(0)) + 8
		canLendRight = right.internalLiveBytes()-boundary >= PageSize/2
	}

	// fancy smancy again
	const (
		actNone = iota
		actLeft
		actRight
	)
	rotateFrom := actNone
	mergeWith := actNone
	if canLendLeft || canLendRight {
		if canLendRight && (!canLendLeft || right.internalLiveBytes() > left.internalLiveBytes()) {
			rotateFrom = actRight
		} else {
			rotateFrom = actLeft
		}
	} else {
		mergeLeftFits := hasLeft && left.internalLiveBytes()+node.internalLiveBytes()+len(parent.internalKey(childIndex-1))-1 <= PageSize
		mergeRightFits := hasRight && node.internalLiveBytes()+right.internalLiveBytes()+len(parent.internalKey(childIndex))-1 <= PageSize
		switch {
		case mergeLeftFits:
			mergeWith = actLeft
		case mergeRightFits:
			mergeWith = actRight
		case hasRight && (!hasLeft || right.internalLiveBytes() >= left.internalLiveBytes()):
			rotateFrom = actRight
		default:
			rotateFrom = actLeft
		}
	}

	if rotateFrom == actRight {
		separator := append([]byte(nil), parent.internalKey(childIndex)...)
		ascending := append([]byte(nil), right.internalKey(0)...)
		movedChild := right.childPageID(0)

		keys := make([][]byte, 0, int(node.keyCount())+1)
		children := make([]uint32, 0, int(node.keyCount())+2)
		for i := 0; i < int(node.keyCount()); i++ {
			keys = append(keys, append([]byte(nil), node.internalKey(i)...))
		}
		keys = append(keys, separator)
		for i := 0; i <= int(node.keyCount()); i++ {
			children = append(children, node.childPageID(i))
		}
		children = append(children, movedChild)
		if err := node.rebuildInternal(keys, children); err != nil {
			return err
		}

		parent.replaceInternalKey(childIndex, ascending)
		right.deleteInternalEntry(0, 0)
		rightDirty = true
		return nil
	}
	if rotateFrom == actLeft {
		separator := append([]byte(nil), parent.internalKey(childIndex-1)...)
		lastKey := int(left.keyCount()) - 1
		ascending := append([]byte(nil), left.internalKey(lastKey)...)
		movedChild := left.childPageID(int(left.keyCount()))

		keys := make([][]byte, 0, int(node.keyCount())+1)
		children := make([]uint32, 0, int(node.keyCount())+2)
		keys = append(keys, separator)
		for i := 0; i < int(node.keyCount()); i++ {
			keys = append(keys, append([]byte(nil), node.internalKey(i)...))
		}
		children = append(children, movedChild)
		for i := 0; i <= int(node.keyCount()); i++ {
			children = append(children, node.childPageID(i))
		}
		if err := node.rebuildInternal(keys, children); err != nil {
			return err
		}

		parent.replaceInternalKey(childIndex-1, ascending)
		left.deleteInternalEntry(lastKey, int(left.keyCount()))
		leftDirty = true
		return nil
	}

	// Merge
	if mergeWith == actLeft {
		// Merge node into left; left survives, node is orphaned.
		separator := append([]byte(nil), parent.internalKey(childIndex-1)...)
		keys := make([][]byte, 0)
		children := make([]uint32, 0)
		for i := 0; i < int(left.keyCount()); i++ {
			keys = append(keys, append([]byte(nil), left.internalKey(i)...))
		}
		keys = append(keys, separator)
		for i := 0; i < int(node.keyCount()); i++ {
			keys = append(keys, append([]byte(nil), node.internalKey(i)...))
		}
		for i := 0; i <= int(left.keyCount()); i++ {
			children = append(children, left.childPageID(i))
		}
		for i := 0; i <= int(node.keyCount()); i++ {
			children = append(children, node.childPageID(i))
		}
		if err := left.rebuildInternal(keys, children); err != nil {
			return err
		}
		parent.deleteInternalEntry(childIndex-1, childIndex)
		leftDirty = true
		// TODO: FreePage(node.pageID) once the freelist exists
	} else {
		// Merge right into node; node survives, right is orphaned.
		separator := append([]byte(nil), parent.internalKey(childIndex)...)
		keys := make([][]byte, 0)
		children := make([]uint32, 0)
		for i := 0; i < int(node.keyCount()); i++ {
			keys = append(keys, append([]byte(nil), node.internalKey(i)...))
		}
		keys = append(keys, separator)
		for i := 0; i < int(right.keyCount()); i++ {
			keys = append(keys, append([]byte(nil), right.internalKey(i)...))
		}
		for i := 0; i <= int(node.keyCount()); i++ {
			children = append(children, node.childPageID(i))
		}
		for i := 0; i <= int(right.keyCount()); i++ {
			children = append(children, right.childPageID(i))
		}
		if err := node.rebuildInternal(keys, children); err != nil {
			return err
		}
		parent.deleteInternalEntry(childIndex, childIndex+1)
		rightDirty = true
		// TODO: FreePage(rightID) once the freelist exists
	}

	// The merge removed a separator from the parent, which may now underflow.
	if len(path) == 1 {
		// parent is the root: only act if it has collapsed to a single child.
		if parent.keyCount() == 0 {
			t.rootPageID = parent.childPageID(0)
			// TODO: FreePage(old root) once the freelist exists
		}
		return nil
	}
	if parent.internalLiveBytes() < PageSize/2 {
		return t.rebalanceInternal(parent, key, path[:len(path)-1])
	}
	return nil
}
