# MySQLight — Build Plan

A relational database engine written from scratch in Go. No external dependencies beyond the standard library. The goal is a fully working database that can parse SQL, store data durably on disk, recover from crashes, and handle concurrent transactions correctly.

---

## What We're Building

A SQLite-class embedded database engine with:
- A B+ tree storage engine with page-based disk I/O
- A write-ahead log (WAL) for crash recovery
- ACID transactions with locking
- A SQL parser (lexer + recursive descent parser) for a useful subset of SQL
- A query executor with table scans, index scans, filtering, projection, and joins
- A REPL shell and a Go API so the engine can be embedded in other programs

Name: **MySQLight** — lives between MySQL and SQLite in spirit.

---

## Architecture Overview

```
  User / REPL
      │
      ▼
  SQL Parser          ← Phase 2
  (Lexer → AST)
      │
      ▼
  Query Planner       ← Phase 5
  (AST → Plan)
      │
      ▼
  Query Executor      ← Phase 3
  (Plan → Rows)
      │
      ├── Catalog Manager (schema, table defs)
      │
      └── Storage Engine             ← Phase 1
            ├── Buffer Pool (page cache)
            ├── B+ Tree (table + index storage)
            └── WAL (crash recovery)   ← Phase 4
```

Every layer is testable in isolation. The executor doesn't know about disk; the storage engine doesn't know about SQL.

---

## File Structure

```
MySQLight/
├── notes.md
├── go.mod
├── main.go                  ← REPL entry point
│
├── storage/
│   ├── pager.go             ← page I/O, file management
│   ├── buffer_pool.go       ← LRU page cache
│   ├── btree.go             ← B+ tree core
│   ├── btree_node.go        ← page layout for internal/leaf nodes
│   ├── freelist.go          ← free page tracking
│   └── storage_test.go
│
├── wal/
│   ├── wal.go               ← write-ahead log writer/reader
│   ├── recovery.go          ← ARIES-style redo/undo on startup
│   └── wal_test.go
│
├── catalog/
│   ├── catalog.go           ← table & index definitions, persisted in a system table
│   ├── schema.go            ← column types, constraints
│   └── catalog_test.go
│
├── parser/
│   ├── lexer.go             ← tokenizer
│   ├── ast.go               ← AST node types
│   ├── parser.go            ← recursive descent parser
│   └── parser_test.go
│
├── executor/
│   ├── executor.go          ← top-level dispatch
│   ├── scan.go              ← table scan, index scan
│   ├── filter.go            ← WHERE evaluation
│   ├── project.go           ← SELECT column list
│   ├── join.go              ← nested-loop join
│   ├── aggregate.go         ← COUNT, SUM, MIN, MAX, AVG
│   ├── insert.go
│   ├── update.go
│   ├── delete.go
│   └── executor_test.go
│
├── txn/
│   ├── txn.go               ← transaction state, BEGIN/COMMIT/ROLLBACK
│   ├── lock_manager.go      ← shared/exclusive table locks, 2PL
│   └── txn_test.go
│
└── planner/                 ← Phase 5, added last
    ├── planner.go           ← AST → logical plan
    ├── optimizer.go         ← rule-based rewrites
    └── planner_test.go
```

---

## Phase 1 — Storage Engine

**Goal:** Read and write pages to disk. Implement a B+ tree that can insert, delete, and look up rows by key.

### 1a. Pager

The pager owns the database file. Everything is divided into fixed-size **pages** (4096 bytes — matches the OS page size). The pager exposes:

```go
type Pager struct { /* file handle, page count */ }

func (p *Pager) ReadPage(pageID uint32) ([]byte, error)
func (p *Pager) WritePage(pageID uint32, data []byte) error
func (p *Pager) AllocatePage() (uint32, error)
func (p *Pager) FreePage(pageID uint32) error
func (p *Pager) PageCount() uint32
```

Page 0 is the **database header**: magic bytes, page size, root page ID of the system catalog, WAL sequence number.

### 1b. Buffer Pool

Reading from disk on every access is too slow. The buffer pool keeps recently-used pages in memory and writes them back lazily.

- Fixed capacity (e.g., 64 pages to start)
- **LRU eviction**: when full, evict the least-recently-used clean page. If the evicted page is dirty (modified but not yet written), flush it to disk first.
- **Pin/unpin**: callers pin a page before use so the pool never evicts a page that's in active use.

```go
type BufferPool struct { /* frames, lru list, pin counts */ }

func (bp *BufferPool) FetchPage(pageID uint32) (*Page, error)  // pin + return
func (bp *BufferPool) UnpinPage(pageID uint32, dirty bool)
func (bp *BufferPool) FlushAll() error
```

