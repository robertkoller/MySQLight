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
| `NewTxnManager(walFile *wal.WAL, pool *storage.BufferPool) *TxnManager` | Initialises with `nextID=1`, empty active map, new lock manager. |
| `(m *TxnManager) LockManager() *LockManager` | Returns the embedded lock manager (used by executor to wire `WithLock` on scans). |
| `(m *TxnManager) Begin() (*Txn, error)` | Allocates a `Txn`, writes `RecordBegin`, registers in active map. |
| `(m *TxnManager) Commit(txn *Txn) error` | Validates active, writes `RecordCommit`, releases all locks, marks committed. |
| `(m *TxnManager) Rollback(txn *Txn) error` | Validates active, calls `pool.RollbackTxn`, writes `RecordAbort`, releases all locks, marks aborted. |

### Key types

```go
type Txn struct {
    ID    uint64
    State TxnState // TxnActive, TxnCommitted, TxnAborted
}
```

There is no per-transaction undo log in the transaction manager. Before-image tracking lives entirely in the buffer pool: `pool.RollbackTxn(txnID)` restores before-images for all pages dirtied by that transaction. If a dirty page was evicted before rollback, its before-image is written back to disk via the pager.

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

## Status — complete

| Component | Status |
|---|---|
| `TxnState` (Active, Committed, Aborted) | Done |
| `Txn` struct | Done |
| `TxnManager` — Begin / Commit / Rollback | Done |
| WAL record writes on each lifecycle event | Done |
| Lock release on Commit and Rollback | Done |
| `pool.RollbackTxn` delegation on Rollback | Done |
| Buffer pool before-image tracking | Done |
| Executor `acquireExclusive` on DML | Done |
| Executor `WithLock` on SELECT scans (strict 2PL) | Done |
| `LockManager` — Acquire / Release / ReleaseAll | Done |
| Channel-based blocking for incompatible locks | Done |
| FIFO waiter grant order | Done |
| `detectDeadlock` — wait-for graph + DFS | Done |
| Tests (9 cases) | Done |

### Not yet implemented

- Background goroutine running `detectDeadlock` periodically — stretch goal
- Row-level locking (MVCC) — stretch goal
