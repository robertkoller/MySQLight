package wal

import (
	"os"
	"testing"
)

func TestWALWriteRead(t *testing.T) {
	// TODO: open a WAL on a temp file
	// TODO: write a BEGIN, UPDATE, and COMMIT record
	// TODO: ReadAll() and assert the three records come back in order with correct fields
	t.Skip("not implemented")
	defer os.Remove("test.wal")
}

func TestWALChecksumRejectsCorruption(t *testing.T) {
	// TODO: write a valid record, then flip a byte in the file
	// TODO: ReadAll() should stop before the corrupted record and not return it
	t.Skip("not implemented")
}

func TestRecovery(t *testing.T) {
	// TODO: set up a database with some pages
	// TODO: write BEGIN + UPDATE records but NOT a COMMIT (simulate crash)
	// TODO: call Recover(), then read the affected pages
	// TODO: assert the pages reflect the BeforeImage (change was rolled back)
	t.Skip("not implemented")
}