### 1c. B+ Tree

This is the core data structure. Every table is a B+ tree keyed by its primary key. Every index is also a B+ tree.

**Why B+ tree and not B-tree?**
- Leaf nodes form a linked list → range scans are fast (no backtracking through the tree)
- Internal nodes only store keys (no values) → more keys fit per page → shorter tree

**Page layout for a leaf node:**
```
[page type: 1B] [key count: 2B] [right sibling page ID: 4B]
[slot array: key count × (key offset: 2B, value offset: 2B)]
[... free space ...]
[values grow left ← ... keys grow right →]
```

**Page layout for an internal node:**
```
[page type: 1B] [key count: 2B]
[child page IDs: (key count + 1) × 4B]
[key slot array: key count × (key offset: 2B)]
[... keys ...]
```

**Operations to implement:**
- `Insert(key, value []byte) error` — walk tree, find leaf, insert. If leaf overflows: split leaf and push median key up to parent. If parent overflows: split parent too (recurse up). If root splits: new root.
- `Delete(key []byte) error` — find leaf, remove entry. If leaf underflows (< half full): try to borrow from sibling. If sibling is too small: merge, remove separator from parent (recurse up).
- `Get(key []byte) ([]byte, error)` — traverse internal nodes, read leaf.
- `Scan(start, end []byte) Iterator` — find start leaf, walk right sibling chain.

**Key milestone:** A standalone B+ tree that stores arbitrary `[]byte` key-value pairs on disk and survives process restart.

---

## Phase 2 — SQL Parser

**Goal:** Turn a SQL string into an AST that the executor can walk.

### 2a. Lexer

Splits the input string into tokens. Token types:

```
Keywords:  SELECT FROM WHERE INSERT INTO VALUES UPDATE SET DELETE
           CREATE TABLE DROP INDEX ON PRIMARY KEY NOT NULL DEFAULT
           BEGIN COMMIT ROLLBACK AND OR NOT LIKE ORDER BY LIMIT
           JOIN INNER LEFT GROUP HAVING COUNT SUM MIN MAX AVG

Literals:  INTEGER  FLOAT  STRING  IDENTIFIER

Punctuation: ( ) , ; . * = < > <= >= != + - / %
```

The lexer is a simple state machine: skip whitespace, read the next character, decide which token type based on the first character, then read until the token ends.

### 2b. AST Nodes

```go
type Statement interface{ stmtNode() }

type SelectStmt struct {
    Columns  []Expr          // * or list of expressions
    From     string          // table name
    Joins    []JoinClause
    Where    Expr
    GroupBy  []Expr
    Having   Expr
    OrderBy  []OrderClause
    Limit    *int
}

type InsertStmt struct {
    Table   string
    Columns []string
    Values  [][]Expr
}

type UpdateStmt struct {
    Table  string
    Set    []Assignment
    Where  Expr
}

type DeleteStmt struct {
    Table string
    Where Expr
}

type CreateTableStmt struct {
    Table   string
    Columns []ColumnDef
}

type CreateIndexStmt struct {
    Index  string
    Table  string
    Column string
    Unique bool
}

type Expr interface{ exprNode() }

// Expr subtypes:
// BinaryExpr (left op right), UnaryExpr, ColumnRef, Literal,
// FuncCall (COUNT, SUM, ...), StarExpr
```

### 2c. Parser

Recursive descent — one function per grammar rule.

```
parseStatement  → parseSelect | parseInsert | parseUpdate | parseDelete
                  | parseCreate | parseDrop | parseBegin | parseCommit | parseRollback

parseSelect     → SELECT columns FROM table [JOIN ...] [WHERE expr]
                  [GROUP BY exprs] [HAVING expr] [ORDER BY exprs] [LIMIT n]

parseExpr       → parseOr
parseOr         → parseAnd (OR parseAnd)*
parseAnd        → parseNot (AND parseNot)*
parseNot        → NOT parseComparison | parseComparison
parseComparison → parseAddSub (( = | != | < | > | <= | >= | LIKE ) parseAddSub)?
parseAddSub     → parseMulDiv (( + | - ) parseMulDiv)*
parseMulDiv     → parseUnary (( * | / | % ) parseUnary)*
parseUnary      → - parsePrimary | parsePrimary
parsePrimary    → INTEGER | FLOAT | STRING | IDENTIFIER | funcCall | ( parseExpr )
```

**Key milestone:** Parse every statement type and round-trip it to a string representation for testing.

---

## Phase 3 — Catalog + Query Executor

**Goal:** Execute SELECT, INSERT, UPDATE, DELETE against real tables stored in the B+ tree.

### 3a. Catalog

