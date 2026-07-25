package txn

import "sync"

type LockMode uint8

const (
	LockShared    LockMode = iota // multiple transactions may hold simultaneously; required to read
	LockExclusive                 // only one transaction may hold; required to write
)

type lockEntry struct {
	holders map[uint64]LockMode
	waiters []waiter
}

type waiter struct {
	txnID uint64
	mode  LockMode
	grant chan struct{}
}

// LockManager grants and releases table-level shared/exclusive locks.
// Uses two-phase locking (2PL): a transaction acquires locks before releasing any.
type LockManager struct {
	locks map[string]*lockEntry
	mu    sync.Mutex
}

// NewLockManager initialises the lock manager with an empty table lock map.
func NewLockManager() *LockManager {
	return &LockManager{locks: make(map[string]*lockEntry)}
}

// canGrant reports whether a new lock of mode can be granted given the current entry state.
// Shared is compatible with other Shared holders. Exclusive requires no holders at all.
func canGrant(entry *lockEntry, mode LockMode) bool {
	if len(entry.holders) == 0 {
		return true
	}
	if mode == LockShared {
		for _, holderMode := range entry.holders {
			if holderMode == LockExclusive {
				return false
			}
		}
		return true
	}
	return false
}

// Acquire grants a shared or exclusive table lock to the given transaction. Shared locks
// are compatible with each other and are granted immediately. An exclusive lock, or any
// lock requested while an exclusive lock is held by another transaction, must wait until
// all current holders release. The caller blocks on a channel that is closed when granted.
func (lm *LockManager) Acquire(txnID uint64, table string, mode LockMode) error {
	lm.mu.Lock()

	entry, ok := lm.locks[table]
	if !ok {
		entry = &lockEntry{holders: make(map[uint64]LockMode)}
		lm.locks[table] = entry
	}

	if canGrant(entry, mode) {
		entry.holders[txnID] = mode
		lm.mu.Unlock()
		return nil
	}

	grantCh := make(chan struct{})
	entry.waiters = append(entry.waiters, waiter{txnID: txnID, mode: mode, grant: grantCh})
	lm.mu.Unlock()

	<-grantCh
	return nil
}

// Release removes the transaction from the holder set for the given table lock and then
// grants the lock to any waiting transactions that are now compatible, in FIFO order.
func (lm *LockManager) Release(txnID uint64, table string) {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	entry, ok := lm.locks[table]
	if !ok {
		return
	}

	delete(entry.holders, txnID)

	var remaining []waiter
	stopGranting := false
	for _, w := range entry.waiters {
		if !stopGranting && canGrant(entry, w.mode) {
			entry.holders[w.txnID] = w.mode
			close(w.grant)
			if w.mode == LockExclusive {
				stopGranting = true
			}
		} else {
			stopGranting = true
			remaining = append(remaining, w)
		}
	}
	entry.waiters = remaining
}

// ReleaseAll releases every lock the given transaction holds, waking any waiters that
// can now be granted their requested lock mode.
func (lm *LockManager) ReleaseAll(txnID uint64) {
	lm.mu.Lock()
	var tables []string
	for table, entry := range lm.locks {
		if _, held := entry.holders[txnID]; held {
			tables = append(tables, table)
		}
	}
	lm.mu.Unlock()

	for _, table := range tables {
		lm.Release(txnID, table)
	}
}

// detectDeadlock builds a wait-for graph and detects cycles using DFS.
// Returns the TxnIDs of one victim per cycle — callers are responsible for rolling them back.
// Intended to run periodically in a background goroutine, not on every Acquire.
func (lm *LockManager) detectDeadlock() []uint64 {
	lm.mu.Lock()
	defer lm.mu.Unlock()

	waitFor := make(map[uint64][]uint64)
	for _, entry := range lm.locks {
		for _, w := range entry.waiters {
			for holderID := range entry.holders {
				waitFor[w.txnID] = append(waitFor[w.txnID], holderID)
			}
		}
	}

	visited := make(map[uint64]bool)
	inStack := make(map[uint64]bool)
	var victims []uint64

	var dfs func(txnID uint64) bool
	dfs = func(txnID uint64) bool {
		visited[txnID] = true
		inStack[txnID] = true
		for _, neighbor := range waitFor[txnID] {
			if !visited[neighbor] {
				if dfs(neighbor) {
					return true
				}
			} else if inStack[neighbor] {
				victims = append(victims, txnID)
				return true
			}
		}
		inStack[txnID] = false
		return false
	}

	for txnID := range waitFor {
		if !visited[txnID] {
			dfs(txnID)
		}
	}

	return victims
}
