# MySQLight Transaction Manager

Phase 4c/4d of MySQLight. The transaction manager provides ACID semantics: it coordinates WAL record writes, table-level locking, and undo log application so that each transaction either commits fully or leaves the database unchanged.

---

## Layering

```
        ┌─────────────────────────────┐
        │        REPL / API           │   BEGIN / COMMIT / ROLLBACK commands
        └──────────────┬──────────────┘
                       │
        ┌──────────────▼──────────────┐
        │         TxnManager          │   txn/txn.go — lifecycle management
        └──────┬───────────┬──────────┘
               │           │
        WAL    │           │  Locks
               ▼           ▼
        wal.WAL       LockManager          txn/lock_manager.go — 2PL table locks
```

---

## Transaction lifecycle

```
         Begin()
            │
            ▼
        TxnActive ──────── Commit() ──────► TxnCommitted
            │
            └────────────  Rollback() ────► TxnAborted
```

`Begin`, `Commit`, and `Rollback` all write a corresponding WAL record (`RecordBegin`, `RecordCommit`, `RecordAbort`) so the WAL always has a complete picture of transaction state for crash recovery.

---

## API — `txn.go`

| Function / Method | Description |
|---|---|
| `NewTxnManager(wal *wal.WAL, writer wal.PageWriter) *TxnManager` | Initialises with `nextID=1`, empty active map, new lock manager. |
| `(m *TxnManager) Begin() (*Txn, error)` | Allocates a `Txn`, writes `RecordBegin`, registers in active map. |
| `(m *TxnManager) Commit(txn *Txn) error` | Validates active, writes `RecordCommit`, releases all locks, marks committed. |
| `(m *TxnManager) Rollback(txn *Txn) error` | Validates active, applies undo log in reverse, writes `RecordAbort`, releases all locks, marks aborted. |

### Key types

```go
type Txn struct {
    ID      uint64
    State   TxnState    // TxnActive, TxnCommitted, TxnAborted
    undoLog []UndoEntry // before-images to restore on rollback
}

type UndoEntry struct {
    pageID      uint32
    beforeImage []byte // full 4096-byte page snapshot
}
```

`undoLog` is appended to by the executor each time a page is modified within a transaction. `Rollback` iterates it in reverse — last modified page is restored first — to unwind changes in the correct order.

---

## Lock manager — `lock_manager.go`

Implements table-level two-phase locking (2PL). A transaction acquires all locks before releasing any. Locks are released in bulk on `Commit` or `Rollback` via `ReleaseAll`.

### Lock compatibility

| Held \ Requested | Shared | Exclusive |
|---|---|---|
| None | Grant | Grant |
| Shared | Grant | Block |
| Exclusive | Block | Block |

Multiple transactions may hold a shared lock simultaneously (readers don't block readers). Any exclusive lock blocks all other requests until it is released.

### Blocking mechanism

When a lock cannot be granted immediately, `Acquire` creates a `chan struct{}` and appends a `waiter` entry to the lock entry's queue, then blocks on `<-grantCh`. When the current holder calls `Release`, it iterates the waiter queue in FIFO order, closing grant channels for compatible waiters. Closing a channel is the signal to the blocked goroutine that its lock has been granted.

```go
// Grant shared waiters in FIFO order, stop at first exclusive waiter.
// Grant an exclusive waiter only when no holders remain.
```

### API

| Method | Description |
|---|---|
| `NewLockManager() *LockManager` | Initialises with empty lock map. |
| `Acquire(txnID uint64, table string, mode LockMode) error` | Grants immediately or blocks until compatible. |
| `Release(txnID uint64, table string)` | Removes from holders, grants compatible waiters in FIFO order. |
| `ReleaseAll(txnID uint64)` | Releases every lock the transaction holds across all tables. |
| `detectDeadlock() []uint64` | Builds wait-for graph, runs DFS to find cycles, returns victim TxnIDs. |

### Deadlock detection

`detectDeadlock` builds a directed wait-for graph: an edge from transaction A to transaction B means A is blocked waiting for a lock that B holds. It detects cycles using depth-first search and returns one victim TxnID per cycle. The caller is responsible for rolling back victims.

Deadlock detection is designed to run periodically in a background goroutine — not on every `Acquire` call — to keep lock acquisition fast.

---

## Two-phase locking (2PL)

The lock manager enforces the *growing phase* (acquire locks as needed) but relies on the caller to enforce the *shrinking phase* (release all locks only at transaction end, never mid-transaction). `TxnManager` upholds this by only calling `ReleaseAll` inside `Commit` and `Rollback`, never earlier.

---

## Phase 4c/4d status — complete

| Component | Status |
|---|---|
| `TxnState` (Active, Committed, Aborted) | Done |
| `UndoEntry` (pageID + before-image) | Done |
| `Txn` struct with undo log | Done |
| `TxnManager` — Begin / Commit / Rollback | Done |
| WAL record writes on each lifecycle event | Done |
| Lock release on Commit and Rollback | Done |
| Undo log application in Rollback | Done |
| `LockManager` — Acquire / Release / ReleaseAll | Done |
| Channel-based blocking for incompatible locks | Done |
| FIFO waiter grant order | Done |
| `detectDeadlock` — wait-for graph + DFS | Done |
| Tests (9 cases) | Done |

### Not in Phase 4c/4d

- Executor integration: acquiring table locks before reads/writes — stretch goal
- Buffer pool integration: appending `UndoEntry` before each page flush — stretch goal
- Background goroutine running `detectDeadlock` periodically — stretch goal
- Row-level locking (MVCC) — stretch goal