The catalog stores table and index definitions. It is itself a table in the B+ tree (stored in a reserved page range at the start of the file).

```go
type TableDef struct {
    Name    string
    Columns []ColumnDef
    RootPageID uint32       // root of this table's B+ tree
}

type ColumnDef struct {
    Name       string
    Type       DataType     // INT, FLOAT, TEXT, BLOB
    PrimaryKey bool
    NotNull    bool
    Default    Value
}

type IndexDef struct {
    Name       string
    TableName  string
    ColumnName string
    Unique     bool
    RootPageID uint32
}
```

On startup: read the catalog table → rebuild `TableDef` and `IndexDef` maps in memory.

### 3b. Row Serialization

Rows are serialized to `[]byte` before being stored as values in the B+ tree.

Format:
```
[null bitmap: ceil(col_count/8) bytes]
[col 0 value][col 1 value]...[col N value]

Fixed-width types (INT=8B, FLOAT=8B) stored inline.
Variable-width types (TEXT, BLOB) stored as [length: 4B][bytes].
```

### 3c. Executor Operators

The executor is a pipeline of **iterator operators** — each operator implements:

```go
type Operator interface {
    Open() error
    Next() (Row, error)   // io.EOF when done
    Close() error
}
```

Operators:

| Operator | Description |
|---|---|
| `TableScan` | Iterate every row in a B+ tree leaf chain |
| `IndexScan` | Traverse a B+ tree index, fetch rows by key |
| `Filter` | Wrap another operator, skip rows where WHERE is false |
| `Project` | Wrap another operator, keep only selected columns, evaluate expressions |
| `NestedLoopJoin` | For each row from left, scan right looking for matches |
| `Aggregate` | Consume all rows, compute COUNT/SUM/MIN/MAX/AVG per group |
| `Sort` | Buffer all rows, sort by ORDER BY keys |
| `Limit` | Stop after N rows |

`SELECT name, age FROM users WHERE age > 25 ORDER BY name LIMIT 10` becomes:

```
Limit(10)
  Sort(name ASC)
    Project(name, age)
      Filter(age > 25)
        TableScan(users)
```

### 3d. INSERT / UPDATE / DELETE

- **INSERT**: serialize row → B+ tree insert. Update all indexes.
- **UPDATE**: table scan with filter → for each matching row: delete old value, insert new value, update indexes.
- **DELETE**: table scan with filter → for each matching row: B+ tree delete, remove from all indexes.

**Key milestone:** A working database that can CREATE TABLE, INSERT rows, SELECT with WHERE/ORDER BY/LIMIT, UPDATE, and DELETE. All data survives restart.

---

## Phase 4 — WAL & Transactions

**Goal:** Crash recovery and ACID transactions.

### 4a. Write-Ahead Log

Before any page is modified on disk, the change is written to the WAL file first. On crash, the WAL is replayed to bring the database to a consistent state.

**WAL record format:**
```
[LSN: 8B]           ← Log Sequence Number, monotonically increasing
[TxnID: 8B]
[record type: 1B]   ← BEGIN, COMMIT, ABORT, UPDATE
[page ID: 4B]       ← for UPDATE records
[before image: 4096B] ← page contents before change
[after image: 4096B]  ← page contents after change
[checksum: 4B]      ← CRC32 of everything above
```

The buffer pool's `UnpinPage` writes a WAL UPDATE record **before** writing the dirty page to the database file. This is the **WAL protocol**: log before data.

### 4b. Recovery (ARIES simplified)

On startup, if the WAL file is non-empty:

1. **Analysis pass**: scan WAL forward from the last checkpoint. Build a set of transactions that were active at crash time (started but never COMMITted or ABORTed).
2. **Redo pass**: re-apply every UPDATE record from the WAL, restoring the database to the exact state it was in at the moment of crash.
3. **Undo pass**: for every transaction that was active at crash time (never committed), apply the before-images in reverse LSN order to roll back their partial changes.

After recovery, truncate the WAL and write a new checkpoint.

### 4c. Transaction Manager

```go
type Txn struct {
    ID        uint64
    State     TxnState     // active, committed, aborted
    UndoLog   []UndoEntry  // for rollback
}

func Begin() *Txn
func (t *Txn) Commit() error
func (t *Txn) Rollback() error
```

### 4d. Lock Manager

Two-phase locking (2PL): a transaction acquires all locks before releasing any.

- **Shared lock (S)**: multiple transactions can hold simultaneously. Required to read a table.
- **Exclusive lock (X)**: only one transaction can hold. Required to write a table.
- Lock compatibility: S+S allowed, S+X blocked, X+X blocked.

