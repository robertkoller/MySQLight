package txn

import (
	"errors"

	"github.com/robertkoller/MySQLight/wal"
)

type TxnState uint8

const (
	TxnActive    TxnState = iota
	TxnCommitted TxnState = iota
	TxnAborted   TxnState = iota
)

type UndoEntry struct {
	pageID      uint32
	beforeImage []byte // snapshot of the page before this transaction modified it
}

type Txn struct {
	ID      uint64
	State   TxnState
	undoLog []UndoEntry
}

// TxnManager creates and tracks active transactions.
type TxnManager struct {
	nextID     uint64
	active     map[uint64]*Txn
	lockMgr    *LockManager
	wal        *wal.WAL
	pageWriter wal.PageWriter
}

// NewTxnManager initialises the transaction manager with a starting transaction ID of one,
// an empty active transaction map, and a new lock manager.
func NewTxnManager(wal *wal.WAL, writer wal.PageWriter) *TxnManager {
	return &TxnManager{nextID: 1, active: make(map[uint64]*Txn), lockMgr: NewLockManager(), wal: wal, pageWriter: writer}
}

// Begin allocates a new transaction with a unique ID and TxnActive state, writes a
// RecordBegin to the WAL, and registers the transaction in the active map.
func (m *TxnManager) Begin() (*Txn, error) {
	txn := Txn{ID: m.nextID, State: TxnActive}
	_, err := m.wal.WriteRecord(&wal.Record{Type: wal.RecordBegin, TxnID: txn.ID})
	if err != nil {
		return nil, err
	}

	m.active[txn.ID] = &txn
	m.nextID++

	return &txn, nil
}

// Commit validates that the transaction is still active, writes a RecordCommit to the WAL,
// releases all locks the transaction holds via the lock manager, marks the transaction as
// committed, and removes it from the active map.
func (m *TxnManager) Commit(txn *Txn) error {
	if txn.State != TxnActive {
		return errors.New("Txn not in an active state")
	}
	_, err := m.wal.WriteRecord(&wal.Record{Type: wal.RecordCommit, TxnID: txn.ID})
	if err != nil {
		return err
	}
	m.lockMgr.ReleaseAll(txn.ID)

	txn.State = TxnCommitted
	delete(m.active, txn.ID)
	return nil
}

// Rollback validates that the transaction is still active, applies the undo log entries
// in reverse order to restore page before-images, writes a RecordAbort to the WAL, releases
// all locks, and marks the transaction as aborted.
func (m *TxnManager) Rollback(txn *Txn) error {
	if txn.State != TxnActive {
		return errors.New("Txn not in an active state")
	}

	for i := len(txn.undoLog) - 1; i >= 0; i-- {
		entry := txn.undoLog[i]
		if err := m.pageWriter.WritePage(entry.pageID, entry.beforeImage); err != nil {
			return err
		}
	}

	if _, err := m.wal.WriteRecord(&wal.Record{Type: wal.RecordAbort, TxnID: txn.ID}); err != nil {
		return err
	}

	m.lockMgr.ReleaseAll(txn.ID)
	txn.State = TxnAborted
	delete(m.active, txn.ID)
	return nil
}
