package storage

import (
	"encoding/binary"
	"errors"
)

// Page layout constants — byte offsets within a 4096-byte page.
// Leaf node:
//   [0]      nodeType (1 byte)
//   [1–2]    keyCount (2 bytes, uint16)
//   [3-4]    freeSpacePointer showing where keys end
//   [5–8]    rightSiblingPageID (4 bytes, uint32)
//   [9–...]  slot array: keyCount × (keyOffset uint16, keylen uint16, valueOffset uint16, valuelen uint16)
//   data grows inward from both ends; keys from low end, values from high end
//
// Internal node:
//   [0]      nodeType (1 byte)
//   [1–2]    keyCount (2 bytes, uint16) // this is a different key, its the keys you use for searching throught the b tree
//   [3-4]    freeSpacePointer
//   [5–...]  child page IDs: (keyCount+1) × uint32
//   then key slot array: keyCount × (keyOffset uint16, KeyLen uint16)

type NodeType uint8

const (
	NodeLeaf     NodeType = 0x01
	NodeInternal NodeType = 0x02
)

const slotStart = 9

type Node struct {
	pageID   uint32
	nodeType NodeType
	data     []byte // raw page bytes (PageSize)
}

// creates the header for a new leaf node
func makeNewLeafHeader() []byte {
	header := make([]byte, PageSize)
	header[0] = byte(NodeLeaf)
	binary.BigEndian.PutUint16(header[1:], 0)
	binary.BigEndian.PutUint16(header[3:], PageSize)

	return header

}

// creates the header for a new internal node
func makeNewInternalHeader() []byte {
	header := make([]byte, PageSize)
	header[0] = byte(NodeInternal)
	binary.BigEndian.PutUint16(header[1:], 0)
	binary.BigEndian.PutUint16(header[3:], PageSize)

	return header

}

// decodeNode reads the node type byte at offset 0 of the raw page data and returns
// a Node wrapping those bytes. It returns an error if the type byte is not a known NodeType.
func decodeNode(pageID uint32, data []byte) (*Node, error) {
	nodeType := data[0]
	if nodeType != byte(NodeLeaf) && nodeType != byte(NodeInternal) {
		return nil, errors.New("Data is not a NodeLeaf or NodeInternal")
	}

	return &Node{pageID, NodeType(nodeType), data}, nil
}

// isLeaf reports whether this node is a leaf.
func (n *Node) isLeaf() bool {
	return n.nodeType == NodeLeaf
}

// keyCount decodes the two-byte big-endian uint16 at offset 1, which stores the number
// of keys currently held in this node.
func (n *Node) keyCount() uint16 {
	count := binary.BigEndian.Uint16(n.data[1:])
	return count
}

// Increments the keyCount
func (n *Node) incrementKeyCount() {
	binary.BigEndian.PutUint16(n.data[1:], n.keyCount()+1)
}

// Decrements the keyCount
func (n *Node) decrementKeyCount() {
	binary.BigEndian.PutUint16(n.data[1:], n.keyCount()-1)
}

// Reads the freeSpacePtr
func (n *Node) findFreeSpace() uint16 {
	freeSpacePointer := binary.BigEndian.Uint16(n.data[3:])
	return freeSpacePointer
}

// alters the freeSpacePtr
func (n *Node) newFreeSpace(newPtr uint16) {
	binary.BigEndian.PutUint16(n.data[3:], newPtr)
}

// leafKey reads the key offset for slot i from the leaf's slot array and returns the
// key bytes at that position within the page.
func (n *Node) leafKey(i int) []byte {
	offset := slotStart + (i * 8)
	keyOffset := binary.BigEndian.Uint16(n.data[offset:])
	keyLen := binary.BigEndian.Uint16(n.data[offset+2:])
	return n.data[keyOffset : keyOffset+keyLen]
}

// leafValue reads the value offset for slot i from the leaf's slot array and returns
// the value bytes at that position within the page.
func (n *Node) leafValue(i int) []byte {
	offset := slotStart + (i * 8) + 4
	valueOffset := binary.BigEndian.Uint16(n.data[offset:])
	valueLen := binary.BigEndian.Uint16(n.data[offset+2:])
	return n.data[valueOffset : valueOffset+valueLen]
}

