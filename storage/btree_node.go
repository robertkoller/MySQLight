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

// decodeNode reads the node type byte at offset 0 of the raw page data and returns
// a Node wrapping those bytes. It returns an error if the type byte is not a known NodeType.
func decodeNode(pageID uint32, data []byte) (*Node, error) {
	nodeType := data[0]
	if nodeType != byte(NodeLeaf) && nodeType != byte(NodeInternal) {
		return nil, errors.New("Data is not a NodeLeaf or NodeInternal")
	}

	return &Node{pageID, NodeType(nodeType), data}, nil
}

// keyCount decodes the two-byte big-endian uint16 at offset 1, which stores the number
// of keys currently held in this node.
func (n *Node) keyCount() uint16 {
	count := binary.BigEndian.Uint16(n.data[1:])
	return count
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

// Increments the keyCount
func (n *Node) incrementKeyCount() {
	binary.BigEndian.PutUint16(n.data[1:], n.keyCount()+1)
}

func (n *Node) isLeaf() bool {
	return n.nodeType == NodeLeaf
}

// creates the header for a new leaf node
func makeNewLeafHeader() []byte {
	header := make([]byte, PageSize)
	header[0] = byte(NodeLeaf)
	binary.BigEndian.PutUint16(header[1:], 0)
	binary.BigEndian.PutUint16(header[3:], PageSize)

	return header

}

func makeNewInternalHeader() []byte {
	header := make([]byte, PageSize)
	header[0] = byte(NodeInternal)
	binary.BigEndian.PutUint16(header[1:], 0)
	binary.BigEndian.PutUint16(header[3:], PageSize)

	return header

}

// Inserts a key and value entry into the page
func (n *Node) insertLeafEntry(key, value []byte, slotIndex int) {
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

// rightSibling decodes the four-byte big-endian uint32 at offset 5, which is the page ID
// of the next leaf in the sorted chain. A value of zero means this is the rightmost leaf.
func (n *Node) rightSibling() uint32 {
	return binary.BigEndian.Uint32(n.data[5:])
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

// childPageID decodes the uint32 child page ID at position i within the internal node.
// Child page IDs are stored starting at byte offset 5, each taking four bytes, so child i
// is at offset 5 + i*4.
func (n *Node) childPageID(i int) uint32 {
	return binary.BigEndian.Uint32(n.data[5+(i*4):])
}

// Sets the right sibling
func (n *Node) setRightSibling(pageID uint32) {
	binary.BigEndian.PutUint32(n.data[5:], pageID)
}
