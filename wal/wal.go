package wal

import (
	"encoding/binary"
	"hash/crc32"
	"io"
	"os"
)

const PageSize = 4096

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
	RecordBegin RecordType = iota //0
	RecordCommit
	RecordAbort
	RecordUpdate
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
	file    *os.File
	nextLSN uint64
}

// Open opens or creates the WAL file at path+".wal". It scans forward through any existing
// records to determine the next LSN, so that new records are assigned correct sequence numbers.
// Partially-written records at the tail (detected by a checksum mismatch) are silently ignored,
// as they are the expected result of a crash mid-write.
func Open(path string) (*WAL, error) {
	file, err := os.OpenFile(path+".wal", os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, err
	}

	wal := WAL{
		file: file,
	}

	records, err := wal.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(records) > 0 {
		wal.nextLSN = records[len(records)-1].LSN + 1
	}

	return &wal, nil
}

// WriteRecord assigns the next LSN to the record, computes a CRC32 checksum over all
// fields except the checksum itself, encodes the record in the WAL wire format, and
// appends it to the WAL file. The assigned LSN is returned so callers can reference
// this record during recovery.
func (w *WAL) WriteRecord(r *Record) (uint64, error) {
	r.LSN = w.nextLSN
	w.nextLSN++
	encoded, sum := encodeRecord(r)
	r.Checksum = sum

	_, err := w.file.Write(encoded)
	if err != nil {
		return 0, err
	}

	return r.LSN, nil
}

// Encodes a WAL record and returns the checksum
func encodeRecord(r *Record) ([]byte, uint32) {
	buffer := make([]byte, 21)
	crc := make([]byte, 4)
	binary.BigEndian.PutUint64(buffer[0:], r.LSN)
	binary.BigEndian.PutUint64(buffer[8:], r.TxnID)
	buffer[16] = byte(r.Type)
	binary.BigEndian.PutUint32(buffer[17:], r.PageID)
	if r.Type == RecordUpdate {
		buffer = append(buffer, append(r.BeforeImage, r.AfterImage...)...)
	}
	sum := crc32.ChecksumIEEE(buffer)
	binary.BigEndian.PutUint32(crc, sum)
	return append(buffer, crc...), sum

}

// ReadAll seeks to the beginning of the WAL and decodes every record in order. Each
// record's CRC32 checksum is verified before it is added to the result. Decoding stops
// at the first checksum mismatch or unexpected EOF, since anything past that point is
// a partial write caused by a crash.
func (w *WAL) ReadAll() ([]*Record, error) {
	var records []*Record
	_, err := w.file.Seek(0, 0)
	if err != nil {
		return nil, err
	}
OuterLoop: // fancy smancy new think I learned here :)
	for {
		buffer := make([]byte, 21)
		_, err := io.ReadFull(w.file, buffer)
		if err == io.EOF {
			break
		}
		if err != nil {
			return records, err
		}

		lsn := binary.BigEndian.Uint64(buffer[0:])
		txnID := binary.BigEndian.Uint64(buffer[8:])
		pageID := binary.BigEndian.Uint32(buffer[17:])
		typed := buffer[16]
		switch typed {
		case byte(RecordUpdate):
			beforeImage := make([]byte, PageSize)
			afterImage := make([]byte, PageSize)
			crc := make([]byte, 4)

			_, err = io.ReadFull(w.file, beforeImage)
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break OuterLoop
			}
			if err != nil {
				return records, err
			}

			_, err = io.ReadFull(w.file, afterImage)
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break OuterLoop
			}
			if err != nil {
				return records, err
			}

			_, err = io.ReadFull(w.file, crc)
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break OuterLoop
			}
			if err != nil {
				return records, err
			}

			crcSum := binary.BigEndian.Uint32(crc)

			buffer = append(buffer, append(beforeImage, afterImage...)...)

			if crc32.ChecksumIEEE(buffer) != crcSum {
				break OuterLoop
			}

			records = append(records, &Record{
				LSN: lsn, TxnID: txnID, Type: RecordType(typed), PageID: pageID, BeforeImage: beforeImage, AfterImage: afterImage, Checksum: crcSum,
			})
		case byte(RecordAbort), byte(RecordBegin), byte(RecordCommit):
			crc := make([]byte, 4)

			_, err = io.ReadFull(w.file, crc)
			if err == io.EOF || err == io.ErrUnexpectedEOF {
				break OuterLoop
			}
			if err != nil {
				return records, err
			}

			crcSum := binary.BigEndian.Uint32(crc)

			if crc32.ChecksumIEEE(buffer) != crcSum {
				break OuterLoop
			}

			records = append(records, &Record{
				LSN: lsn, TxnID: txnID, Type: RecordType(typed), PageID: pageID, Checksum: crcSum,
			})
		default:
			break OuterLoop
		}

		if lsn+1 > w.nextLSN {
			w.nextLSN = lsn + 1
		}

	}

	return records, nil
}

// Checkpoint truncates the WAL file to zero bytes after a clean commit or a successful
// recovery. This marks the point at which all changes are durably reflected in the main
// database file and the log is no longer needed for recovery.
func (w *WAL) Checkpoint() error {
	if err := w.file.Truncate(0); err != nil {
		return err
	}
	_, err := w.file.Seek(0, 0)
	return err
}

// Close syncs any OS-buffered writes to disk and then closes the WAL file handle,
// ensuring no pending log records are lost before the process exits.
func (w *WAL) Close() error {
	if err := w.file.Sync(); err != nil {
		return err
	}
	return w.file.Close()
}