// rightSibling decodes the pageID for the next leaf in the chain. a 0 value means this is the
// // rightmost leaf
func (n *Node) rightSibling() uint32 {
	return binary.BigEndian.Uint32(n.data[5:])
}

// Sets the right sibling
func (n *Node) setRightSibling(pageID uint32) {
	binary.BigEndian.PutUint32(n.data[5:], pageID)
}

// Inserts a key and value entry into the page
func (n *Node) insertLeafEntry(key []byte, value []byte, slotIndex int) {
	insertValue := n.findFreeSpace() - uint16(len(value))
	copy(n.data[insertValue:], value)
	insertKey := insertValue - uint16(len(key))
	copy(n.data[insertKey:], key)
	n.newFreeSpace(insertKey)

	slotInsert := make([]byte, 8)
	binary.BigEndian.PutUint16(slotInsert[0:], insertKey)
	binary.BigEndian.PutUint16(slotInsert[2:], uint16(len(key)))
	binary.BigEndian.PutUint16(slotInsert[4:], insertValue)
	binary.BigEndian.PutUint16(slotInsert[6:], uint16(len(value)))

	slotByte := slotStart + slotIndex*8
	copy(n.data[slotByte+8:], n.data[slotByte:slotStart+int(n.keyCount())*8])
	copy(n.data[slotByte:], slotInsert)
	n.incrementKeyCount()
}

// Delete a leaf entry by shifting things down over it
func (n *Node) deleteLeafEntry(slotIndex int) {
	slotByte := slotStart + slotIndex*8
	copy(n.data[slotByte:], n.data[slotByte+8:slotStart+int(n.keyCount()*8)])
	n.decrementKeyCount()
}

// childPageID decodes the uint32 child page ID at position i within the internal node.
// Child page IDs are stored starting at byte offset 5, each taking four bytes, so child i
// is at offset 5 + i*4.
func (n *Node) childPageID(i int) uint32 {
	return binary.BigEndian.Uint32(n.data[5+(i*4):])
}

// internalKey locates the key slot array, which starts after the child page ID section,
// and returns the key bytes at slot i.
func (n *Node) internalKey(i int) []byte {
	offset := 5 + ((n.keyCount() + 1) * 4) // this gets us to the key slots
	offsetI := offset + uint16((i * 4))    // this gets us to where the key is
	keyOffset := binary.BigEndian.Uint16(n.data[offsetI:])
	keyLen := binary.BigEndian.Uint16(n.data[offsetI+2:])

	return n.data[keyOffset : keyOffset+keyLen]
}

// insertInternalEntry inserts a separator key at slotIndex and its new right child at child
// position slotIndex+1. Because adding a child pointer pushes the key slot array right by 4
// bytes, the existing key slots are physically shifted before the new slot is written.
func (n *Node) insertInternalEntry(key []byte, slotIndex int, rightChildID uint32) {
	insertKeyIndex := n.findFreeSpace() - uint16(len(key))
	slotsBase := 5 + (n.keyCount()+1)*4

	copy(n.data[int(slotsBase)+4+(slotIndex+1)*4:], n.data[slotsBase+uint16(slotIndex*4):slotsBase+n.keyCount()*4]) // move everything after insert by 8
	copy(n.data[slotsBase+4:slotsBase+4+uint16(slotIndex)*4], n.data[slotsBase:slotsBase+uint16(slotIndex)*4])      // move stuff before the insert 4 right
	copy(n.data[5+(slotIndex+2)*4:], n.data[5+(slotIndex+1)*4:5+(n.keyCount()+1)*4])                                //shift to make space for rightChildID

	childSlot := make([]byte, 4)
	binary.BigEndian.PutUint32(childSlot, rightChildID)
	copy(n.data[5+(slotIndex+1)*4:], childSlot) // insert rightChildID

	keySlot := make([]byte, 4)
	binary.BigEndian.PutUint16(keySlot[0:], insertKeyIndex)
	binary.BigEndian.PutUint16(keySlot[2:], uint16(len(key)))
	copy(n.data[slotsBase+4+uint16(slotIndex*4):], keySlot) // copy the reference to key in
	copy(n.data[insertKeyIndex:], key)                      // copy the actual key in

	n.newFreeSpace(insertKeyIndex)
	n.incrementKeyCount()
}

