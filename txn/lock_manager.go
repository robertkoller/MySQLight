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

func NewLockManager() *LockManager {
	// TODO: initialise locks map
	panic("not implemented")
}

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

func (lm *LockManager) Release(txnID uint64, table string) {
	// TODO: remove txnID from the lock entry's holders
	// TODO: examine waiters in FIFO order and grant to compatible ones
}

func (lm *LockManager) ReleaseAll(txnID uint64) {
	// TODO: call Release(txnID, table) for every table this transaction holds a lock on
	// TODO: wake any waiters that can now be granted
}

func (lm *LockManager) detectDeadlock() {
	// TODO: build a wait-for graph: edge txnA → txnB if txnA is waiting for a lock txnB holds
	// TODO: detect cycles using DFS
	// TODO: for each cycle: choose a victim transaction and call Rollback on it
	// NOTE: run this periodically in a background goroutine, not on every Acquire
}
