# MySQLight

A relational database engine written from scratch in Go. No external dependencies — the B+ tree, SQL parser, write-ahead log, transaction manager, and query planner are all hand-built. Think SQLite-class embedded database.

---

## What's built

All five phases are complete and fully tested (238 tests across 7 packages, 0 failures).

| Layer | Package | What it does |
|---|---|---|
| **Storage** | `storage/` | Fixed-size pages, LRU buffer pool, B+ tree. Every table and index is a B+ tree in a single `.db` file. |
| **Parser** | `parser/` | Lexer + recursive descent parser. Turns a SQL string into an AST. |
| **Catalog** | `catalog/` | Table and index definitions, persisted in a system B+ tree. |
| **Executor** | `executor/` | Iterator operator pipeline. Runs SELECT/INSERT/UPDATE/DELETE against the catalog and storage layer. FK enforcement, DDL, transactions. |
| **WAL** | `wal/` | Write-ahead log with CRC32 checksums. ARIES-style crash recovery: combined analysis+redo pass, then undo pass. |
| **Transactions** | `txn/` | ACID transactions, table-level two-phase locking, deadlock detection via wait-for graph. |
| **Planner** | `planner/` | AST → logical plan tree. Rule-based optimizer: predicate pushdown, constant folding, column pruning, index selection. |

---

## Supported SQL

```sql
-- Tables
CREATE TABLE users (
    id       INTEGER PRIMARY KEY,
    name     TEXT NOT NULL,
    age      INTEGER
);
CREATE TABLE orders (
    id      INTEGER PRIMARY KEY,
    user_id INTEGER,
    total   FLOAT,
    FOREIGN KEY (user_id) REFERENCES users (id) ON DELETE CASCADE
);
DROP TABLE users;

-- Indexes
CREATE INDEX idx_age ON users (age);
CREATE UNIQUE INDEX idx_name ON users (name);
DROP INDEX idx_age;

-- Queries
INSERT INTO users (id, name, age) VALUES (1, 'Alice', 30), (2, 'Bob', 25);
SELECT id, name FROM users WHERE age > 25 ORDER BY name DESC LIMIT 10;
SELECT COUNT(*), AVG(age) FROM users WHERE age > 18 GROUP BY age;
SELECT u.name, o.total FROM users u JOIN orders o ON u.id = o.user_id;
UPDATE users SET age = 31 WHERE id = 1;
DELETE FROM users WHERE age < 18;

-- Transactions
BEGIN;
INSERT INTO orders (id, user_id, total) VALUES (100, 1, 49.99);
UPDATE users SET age = age + 1 WHERE id = 1;
COMMIT;

BEGIN;
DELETE FROM users WHERE id = 2;
ROLLBACK;
```

---

## How it works

Data flow through the engine:

```
SQL string
    → Lexer → tokens
    → Parser → AST
    → Planner → logical plan tree
    → Optimizer → rewritten plan (predicate pushdown, index selection, ...)
    → Executor → rows / mutations
    → Storage (B+ tree, buffer pool, pager) → disk
    → WAL → .wal file (written before any page hits disk)
```

Everything lives in two files: `mydata.db` (all tables, indexes, and the system catalog) and `mydata.db.wal` (the write-ahead log, truncated on clean shutdown).

---

## Using as a library

```go
import mysqlight "github.com/robertkoller/MySQLight"

db, err := mysqlight.Open("myapp.db")
if err != nil {
    log.Fatal(err)
}
defer db.Close()

// DDL and DML
db.Exec("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, age INTEGER)")
db.Exec("INSERT INTO users (id, name, age) VALUES (1, 'Alice', 30), (2, 'Bob', 25)")

// Query
result, err := db.Exec("SELECT id, name, age FROM users WHERE age > 25")
for _, row := range result.Rows {
    for i, val := range row {
        fmt.Printf("%s: %s\n", result.Columns[i], mysqlight.FormatValue(val, result.ColumnTypes[i]))
    }
}

// Transactions
db.Exec("BEGIN")
db.Exec("UPDATE users SET age = 31 WHERE id = 1")
db.Exec("ROLLBACK") // or COMMIT
```

