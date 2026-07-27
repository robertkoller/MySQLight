package executor

import (
	"github.com/robertkoller/MySQLight/catalog"
	"github.com/robertkoller/MySQLight/storage"
	"github.com/robertkoller/MySQLight/txn"
)

// TableScan iterates every row in a table's B+ tree leaf chain.
type TableScan struct {
	tree      *storage.BTree
	columns   []catalog.ColumnDef
	iterator  storage.Iterator
	txnID     uint64
	tableName string
	lockMgr   *txn.LockManager // nil means no locking
}

// NewTableScan stores the tree and column definitions. The scan does not begin until Open is called.
func NewTableScan(tree *storage.BTree, columns []catalog.ColumnDef) *TableScan {
	return &TableScan{tree: tree, columns: columns}
}

// WithLock attaches transaction lock context to the scan. When set, Open acquires a shared
// table lock and the lock is held until the transaction commits or rolls back (strict 2PL).
func (t *TableScan) WithLock(txnID uint64, tableName string, lockMgr *txn.LockManager) *TableScan {
	t.txnID = txnID
	t.tableName = tableName
	t.lockMgr = lockMgr
	return t
}

// Open acquires a shared lock on the table (if a lock manager is set) and starts a full scan.
func (t *TableScan) Open() error {
	if t.lockMgr != nil {
		if err := t.lockMgr.Acquire(t.txnID, t.tableName, txn.LockShared); err != nil {
			return err
		}
	}
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

// Close shuts down the B+ tree iterator. The shared lock is held until commit or rollback
// (strict 2PL) and is not released here.
func (t *TableScan) Close() error {
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
	txnID     uint64
	tableName string
	lockMgr   *txn.LockManager
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

// WithLock attaches transaction lock context to the scan.
func (s *IndexScan) WithLock(txnID uint64, tableName string, lockMgr *txn.LockManager) *IndexScan {
	s.txnID = txnID
	s.tableName = tableName
	s.lockMgr = lockMgr
	return s
}

// Open acquires a shared lock on the table (if a lock manager is set) and starts the range scan.
func (s *IndexScan) Open() error {
	if s.lockMgr != nil {
		if err := s.lockMgr.Acquire(s.txnID, s.tableName, txn.LockShared); err != nil {
			return err
		}
	}
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

// Close shuts down the index iterator. The shared lock is held until commit or rollback.
func (s *IndexScan) Close() error {
	return s.iterator.Close()
}
