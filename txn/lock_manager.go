package txn

type LockMode uint8

const (
	LockShared    LockMode = iota // multiple transactions may hold simultaneously; required to read
	LockExclusive LockMode = iota // only one transaction may hold; required to write
)

type lockEntry struct {
	// TODO: holders map[uint64]LockMode  — txnID → mode for current holders
	// TODO: waiters []waiter             — transactions blocked on this lock
}

type waiter struct {
	// TODO: txnID uint64
	// TODO: mode  LockMode
	// TODO: grant chan struct{}  — closed when the lock is granted
}

// LockManager grants and releases table-level shared/exclusive locks.
// Uses two-phase locking (2PL): a transaction acquires locks before releasing any.
type LockManager struct {
	// TODO: locks map[string]*lockEntry  — keyed by table name
	// TODO: mu    sync.Mutex
}

// NewLockManager initialises the lock manager with an empty table lock map.
func NewLockManager() *LockManager {
	// TODO: initialise locks map
	panic("not implemented")
}

// Acquire grants a shared or exclusive table lock to the given transaction. Shared locks
// are compatible with each other and are granted immediately. An exclusive lock, or any
// lock requested while an exclusive lock is held by another transaction, must wait until
// all current holders release. If the lock cannot be granted, the caller blocks on a channel
// that is closed when the lock is available. After granting, deadlock detection is consulted
// to ensure no cycle has formed in the wait-for graph.
func (lm *LockManager) Acquire(txnID uint64, table string, mode LockMode) error {
	// TODO: check compatibility:
	//   Shared + Shared   → grant immediately (multiple readers allowed)
	//   Shared + Exclusive → block (wait for all holders to release)
	//   Exclusive + any   → block (wait for all holders to release)
	// TODO: if compatible, add txnID to holders and return nil
	// TODO: if not compatible, add to waiters and block on the grant channel
	// TODO: after granting, check for deadlock (see detectDeadlock)
	panic("not implemented")
}

// Release removes the transaction from the holder set for the given table lock and then
// grants the lock to any waiting transactions that are now compatible, in FIFO order.
func (lm *LockManager) Release(txnID uint64, table string) {
	// TODO: remove txnID from the lock entry's holders
	// TODO: examine waiters in FIFO order and grant to compatible ones
}

// ReleaseAll releases every lock the given transaction holds, waking any waiters that
// can now be granted their requested lock mode.
func (lm *LockManager) ReleaseAll(txnID uint64) {
	// TODO: call Release(txnID, table) for every table this transaction holds a lock on
	// TODO: wake any waiters that can now be granted
}

// detectDeadlock builds a wait-for graph where an edge from transaction A to transaction B
// means A is blocked waiting for a lock that B currently holds. It detects cycles in this
// graph using depth-first search. For each cycle found, one victim transaction is chosen
// and rolled back to break the deadlock. This function runs periodically in a background
// goroutine rather than on every Acquire call to keep lock acquisition fast.
func (lm *LockManager) detectDeadlock() {
	// TODO: build a wait-for graph: edge txnA → txnB if txnA is waiting for a lock txnB holds
	// TODO: detect cycles using DFS
	// TODO: for each cycle: choose a victim transaction and call Rollback on it
	// NOTE: run this periodically in a background goroutine, not on every Acquire
}
