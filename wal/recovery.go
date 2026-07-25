package wal

type PageWriter interface {
	WritePage(pageID uint32, data []byte) error
}

// Recover runs ARIES-style crash recovery on startup.
// It must be called before the storage engine accepts any new transactions.
// Dependency: needs access to the storage pager to apply before/after images.
// TODO: import the storage package once the module path is finalised.
func Recover(walPath string, writer PageWriter) error {
	wal, err := Open(walPath)
	if err != nil {
		return err
	}

	records, err := wal.ReadAll()
	if err != nil {
		return err
	}
	active, err := analysisPass(records, writer)
	if err != nil {
		return err
	}
	undoPass(records, active, writer, wal)
	if err := wal.Checkpoint(); err != nil {
		return err
	}
	return wal.Close()
}

// analysisPass scans the WAL forward and returns the set of transaction IDs
// that were active at the time of the crash (started but never committed/aborted).
func analysisPass(records []*Record, writer PageWriter) (map[uint64]bool, error) {
	output := make(map[uint64]bool)

	for _, record := range records {
		switch record.Type {
		case RecordAbort:
			delete(output, record.TxnID)
		case RecordBegin:
			output[record.TxnID] = true
		case RecordCommit:
			delete(output, record.TxnID)
		case RecordUpdate:
			if err := writer.WritePage(record.PageID, record.AfterImage); err != nil {
				return output, err
			}
		}
	}

	return output, nil
}

// undoPass rolls back all transactions that were active at crash time by
// applying their BeforeImages in reverse LSN order.
func undoPass(records []*Record, activeTxns map[uint64]bool, writer PageWriter, wal *WAL) error {
	for i := len(records) - 1; i >= 0; i-- {
		record := records[i]
		if record.Type == RecordUpdate {
			_, exists := activeTxns[record.TxnID]
			if exists {
				if err := writer.WritePage(record.PageID, record.BeforeImage); err != nil {
					return err
				}
			}
		}
	}

	for id := range activeTxns {
		_, err := wal.WriteRecord(&Record{TxnID: id, Type: RecordAbort})
		if err != nil {
			return err
		}
	}

	return nil
}
