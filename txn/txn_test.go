package txn

import "testing"

func TestBeginCommit(t *testing.T) {
	// TODO: Begin a transaction, do some writes, Commit
	// TODO: assert the transaction is marked committed and locks are released
	t.Skip("not implemented")
}

func TestRollback(t *testing.T) {
	// TODO: Begin a transaction, write rows, Rollback
	// TODO: assert the rows are gone (before-images were restored)
	t.Skip("not implemented")
}

func TestLockCompatibility(t *testing.T) {
	// TODO: two goroutines each acquire a shared lock on the same table → both succeed
	// TODO: one holds a shared lock, another tries exclusive → second blocks until first releases
	// TODO: two goroutines each try exclusive on the same table → second blocks until first releases
	t.Skip("not implemented")
}

func TestDeadlockDetection(t *testing.T) {
	// TODO: txnA holds lock on tableX and waits for tableY
	// TODO: txnB holds lock on tableY and waits for tableX → deadlock
	// TODO: assert one transaction is aborted and the other proceeds
	t.Skip("not implemented")
}
