# MySQLight

A relational database engine written from scratch in Go. No external dependencies — the B+ tree, SQL parser, write-ahead log, and transaction manager are all hand-built. Think SQLite-class embedded database.

---

## Getting Started

**Requirements:** Go 1.21+

```bash
git clone https://github.com/robertkoller/MySQLight
cd MySQLight
go build ./...
```

### Start the REPL

```bash
go run main.go mydata.db
```

This opens (or creates) `mydata.db` and drops you into an interactive shell:

```
MySQLight> CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, age INTEGER);
MySQLight> INSERT INTO users (id, name, age) VALUES (1, 'Alice', 30);
MySQLight> SELECT * FROM users WHERE age > 25;
MySQLight> .exit
```

### Use as a Library

```go
import "github.com/robertkoller/MySQLight"

db, err := mysqlight.Open("mydata.db")
rows, err := db.Query("SELECT name FROM users WHERE age > ?", 25)
```

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
ROLLBACK;

-- Statistics (used by the query planner)
ANALYZE users;
```

---

## How It Works

The engine is built in five layers, each testable in isolation:

| Layer | What it does |
|---|---|
| **Storage** | Pages, buffer pool, B+ tree. Every table and index is a B+ tree stored in a single `.db` file. |
| **Parser** | Lexer + recursive descent parser. Turns a SQL string into an AST. |
| **Executor** | Walks the AST, talks to the catalog to find tables, and runs iterator operators (scan → filter → project → sort). |
| **WAL + Transactions** | Write-ahead log for crash recovery. ACID transactions with two-phase locking. |
| **Planner** | Converts the AST to a logical plan, applies rewrites (predicate pushdown, constant folding), and picks indexes using a simple cost model. |

Data flow:

```
SQL string
    → Lexer → tokens
    → Parser → AST
    → Planner → logical plan
    → Executor → rows
    → Storage (B+ tree, buffer pool, pager) → disk
```

Everything lives in two files: `mydata.db` (all tables, indexes, and the catalog) and `mydata.db.wal` (the write-ahead log, truncated after a clean shutdown).

---

## Project Structure

```
MySQLight/
├── main.go          — REPL entry point
├── storage/         — pager, buffer pool, B+ tree
├── wal/             — write-ahead log, crash recovery
├── catalog/         — table & index definitions
├── parser/          — lexer, AST, recursive descent parser
├── executor/        — query operators (scan, filter, join, aggregate, ...)
├── txn/             — transactions, lock manager
└── planner/         — logical plan, rule-based optimizer, index selection
```

---

## Running Tests

```bash
go test ./...
```

Each package has its own test file. The storage tests insert thousands of rows and verify they survive a process restart. The WAL tests simulate mid-transaction crashes.

---

## Status

Under active development. See `notes.md` for the full build plan and phase breakdown.