// deletes internal entry and shifts stuff down
func (n *Node) deleteInternalEntry(seperatorKeyIndex int, childIndex int) error {
	oldCount := int(n.keyCount())
	var keys [][]byte
	var children []uint32
	for i := 0; i < oldCount; i++ {
		if i != seperatorKeyIndex {
			keys = append(keys, n.internalKey(i))
		}
		if i != childIndex {
			children = append(children, n.childPageID(i))
		}
	}
	if oldCount != childIndex {
		children = append(children, n.childPageID(oldCount))
	}

	temp := makeNewInternalHeader()
	tempNode, err := decodeNode(n.pageID, temp)
	if err != nil {
		return err
	}

	binary.BigEndian.PutUint32(temp[5:], children[0])
	for i := 0; i < len(keys); i++ {
		tempNode.insertInternalEntry(keys[i], i, children[i+1])
	}
	copy(n.data, temp)
	return nil
}

// replaceInternalKey replaces separator key i with newKey. It rebuilds the page rather than
// writing the new key below the free-space pointer in place: the free-space pointer only ever
// moves down, so repeated in-place replacements (one per leaf-borrow and rotation) would leak
// the old keys' bytes and eventually march the pointer into the child/slot region, corrupting
// the node. Rebuilding compacts the live keys on every call.
func (n *Node) replaceInternalKey(i int, newKey []byte) {
	keys := make([][]byte, 0, int(n.keyCount()))
	for j := 0; j < int(n.keyCount()); j++ {
		if j == i {
			keys = append(keys, append([]byte(nil), newKey...))
		} else {
			keys = append(keys, append([]byte(nil), n.internalKey(j)...))
		}
	}
	children := make([]uint32, 0, int(n.keyCount())+1)
	for j := 0; j <= int(n.keyCount()); j++ {
		children = append(children, n.childPageID(j))
	}
	n.rebuildInternal(keys, children)
}

// Returns the number bytes the leaf is using (storage amount)
func (n *Node) leafLiveBytes() int {
	total := slotStart + int(n.keyCount())*8
	for i := 0; i < int(n.keyCount()); i++ {
		total += len(n.leafKey(i)) + len(n.leafValue(i))
	}
	return total
}

// internalLiveBytes returns how many bytes this internal node is using
func (n *Node) internalLiveBytes() int {
	total := 5 + (int(n.keyCount())+1)*4 + int(n.keyCount())*4
	for i := 0; i < int(n.keyCount()); i++ {
		total += len(n.internalKey(i))
	}
	return total
}

// rebuildLeaf rewrites this leaf's page to hold exactly the given key/value pairs, packed
// with no dead space. this was an issue when deleting as we would have leftover space
func (n *Node) rebuildLeaf(keys [][]byte, values [][]byte) error {
	sibling := n.rightSibling()
	temp := makeNewLeafHeader()
	tempNode, err := decodeNode(n.pageID, temp)
	if err != nil {
		return err
	}
	for i := 0; i < len(keys); i++ {
		tempNode.insertLeafEntry(keys[i], values[i], i)
	}
	copy(n.data, temp)
	n.setRightSibling(sibling)
	return nil
}

// compactLeaf rewrites the leaf in place from its own live entries, reclaiming the dead
// bytes left behind by deleteLeafEntry
func (n *Node) compactLeaf() error {
	keys := make([][]byte, 0, int(n.keyCount()))
	values := make([][]byte, 0, int(n.keyCount()))
	for i := 0; i < int(n.keyCount()); i++ {
		keys = append(keys, append([]byte(nil), n.leafKey(i)...))
		values = append(values, append([]byte(nil), n.leafValue(i)...))
	}
	return n.rebuildLeaf(keys, values)
}

// rebuildInternal overwrites this internal node's page so it holds exactly the given
// separator keys and child page IDs
func (n *Node) rebuildInternal(keys [][]byte, children []uint32) error {
	temp := makeNewInternalHeader()
	tempNode, err := decodeNode(n.pageID, temp)
	if err != nil {
		return err
	}
	binary.BigEndian.PutUint32(temp[5:], children[0])
	for i := 0; i < len(keys); i++ {
		tempNode.insertInternalEntry(keys[i], i, children[i+1])
	}
	copy(n.data, temp)
	return nil
}
