package wal

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
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

func TestWALWriteRead(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test")

	walFile, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer walFile.Close()

	beforeImage := bytes.Repeat([]byte{0xAA}, PageSize)
	afterImage := bytes.Repeat([]byte{0xBB}, PageSize)

	lsn0, err := walFile.WriteRecord(&Record{TxnID: 1, Type: RecordBegin})
	if err != nil {
		t.Fatalf("WriteRecord BEGIN: %v", err)
	}
	lsn1, err := walFile.WriteRecord(&Record{TxnID: 1, Type: RecordUpdate, PageID: 7, BeforeImage: beforeImage, AfterImage: afterImage})
	if err != nil {
		t.Fatalf("WriteRecord UPDATE: %v", err)
	}
	lsn2, err := walFile.WriteRecord(&Record{TxnID: 1, Type: RecordCommit})
	if err != nil {
		t.Fatalf("WriteRecord COMMIT: %v", err)
	}

	records, err := walFile.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(records) != 3 {
		t.Fatalf("expected 3 records, got %d", len(records))
	}

	if records[0].LSN != lsn0 || records[0].Type != RecordBegin || records[0].TxnID != 1 {
		t.Errorf("record 0 mismatch: %+v", records[0])
	}
	if records[1].LSN != lsn1 || records[1].Type != RecordUpdate || records[1].PageID != 7 {
		t.Errorf("record 1 mismatch: %+v", records[1])
	}
	if !bytes.Equal(records[1].BeforeImage, beforeImage) {
		t.Error("before image mismatch")
	}
	if !bytes.Equal(records[1].AfterImage, afterImage) {
		t.Error("after image mismatch")
	}
	if records[2].LSN != lsn2 || records[2].Type != RecordCommit {
		t.Errorf("record 2 mismatch: %+v", records[2])
	}
}

func TestWALChecksumRejectsCorruption(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test")

	walFile, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := walFile.WriteRecord(&Record{TxnID: 1, Type: RecordBegin}); err != nil {
		t.Fatalf("WriteRecord: %v", err)
	}
	walFile.Close()

	walPath := path + ".wal"
	data, err := os.ReadFile(walPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	data[5] ^= 0xFF
	if err := os.WriteFile(walPath, data, 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	walFile, err = Open(path)
	if err != nil {
		t.Fatalf("Open after corruption: %v", err)
	}
	defer walFile.Close()

	records, err := walFile.ReadAll()
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records after corruption, got %d", len(records))
	}
}

func TestRecovery(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test")

	walFile, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	beforeImage := bytes.Repeat([]byte{0x11}, PageSize)
	afterImage := bytes.Repeat([]byte{0x22}, PageSize)

	if _, err := walFile.WriteRecord(&Record{TxnID: 42, Type: RecordBegin}); err != nil {
		t.Fatalf("WriteRecord BEGIN: %v", err)
	}
	if _, err := walFile.WriteRecord(&Record{TxnID: 42, Type: RecordUpdate, PageID: 3, BeforeImage: beforeImage, AfterImage: afterImage}); err != nil {
		t.Fatalf("WriteRecord UPDATE: %v", err)
	}
	walFile.Close()

	writer := &mockPageWriter{}
	if err := Recover(path, writer); err != nil {
		t.Fatalf("Recover: %v", err)
	}

	if !bytes.Equal(writer.written[3], beforeImage) {
		t.Error("expected before image on page 3 after recovery, undo did not apply")
	}
}
