package txn

import (
	"bytes"
	"path/filepath"
	"testing"
	"time"

	"github.com/robertkoller/MySQLight/wal"
)

type mockPageWriter struct {
	written map[uint32][]byte
}

func (m *mockPageWriter) WritePage(pageID uint32, data []byte) error {
	if m.written == nil {
		m.written = make(map[uint32][]byte)
	}
	m.written[pageID] = append([]byte{}, data...)
	return nil
}

func newTestManager(t *testing.T) (*TxnManager, func()) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test")
	walFile, err := wal.Open(path)
	if err != nil {
		t.Fatalf("wal.Open: %v", err)
	}
	writer := &mockPageWriter{}
	manager := NewTxnManager(walFile, writer)
	return manager, func() { walFile.Close() }
}

func TestBeginCommit(t *testing.T) {
	manager, cleanup := newTestManager(t)
	defer cleanup()

	transaction, err := manager.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if transaction.State != TxnActive {
		t.Errorf("expected TxnActive, got %v", transaction.State)
	}
	if _, ok := manager.active[transaction.ID]; !ok {
		t.Error("transaction should be in active map after Begin")
	}

	if err := manager.Commit(transaction); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if transaction.State != TxnCommitted {
		t.Errorf("expected TxnCommitted, got %v", transaction.State)
	}
	if _, ok := manager.active[transaction.ID]; ok {
		t.Error("transaction should be removed from active map after Commit")
	}
}

func TestCommitInactiveErrors(t *testing.T) {
	manager, cleanup := newTestManager(t)
	defer cleanup()

	transaction, _ := manager.Begin()
	manager.Commit(transaction)

	if err := manager.Commit(transaction); err == nil {
		t.Error("expected error committing an already-committed transaction")
	}
}

func TestRollback(t *testing.T) {
	manager, cleanup := newTestManager(t)
	defer cleanup()

	writer := manager.pageWriter.(*mockPageWriter)
	beforeImage := bytes.Repeat([]byte{0x11}, 4096)

	transaction, err := manager.Begin()
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}

	transaction.undoLog = append(transaction.undoLog, UndoEntry{pageID: 5, beforeImage: beforeImage})

	if err := manager.Rollback(transaction); err != nil {
		t.Fatalf("Rollback: %v", err)
	}
	if transaction.State != TxnAborted {
		t.Errorf("expected TxnAborted, got %v", transaction.State)
	}
	if _, ok := manager.active[transaction.ID]; ok {
		t.Error("transaction should be removed from active map after Rollback")
	}
	if !bytes.Equal(writer.written[5], beforeImage) {
		t.Error("before image was not applied to page 5 during rollback")
	}
}

func TestRollbackInactiveErrors(t *testing.T) {
	manager, cleanup := newTestManager(t)
	defer cleanup()

	transaction, _ := manager.Begin()
	manager.Rollback(transaction)

	if err := manager.Rollback(transaction); err == nil {
		t.Error("expected error rolling back an already-aborted transaction")
	}
}

func TestLockSharedCompatible(t *testing.T) {
	manager, cleanup := newTestManager(t)
	defer cleanup()

	if err := manager.lockMgr.Acquire(1, "users", LockShared); err != nil {
		t.Fatalf("Acquire txn1 shared: %v", err)
	}
	if err := manager.lockMgr.Acquire(2, "users", LockShared); err != nil {
		t.Fatalf("Acquire txn2 shared: %v", err)
	}
}

func TestLockExclusiveBlocksShared(t *testing.T) {
	manager, cleanup := newTestManager(t)
	defer cleanup()

	if err := manager.lockMgr.Acquire(1, "users", LockShared); err != nil {
		t.Fatalf("Acquire shared: %v", err)
	}

	acquired := make(chan struct{})
	go func() {
		manager.lockMgr.Acquire(2, "users", LockExclusive)
		close(acquired)
	}()

	select {
	case <-acquired:
		t.Error("exclusive lock should not be granted while a shared lock is held")
	case <-time.After(50 * time.Millisecond):
	}

	manager.lockMgr.Release(1, "users")

	select {
	case <-acquired:
	case <-time.After(100 * time.Millisecond):
		t.Error("exclusive lock was not granted after shared lock was released")
	}
}

func TestLockExclusiveBlocksExclusive(t *testing.T) {
	manager, cleanup := newTestManager(t)
	defer cleanup()

	if err := manager.lockMgr.Acquire(1, "users", LockExclusive); err != nil {
		t.Fatalf("Acquire exclusive: %v", err)
	}

	acquired := make(chan struct{})
	go func() {
		manager.lockMgr.Acquire(2, "users", LockExclusive)
		close(acquired)
	}()

	select {
	case <-acquired:
		t.Error("second exclusive lock should not be granted while first is held")
	case <-time.After(50 * time.Millisecond):
	}

	manager.lockMgr.Release(1, "users")

	select {
	case <-acquired:
	case <-time.After(100 * time.Millisecond):
		t.Error("exclusive lock was not granted after first exclusive was released")
	}
}

func TestReleaseAll(t *testing.T) {
	manager, cleanup := newTestManager(t)
	defer cleanup()

	manager.lockMgr.Acquire(1, "users", LockExclusive)
	manager.lockMgr.Acquire(1, "orders", LockExclusive)

	acquired := make(chan struct{})
	go func() {
		manager.lockMgr.Acquire(2, "users", LockShared)
		close(acquired)
	}()

	select {
	case <-acquired:
		t.Error("shared lock should be blocked while exclusive is held")
	case <-time.After(50 * time.Millisecond):
	}

	manager.lockMgr.ReleaseAll(1)

	select {
	case <-acquired:
	case <-time.After(100 * time.Millisecond):
		t.Error("lock was not granted after ReleaseAll")
	}
}

func TestDeadlockDetection(t *testing.T) {
	manager, cleanup := newTestManager(t)
	defer cleanup()

	manager.lockMgr.Acquire(1, "tableX", LockExclusive)
	manager.lockMgr.Acquire(2, "tableY", LockExclusive)

	go func() { manager.lockMgr.Acquire(1, "tableY", LockExclusive) }()
	go func() { manager.lockMgr.Acquire(2, "tableX", LockExclusive) }()

	time.Sleep(50 * time.Millisecond)

	victims := manager.lockMgr.detectDeadlock()
	if len(victims) == 0 {
		t.Error("expected detectDeadlock to identify at least one victim in a deadlock cycle")
	}
}
