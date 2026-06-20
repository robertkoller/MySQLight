package storage

// Page layout constants — byte offsets within a 4096-byte page.
// Leaf node:
//   [0]      nodeType (1 byte)
//   [1–2]    keyCount (2 bytes, uint16)
//   [3–6]    rightSiblingPageID (4 bytes, uint32)
//   [7–...]  slot array: keyCount × (keyOffset uint16, valueOffset uint16)
//   data grows inward from both ends; keys from low end, values from high end
//
// Internal node:
//   [0]      nodeType (1 byte)
//   [1–2]    keyCount (2 bytes, uint16)
//   [3–...]  child page IDs: (keyCount+1) × uint32
//   then key slot array: keyCount × keyOffset uint16

type NodeType uint8

const (
	NodeLeaf     NodeType = 0x01
	NodeInternal NodeType = 0x02
)

type Node struct {
	pageID   uint32
	nodeType NodeType
	data     []byte // raw page bytes (PageSize)
}

// decodeNode reads the node type byte at offset 0 of the raw page data and returns
// a Node wrapping those bytes. It returns an error if the type byte is not a known NodeType.
func decodeNode(pageID uint32, data []byte) (*Node, error) {
	// TODO: read data[0] to get nodeType; reject unknown values
	// TODO: return &Node{pageID, nodeType, data}
	panic("not implemented")
}

// keyCount decodes the two-byte big-endian uint16 at offset 1, which stores the number
// of keys currently held in this node.
func (n *Node) keyCount() uint16 {
	// TODO: decode the 2-byte uint16 at offset 1 (big-endian)
	panic("not implemented")
}

func (n *Node) isLeaf() bool {
	return n.nodeType == NodeLeaf
}

// --- Leaf accessors ---

// leafKey reads the key offset for slot i from the leaf's slot array and returns the
// key bytes at that position within the page.
func (n *Node) leafKey(i int) []byte {
	// TODO: read the keyOffset from the slot array entry i
	// TODO: return the key bytes starting at that offset
	panic("not implemented")
}

// leafValue reads the value offset for slot i from the leaf's slot array and returns
// the value bytes at that position within the page.
func (n *Node) leafValue(i int) []byte {
	// TODO: read the valueOffset from the slot array entry i
	// TODO: return the value bytes starting at that offset
	panic("not implemented")
}

// rightSibling decodes the four-byte big-endian uint32 at offset 3, which is the page ID
// of the next leaf in the sorted chain. A value of zero means this is the rightmost leaf.
func (n *Node) rightSibling() uint32 {
	// TODO: decode the 4-byte uint32 at offset 3 (big-endian)
	panic("not implemented")
}

// --- Internal node accessors ---

// internalKey locates the key slot array, which starts after the child page ID section,
// and returns the key bytes at slot i.
func (n *Node) internalKey(i int) []byte {
	// TODO: locate the key slot array (starts after child page IDs)
	// TODO: return the key bytes at slot i
	panic("not implemented")
}

// childPageID decodes the uint32 child page ID at position i within the internal node.
// Child page IDs are stored starting at byte offset 3, each taking four bytes, so child i
// is at offset 3 + i*4.
func (n *Node) childPageID(i int) uint32 {
	// TODO: decode the uint32 child page ID at position i (offset 3 + i*4)
	panic("not implemented")
}
