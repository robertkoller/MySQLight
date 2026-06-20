package txn

type TxnState uint8

const (
	TxnActive    TxnState = iota
	TxnCommitted TxnState = iota
	TxnAborted   TxnState = iota
)

type UndoEntry struct {
	// TODO: pageID      uint32
	// TODO: beforeImage []byte // snapshot of the page before this transaction modified it
}

type Txn struct {
	ID      uint64
	State   TxnState
	undoLog []UndoEntry
}

// TxnManager creates and tracks active transactions.
type TxnManager struct {
	// TODO: nextID   uint64
	// TODO: active   map[uint64]*Txn
	// TODO: lockMgr  *LockManager
	// TODO: wal      reference to the WAL for writing BEGIN/COMMIT/ABORT records
}

func NewTxnManager() *TxnManager {
	// TODO: initialise nextID=1, active map, lock manager
	panic("not implemented")
}

func (m *TxnManager) Begin() (*Txn, error) {
	// TODO: allocate a new Txn with ID=nextID, State=TxnActive
	// TODO: write a RecordBegin to the WAL
	// TODO: add to m.active map; increment nextID
	panic("not implemented")
}

func (m *TxnManager) Commit(txn *Txn) error {
	// TODO: return error if txn.State != TxnActive
	// TODO: write a RecordCommit to the WAL
	// TODO: release all locks held by this transaction via the lock manager
	// TODO: set txn.State = TxnCommitted, remove from m.active
	panic("not implemented")
}

func (m *TxnManager) Rollback(txn *Txn) error {
	// TODO: return error if txn.State != TxnActive
	// TODO: apply undo log entries in reverse order (restore before-images to the pager)
	// TODO: write a RecordAbort to the WAL
	// TODO: release all locks held by this transaction
	// TODO: set txn.State = TxnAborted, remove from m.active
	panic("not implemented")
}