Start with **table-level locking** (simple, correct). Row-level locking is a stretch goal.

Deadlock detection: a background goroutine periodically checks the wait-for graph and aborts one transaction in each cycle.

**Key milestone:** `BEGIN; INSERT ...; INSERT ...; COMMIT;` works. `BEGIN; INSERT ...; ROLLBACK;` leaves the table unchanged. A simulated crash mid-transaction leaves the table unchanged after recovery.

---

## Phase 5 — Query Planner

**Goal:** Choose a smarter execution plan based on available indexes.

### 5a. Logical Plan

Convert the AST into a tree of logical operators (same shape as the executor but abstract — no physical decisions yet):

```
LogicalFilter → LogicalScan → LogicalProject
```

### 5b. Rule-Based Optimizer

Apply rewrite rules before choosing physical operators:

- **Predicate pushdown**: move Filter nodes as close to the scan as possible so fewer rows travel up the tree.
- **Constant folding**: evaluate `2 + 3` at plan time, not per-row.
- **Column pruning**: only load columns that are actually referenced.

### 5c. Index Selection

For each `Filter(col = value)` or `Filter(col BETWEEN a AND b)` over a table that has an index on `col`, replace `TableScan → Filter` with `IndexScan`. The planner checks the catalog for matching indexes.

Simple cost model:
- `TableScan` cost = number of pages in table
- `IndexScan` cost = index height + estimated matching rows (from basic statistics)

Statistics: on `ANALYZE table`, count rows and store min/max/distinct counts per column in the catalog.

**Key milestone:** `SELECT * FROM orders WHERE customer_id = 42` uses the index on `customer_id` instead of scanning the whole table.

---

## SQL Subset Supported

By the end of all phases:

```sql
-- DDL
CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, age INTEGER);
CREATE INDEX idx_age ON users (age);
DROP TABLE users;
DROP INDEX idx_age;

-- DML
INSERT INTO users (id, name, age) VALUES (1, 'Alice', 30), (2, 'Bob', 25);
SELECT id, name FROM users WHERE age > 25 ORDER BY name DESC LIMIT 10;
SELECT COUNT(*), AVG(age) FROM users WHERE age > 18;
SELECT u.name, o.total FROM users u JOIN orders o ON u.id = o.user_id;
UPDATE users SET age = 31 WHERE id = 1;
DELETE FROM users WHERE age < 18;

-- Transactions
BEGIN;
INSERT INTO orders (id, user_id, total) VALUES (100, 1, 49.99);
UPDATE users SET balance = balance - 49.99 WHERE id = 1;
COMMIT;

BEGIN;
DELETE FROM users WHERE id = 2;
ROLLBACK;  -- user 2 is back

-- Utility
ANALYZE users;
```

---

## Build Order

| Phase | What | Validates With |
|-------|------|----------------|
| 1 | B+ tree + buffer pool + pager | Unit tests: insert 10k rows, restart, scan all |
| 2 | SQL parser | Unit tests: parse every statement type, check AST |
| 3 | Catalog + executor (no WAL) | Integration: CREATE → INSERT → SELECT in memory |
| 4 | WAL + transactions | Crash simulation tests, rollback tests |
| 5 | Query planner | EXPLAIN output, benchmark index vs scan |

Each phase merges to main in a working state — no half-built systems sitting in the tree.

---

## Key Design Decisions

**No dependencies.** Standard library only. This means writing the B+ tree, the parser, the WAL, everything from scratch — which is the point.

**Page size = 4096 bytes.** Matches OS virtual memory pages. Reads/writes are always whole pages.

**Keys are `[]byte`.** The B+ tree is agnostic about types. The executor handles type-aware comparison before calling into storage.

**Primary key is always the B+ tree key.** If no PRIMARY KEY is declared, a hidden auto-increment integer column is added. Rows are stored in primary key order (clustered index).

**Single-file database.** Everything (all tables, all indexes, the catalog) lives in one `.db` file. The WAL is a second file with the same name + `.wal` suffix, identical to SQLite's approach.

**Go API alongside the REPL.** The engine is usable as a library:
```go
db, _ := mysqlight.Open("mydata.db")
rows, _ := db.Query("SELECT name FROM users WHERE age > ?", 25)
```

---

## Stretch Goals (after all 5 phases)

- **Row-level locking** instead of table-level (MVCC or lock-per-row)
- **Hash join** and **merge join** in addition to nested-loop
- **Subqueries** in WHERE and FROM
- **Views** (stored SELECT statements)
- **TCP wire protocol** (PostgreSQL-compatible enough to connect with `psql`)
- **Write buffer / LSM tree** as an alternative to B+ tree for write-heavy workloads
