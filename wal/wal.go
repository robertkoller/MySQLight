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

func Open(path string) (*WAL, error) {
	// TODO: open or create path+".wal" with O_RDWR|O_CREATE
	// TODO: scan to the end of valid records to determine nextLSN
	panic("not implemented")
}

func (w *WAL) WriteRecord(r *Record) (lsn uint64, err error) {
	// TODO: assign r.LSN = w.nextLSN; w.nextLSN++
	// TODO: compute CRC32 over all fields except Checksum, store in r.Checksum
	// TODO: encode the record and append it to the WAL file
	// TODO: return the assigned LSN
	panic("not implemented")
}

func (w *WAL) ReadAll() ([]*Record, error) {
	// TODO: seek to offset 0
	// TODO: decode records one by one until EOF or a bad checksum (partial write on crash)
	// TODO: verify each record's checksum before appending to the result slice
	panic("not implemented")
}

func (w *WAL) Checkpoint() error {
	// TODO: truncate the WAL file to zero (os.Truncate) after a clean commit or recovery
	panic("not implemented")
}

func (w *WAL) Close() error {
	// TODO: file.Sync() then file.Close()
	panic("not implemented")
}