---

## Running the REPL

```bash
go run ./cmd/mysqlight mydata.db
```

```
MySQLight  mydata.db
Type SQL statements ending with ; or .exit to quit.

mysqlight> SELECT id, name FROM users WHERE age > 25;
id    name
----  -----
1     Alice

(1 row)
mysqlight> .exit
```

---

## Project structure

```
MySQLight/
├── db.go            — public library API (Open, Exec, Close)
├── db_test.go       — integration tests for the library API
├── cmd/
│   └── mysqlight/
│       └── main.go  — interactive REPL
├── storage/         — pager, buffer pool, B+ tree, freelist
│   ├── pager.go
│   ├── buffer_pool.go
│   ├── btree.go
│   ├── btree_node.go
│   ├── freelist.go
│   └── storage_test.go
├── wal/             — write-ahead log, crash recovery
│   ├── wal.go
│   ├── recovery.go
│   └── wal_test.go
├── catalog/         — table & index metadata
│   ├── catalog.go
│   ├── schema.go
│   ├── encoding.go
│   └── catalog_test.go
├── parser/          — lexer, AST node types, recursive descent parser
│   ├── lexer.go
│   ├── ast.go
│   ├── parser.go
│   └── parser_test.go
├── executor/        — operator pipeline, DML, DDL, FK enforcement
│   ├── executor.go
│   ├── scan.go
│   ├── filter.go
│   ├── project.go
│   ├── join.go
│   ├── aggregate.go
│   ├── insert.go
│   ├── update.go
│   ├── delete.go
│   ├── fk.go
│   └── executor_test.go
├── txn/             — transactions, lock manager, deadlock detection
│   ├── txn.go
│   ├── lock_manager.go
│   └── txn_test.go
├── planner/         — logical plan, rule-based optimizer
│   ├── planner.go
│   ├── optimizer.go
│   └── planner_test.go
└── go.mod
```

---

## Running the tests

```bash
go test ./...
```

238 tests across 7 packages. Includes:
- B+ tree insert/delete/scan with durability verification (restarts the pager mid-test)
- WAL checksum corruption detection and crash recovery simulation
- Transaction rollback restoring page state
- Lock compatibility, blocking, FIFO grant order, deadlock detection
- Full SQL end-to-end: DDL, DML, FK enforcement, aggregate queries, joins
- Optimizer: constant folding, predicate push-down, index selection

---

## Design notes

**No dependencies.** Standard library only (`encoding/binary`, `hash/crc32`, `sync`, `os`, `io`, `container/list`).

**Page size = 4096 bytes.** Matches OS virtual memory pages. All reads and writes are whole pages.

**Single-file database.** Everything lives in one `.db` file. The WAL is a separate `.wal` file — the same layout as SQLite.

**B+ tree as the universal data structure.** Tables, indexes, and the system catalog are all B+ trees. Leaf nodes form a linked list for O(1) range scan setup.

**WAL protocol: log before data.** The buffer pool writes a WAL UPDATE record (before-image + after-image + CRC32) before any dirty page is flushed to disk. Recovery replays the log forward (redo), then reverses losers (undo).

**Strict two-phase locking.** Table-level S/X locks. Scans acquire shared locks in `Open()`; DML acquires exclusive locks before the first mutation. Locks are held until `COMMIT` or `ROLLBACK` — never released mid-transaction.

**Before-images in the buffer pool.** When a page is first dirtied in a transaction, the buffer pool captures a before-image (taken from the clean snapshot recorded at page load time). `RollbackTxn` restores before-images, either from the in-memory cache or by writing back to the pager if the page was evicted.

---

## Stretch goals

- Wire planner output into executor (physical plan builder instead of direct AST dispatch)
- Row-level locking / MVCC
- Hash join and merge join
- Subqueries and CTEs
- `ANALYZE` statistics and cost-model index selection
- TCP wire protocol (PostgreSQL-compatible)
