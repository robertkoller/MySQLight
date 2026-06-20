package wal

import "os"

// WAL record wire format (all fields big-endian):
//   LSN          8 bytes
//   TxnID        8 bytes
//   Type         1 byte
//   PageID       4 bytes  (only meaningful for RecordUpdate)
//   BeforeImage  4096 bytes (only present for RecordUpdate)
//   AfterImage   4096 bytes (only present for RecordUpdate)
//   Checksum     4 bytes  (CRC32 of all preceding bytes in the record)

type RecordType uint8

const (
	RecordBegin  RecordType = iota
	RecordCommit RecordType = iota
	RecordAbort  RecordType = iota
	RecordUpdate RecordType = iota
)

type Record struct {
	LSN         uint64
	TxnID       uint64
	Type        RecordType
	PageID      uint32
	BeforeImage []byte // PageSize bytes; nil for non-Update records
	AfterImage  []byte // PageSize bytes; nil for non-Update records
	Checksum    uint32
}

type WAL struct {
	// TODO: file    *os.File
	// TODO: nextLSN uint64
}

var _ = os.O_RDWR // ensure os is used

// Open opens or creates the WAL file at path+".wal". It scans forward through any existing
// records to determine the next LSN, so that new records are assigned correct sequence numbers.
// Partially-written records at the tail (detected by a checksum mismatch) are silently ignored,
// as they are the expected result of a crash mid-write.
func Open(path string) (*WAL, error) {
	// TODO: open or create path+".wal" with O_RDWR|O_CREATE
	// TODO: scan to the end of valid records to determine nextLSN
	panic("not implemented")
}

// WriteRecord assigns the next LSN to the record, computes a CRC32 checksum over all
// fields except the checksum itself, encodes the record in the WAL wire format, and
// appends it to the WAL file. The assigned LSN is returned so callers can reference
// this record during recovery.
func (w *WAL) WriteRecord(r *Record) (lsn uint64, err error) {
	// TODO: assign r.LSN = w.nextLSN; w.nextLSN++
	// TODO: compute CRC32 over all fields except Checksum, store in r.Checksum
	// TODO: encode the record and append it to the WAL file
	// TODO: return the assigned LSN
	panic("not implemented")
}

// ReadAll seeks to the beginning of the WAL and decodes every record in order. Each
// record's CRC32 checksum is verified before it is added to the result. Decoding stops
// at the first checksum mismatch or unexpected EOF, since anything past that point is
// a partial write caused by a crash.
func (w *WAL) ReadAll() ([]*Record, error) {
	// TODO: seek to offset 0
	// TODO: decode records one by one until EOF or a bad checksum (partial write on crash)
	// TODO: verify each record's checksum before appending to the result slice
	panic("not implemented")
}

// Checkpoint truncates the WAL file to zero bytes after a clean commit or a successful
// recovery. This marks the point at which all changes are durably reflected in the main
// database file and the log is no longer needed for recovery.
func (w *WAL) Checkpoint() error {
	// TODO: truncate the WAL file to zero (os.Truncate) after a clean commit or recovery
	panic("not implemented")
}

// Close syncs any OS-buffered writes to disk and then closes the WAL file handle,
// ensuring no pending log records are lost before the process exits.
func (w *WAL) Close() error {
	// TODO: file.Sync() then file.Close()
	panic("not implemented")
}
