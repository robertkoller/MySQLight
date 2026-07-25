# MySQLight WAL

Phase 4a/4b of MySQLight. The write-ahead log (WAL) guarantees crash recovery — any change that was committed before a crash will survive it, and any change that was not committed will be rolled back. The WAL is a second file (`<dbname>.wal`) that sits alongside the main database file.

---

## The core invariant: log before data

Before any dirty page is written from the buffer pool to the `.db` file, a WAL record describing that change must be fsynced to the `.wal` file first. This means the on-disk WAL always reflects at least as much as the on-disk database file, so recovery can always reconstruct the correct state.

---

## Layering

```
        ┌─────────────────────────────┐
        │       TxnManager            │   txn/txn.go — writes BEGIN/COMMIT/ABORT records
        └──────────────┬──────────────┘
                       │ WriteRecord
        ┌──────────────▼──────────────┐
        │            WAL              │   wal/wal.go — append-only log file
        └──────────────┬──────────────┘
                       │ on startup
        ┌──────────────▼──────────────┐
        │          Recovery           │   wal/recovery.go — ARIES-style redo + undo
        └──────────────┬──────────────┘
                       │ WritePage
        ┌──────────────▼──────────────┐
        │        PageWriter           │   interface — implemented by storage.Pager
        └─────────────────────────────┘
```

---

## Wire format

Every record is encoded big-endian and appended sequentially. The file is never modified in-place — only appended to, then truncated on checkpoint.

```
Non-UPDATE records (BEGIN, COMMIT, ABORT) — 25 bytes:
  [LSN:      8 bytes]   monotonically increasing sequence number
  [TxnID:    8 bytes]   which transaction this record belongs to
  [Type:     1 byte]    0=BEGIN, 1=COMMIT, 2=ABORT, 3=UPDATE
  [PageID:   4 bytes]   unused for non-UPDATE; present for fixed-width parsing
  [Checksum: 4 bytes]   CRC32 of all preceding bytes in this record

UPDATE records — 8,217 bytes:
  [LSN:         8 bytes]
  [TxnID:       8 bytes]
  [Type:        1 byte]    = 3 (RecordUpdate)
  [PageID:      4 bytes]   which page was modified
  [BeforeImage: 4096 bytes] page contents before the change
  [AfterImage:  4096 bytes] page contents after the change
  [Checksum:    4 bytes]   CRC32 of all preceding bytes
```

The checksum allows `ReadAll` to detect a crash-truncated record at the tail: a partial write produces a checksum mismatch, and reading stops there.

---

## API — `wal.go`

| Function / Method | Description |
|---|---|
| `Open(path string) (*WAL, error)` | Opens or creates `path+".wal"`. Scans existing records to restore `nextLSN`. |
| `(w *WAL) WriteRecord(r *Record) (uint64, error)` | Assigns LSN, computes CRC32, appends to file. Returns the assigned LSN. |
| `(w *WAL) ReadAll() ([]*Record, error)` | Seeks to start, decodes all records, stops at first checksum mismatch. |
| `(w *WAL) Checkpoint() error` | Truncates the WAL to zero and seeks to start. Called after clean recovery. |
| `(w *WAL) Close() error` | `file.Sync()` then `file.Close()`. |
| `encodeRecord(r *Record) ([]byte, uint32)` | Encodes a record to wire bytes and returns its CRC32. Used by `WriteRecord`. |

### `PageWriter` interface

```go
type PageWriter interface {
    WritePage(pageID uint32, data []byte) error
}
```

Defined in the `wal` package to avoid a circular import: `storage` would need to import `wal` (to write log records before flushing pages), so `wal` cannot import `storage`. `storage.Pager` implements `PageWriter` without the `wal` package ever depending on `storage`.

---

## Recovery — `recovery.go`

Called once at startup if the WAL file is non-empty. Implements a simplified version of ARIES (Analysis, Redo, Undo).

```go
func Recover(walPath string, writer PageWriter) error
```

### Three passes

**Analysis + Redo (combined, one forward scan)**

`analysisPass` reads every record in LSN order and does two things simultaneously:

- Maintains the *active transaction set*: `BEGIN` adds a TxnID, `COMMIT` and `ABORT` remove it.
- Applies every `UPDATE` record's `AfterImage` to the relevant page via `writer.WritePage`.

Combining these is safe — both scan forward over the same records. After this pass, the database is in the exact byte state it was at the moment of crash, including any uncommitted writes.

**Undo (reverse scan)**

`undoPass` iterates records from highest LSN to lowest. For every `UPDATE` record whose `TxnID` is in the active set (i.e. never committed), it applies the `BeforeImage`, rolling back that change. After undoing all records for a transaction, a `RecordAbort` is written to the WAL so a second crash during recovery does not undo the same transaction twice.

**Checkpoint**

After both passes, `Checkpoint()` truncates the WAL. All changes are now durably in the `.db` file with uncommitted writes fully reversed.

### Why redo before undo?

Redo is applied blindly to all records regardless of commit status. This brings the database to a consistent known state first. Undo then selectively reverses uncommitted work. Interleaving the two would apply before-images against a partially-recovered database, producing an incorrect result.

---

## Phase 4a/4b status — complete

| Component | Status |
|---|---|
| Wire format (LSN, TxnID, Type, PageID, images, CRC32) | Done |
| `Open` — file creation, LSN scan on startup | Done |
| `WriteRecord` — LSN assignment, CRC32, append | Done |
| `ReadAll` — sequential decode, crash-tail detection | Done |
| `Checkpoint` — truncate + seek | Done |
| `Close` — fsync + close | Done |
| `PageWriter` interface (avoids circular import) | Done |
| `Recover` — analysis+redo combined, undo pass | Done |
| Tests (write/read roundtrip, checksum corruption, recovery) | Done |

### Not in Phase 4a/4b

- WAL integration with buffer pool `UnpinPage` — Phase 4c/4d
- WAL integration with executor DML (before-image capture) — Phase 4c/4d
