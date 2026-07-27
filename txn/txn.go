package txn

import (
	"errors"

	"github.com/robertkoller/MySQLight/storage"
	"github.com/robertkoller/MySQLight/wal"
)

type TxnState uint8

const (
	TxnActive    TxnState = iota
	TxnCommitted TxnState = iota
	TxnAborted   TxnState = iota
)

type Txn struct {
	ID    uint64
	State TxnState
}

// TxnManager creates and tracks active transactions.
type TxnManager struct {
	nextID  uint64
	active  map[uint64]*Txn
	lockMgr *LockManager
	wal     *wal.WAL
	pool    *storage.BufferPool
}

// NewTxnManager initialises the transaction manager with a starting transaction ID of one,
// an empty active transaction map, and a new lock manager.
func NewTxnManager(walFile *wal.WAL, pool *storage.BufferPool) *TxnManager {
	return &TxnManager{
		nextID:  1,
		active:  make(map[uint64]*Txn),
		lockMgr: NewLockManager(),
		wal:     walFile,
		pool:    pool,
	}
}

// LockManager returns the manager's lock manager so callers can acquire table locks.
func (m *TxnManager) LockManager() *LockManager {
	return m.lockMgr
}

// Begin allocates a new transaction with a unique ID and TxnActive state, writes a
// RecordBegin to the WAL, and registers the transaction in the active map.
func (m *TxnManager) Begin() (*Txn, error) {
	txnObj := &Txn{ID: m.nextID, State: TxnActive}
	if _, err := m.wal.WriteRecord(&wal.Record{Type: wal.RecordBegin, TxnID: txnObj.ID}); err != nil {
		return nil, err
	}
	m.active[txnObj.ID] = txnObj
	m.nextID++
	return txnObj, nil
}

// Commit validates that the transaction is still active, writes a RecordCommit to the WAL,
// releases all locks the transaction holds via the lock manager, marks the transaction as
// committed, and removes it from the active map.
func (m *TxnManager) Commit(txnObj *Txn) error {
	if txnObj.State != TxnActive {
		return errors.New("transaction is not active")
	}
	if _, err := m.wal.WriteRecord(&wal.Record{Type: wal.RecordCommit, TxnID: txnObj.ID}); err != nil {
		return err
	}
	m.lockMgr.ReleaseAll(txnObj.ID)
	txnObj.State = TxnCommitted
	delete(m.active, txnObj.ID)
	return nil
}

// Rollback validates that the transaction is still active, uses the buffer pool to restore
// all page before-images the transaction modified, writes a RecordAbort to the WAL, releases
// all locks, and marks the transaction as aborted.
func (m *TxnManager) Rollback(txnObj *Txn) error {
	if txnObj.State != TxnActive {
		return errors.New("transaction is not active")
	}
	if err := m.pool.RollbackTxn(txnObj.ID); err != nil {
		return err
	}
	if _, err := m.wal.WriteRecord(&wal.Record{Type: wal.RecordAbort, TxnID: txnObj.ID}); err != nil {
		return err
	}
	m.lockMgr.ReleaseAll(txnObj.ID)
	txnObj.State = TxnAborted
	delete(m.active, txnObj.ID)
	return nil
}
