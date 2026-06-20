package wal

// Recover runs ARIES-style crash recovery on startup.
// It must be called before the storage engine accepts any new transactions.
// Dependency: needs access to the storage pager to apply before/after images.
// TODO: import the storage package once the module path is finalised.
func Recover(walPath string) error {
	// TODO: open the WAL; if empty or missing, return nil (nothing to recover)
	// TODO: records, err := wal.ReadAll()
	// TODO: activeTxns := analysisPass(records)
	// TODO: redoPass(records)
	// TODO: undoPass(records, activeTxns)
	// TODO: wal.Checkpoint() — truncate WAL after clean recovery
	panic("not implemented")
}

// analysisPass scans the WAL forward and returns the set of transaction IDs
// that were active at the time of the crash (started but never committed/aborted).
func analysisPass(records []*Record) map[uint64]bool {
	// TODO: iterate records forward
	// TODO: RecordBegin  → add TxnID to active set
	// TODO: RecordCommit → remove TxnID from active set
	// TODO: RecordAbort  → remove TxnID from active set
	// TODO: return the active set — these transactions need to be undone
	panic("not implemented")
}

// redoPass re-applies every UPDATE record's AfterImage to bring the database
// to the exact state it was in at the moment of the crash.
func redoPass(records []*Record) error {
	// TODO: iterate records forward
	// TODO: for each RecordUpdate: write AfterImage to the page via the pager
	//         (redo applies to ALL updates, including uncommitted ones)
	panic("not implemented")
}

// undoPass rolls back all transactions that were active at crash time by
// applying their BeforeImages in reverse LSN order.
func undoPass(records []*Record, activeTxns map[uint64]bool) error {
	// TODO: iterate records in reverse (highest LSN first)
	// TODO: for each RecordUpdate whose TxnID is in activeTxns:
	//         write BeforeImage to the page via the pager
	// TODO: after all updates for a txn are undone, write a RecordAbort for it
	panic("not implemented")
}
