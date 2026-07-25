package executor

import (
	"github.com/robertkoller/MySQLight/catalog"
	"github.com/robertkoller/MySQLight/storage"
)

// TableScan iterates every row in a table's B+ tree leaf chain.
type TableScan struct {
	tree     *storage.BTree
	columns  []catalog.ColumnDef
	iterator storage.Iterator
}

// NewTableScan stores the tree and column definitions. The scan does not begin until Open is called.
func NewTableScan(tree *storage.BTree, columns []catalog.ColumnDef) *TableScan {
	return &TableScan{tree: tree, columns: columns}
}

// Open acquires a shared lock on the table and calls btree.Scan(nil, nil) to obtain
// an iterator that will walk all leaf pages in key order.
func (t *TableScan) Open() error {
	// TODO: acquire shared lock on table (Phase 4)
	iterator, err := t.tree.Scan(nil, nil)
	if err != nil {
		return err
	}
	t.iterator = iterator
	return nil
}

// Next calls the B+ tree iterator to get the next raw key-value bytes, deserialises the
// value into a Row using the stored column definitions, and returns it. Returns io.EOF
// when the iterator is exhausted.
func (t *TableScan) Next() (Row, error) {
	_, value, err := t.iterator.Next()
	if err != nil {
		return nil, err
	}
	return decodeRow(value, t.columns)
}

// Close shuts down the B+ tree iterator and releases the shared lock on the table.
func (t *TableScan) Close() error {
	// TODO: release the shared lock (Phase 4)
	return t.iterator.Close()
}

// IndexScan traverses a B+ tree index and fetches matching rows from the data tree by primary key.
type IndexScan struct {
	indexTree *storage.BTree
	dataTree  *storage.BTree
	columns   []catalog.ColumnDef
	start     []byte
	end       []byte
	iterator  storage.Iterator
}

// NewIndexScan stores both trees, column definitions, and the key range to scan.
// The scan does not begin until Open is called.
func NewIndexScan(indexTree *storage.BTree, dataTree *storage.BTree, columns []catalog.ColumnDef, start []byte, end []byte) *IndexScan {
	return &IndexScan{
		indexTree: indexTree,
		dataTree:  dataTree,
		columns:   columns,
		start:     start,
		end:       end,
	}
}

// Open acquires a shared lock on the table and starts a range scan on the index B+ tree
// between start and end.
func (s *IndexScan) Open() error {
	// TODO: acquire shared lock on table (Phase 4)
	iterator, err := s.indexTree.Scan(s.start, s.end)
	if err != nil {
		return err
	}
	s.iterator = iterator
	return nil
}

// Next gets the next index entry, uses its value as a primary key to fetch the full row
// from the data tree, deserialises it, and returns it. Returns io.EOF when exhausted.
func (s *IndexScan) Next() (Row, error) {
	_, primaryKey, err := s.iterator.Next()
	if err != nil {
		return nil, err
	}
	rowBytes, err := s.dataTree.Get(primaryKey)
	if err != nil {
		return nil, err
	}
	return decodeRow(rowBytes, s.columns)
}

// Close shuts down the index iterator and releases the shared lock on the table.
func (s *IndexScan) Close() error {
	// TODO: release shared lock (Phase 4)
	return s.iterator.Close()
}
