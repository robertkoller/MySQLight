# MySQLight Executor

Phase 3 of MySQLight. The executor takes a parsed AST statement, builds a
pipeline of physical operators, and drives it to produce result rows or apply
mutations. It is the only layer that touches both the catalog (for schema) and
the storage layer (for data).

---

## Layering

```
        ┌─────────────────────────────┐
        │          REPL / API         │
        └──────────────┬──────────────┘
                       │ Statement (AST node)
        ┌──────────────▼──────────────┐
        │           Executor          │   executor.go — Execute / collectRows
        └──┬──────────────────────┬───┘
           │                      │
    schema │                 data │
           ▼                      ▼
      Catalog                 B+ Trees
  (GetTable, etc.)      (Insert / Delete / Scan)
```

---

## Operator interface

Every physical operator implements the same three-method iterator:

```go
type Operator interface {
    Open() error        // initialise state, open children
    Next() (Row, error) // return next row; io.EOF when done
    Close() error       // release resources
}

type Row []catalog.Value
```

`collectRows` drives any operator to completion:

```go
func collectRows(op Operator) ([]Row, error)
// Open → loop Next until io.EOF → Close → return rows
```

---

## Operator pipeline

A SQL query is compiled into a tree of operators. Each operator pulls rows from
its child on demand (volcano / pull model). `SELECT name FROM users WHERE age > 25
ORDER BY name LIMIT 10` becomes:

```
Limit(10)
  Sort(name ASC)
    Project([name])
      Filter(age > 25)
        TableScan(users)
```

### Operators

| Operator | File | Description |
|---|---|---|
| `TableScan` | scan.go | Walks all leaf pages of a data B+ tree in key order |
| `IndexScan` | scan.go | Walks an index B+ tree; fetches each row from the data tree by primary key |
| `Filter` | filter.go | Skips rows where the WHERE predicate returns false |
| `Project` | project.go | Evaluates the SELECT expression list; emits only those columns |
| `Sort` | project.go | Buffers all rows in `Open`, sorts by ORDER BY keys |
| `Limit` | project.go | Passes through the first N rows then returns `io.EOF` |
| `NestedLoopJoin` | join.go | For each left row, scans all right rows and emits matching pairs |
| `Aggregate` | aggregate.go | Materialises all rows in `Open`, groups by GROUP BY, computes aggregates |

### Pipeline order for SELECT

```
TableScan → Joins → Filter → Aggregate or (Sort → Project) → Limit
```

When the SELECT list contains an aggregate function (`COUNT`, `SUM`, `MIN`,
`MAX`, `AVG`) or there is a `GROUP BY`, `Aggregate` replaces the `Project` step.
`Sort` is placed before `Project` so `ORDER BY` can reference pre-projection
column names. `ORDER BY` on aggregate results is a known limitation — not yet
supported.

---

## Expression evaluation

`evalExpr` in `filter.go` is the shared expression evaluator used by Filter,
Project, Sort, NestedLoopJoin, and Aggregate.

```go
func evalExpr(expr parser.Expr, row Row, columns []catalog.ColumnDef) (interface{}, error)
```

Return types: `int64`, `float64`, `string`, `[]byte` for non-null values; `nil`
for NULL. Booleans are native Go `bool` (from comparisons and logical ops).

| Expression | Handled as |
|---|---|
| `Literal{Kind:"integer"}` | `strconv.ParseInt` → `int64` |
| `Literal{Kind:"float"}` | `strconv.ParseFloat` → `float64` |
| `Literal{Kind:"string"}` | `string` |
| `Literal{Kind:"null"}` | `nil` |
| `ColumnRef{Column:"age"}` | look up column index by name, return `row[i]` typed value |
| `BinaryExpr{Op:"=", ...}` | evaluate both sides; compare or compute |
| `UnaryExpr{Op:"NOT"}` | negate bool |
| `UnaryExpr{Op:"-"}` | negate number |
| `FuncCall` | error — only valid inside `Aggregate` |

Binary operator null propagation: any operation with a `nil` operand returns
`nil` (SQL three-valued logic). Exception: `AND`/`OR` with a non-null side
could be short-circuited, but currently both sides are always evaluated.

LIKE uses `%` as the only wildcard. `_` is not supported.

---

## Row serialisation

Rows are stored as `[]byte` in the B+ tree. `encodeRow` and `decodeRow` in
`row.go` handle the wire format:

```
[null bitmap: ceil(col_count / 8) bytes]
  bit i = 1 means column i is NULL

[for each non-null column in definition order]
  TypeInt:   int64 as uint64 big-endian (8 bytes)
  TypeFloat: float64 as IEEE-754 bits big-endian (8 bytes)
  TypeText:  uint32 length (4 bytes) + UTF-8 bytes
  TypeBlob:  uint32 length (4 bytes) + raw bytes
```

NULL columns contribute only their bitmap bit — nothing in the data region.

---

## DML execution

### INSERT

1. Build full row from `VALUES` expressions (`buildInsertRow`). Positional if
   no column list; named otherwise. Missing columns get their schema default or
   NULL.
2. Validate `NOT NULL` constraints.
3. Verify FK parent references (`verifyFKParents`).
4. `encodeRow` → `dataTree.Insert(pkBytes, rowBytes)`.
5. For each index: `indexTree.Insert(indexKeyBytes, pkBytes)`.

### UPDATE

1. Collect matching rows via `TableScan + Filter`.
2. Apply `SET` assignments (`evalExprToValue`).
3. Validate `NOT NULL` constraints.
4. Verify FK parent references for the new row.
5. If PK changed: apply `ON UPDATE` FK actions to child tables.
6. `dataTree.Delete(oldPK)` then `dataTree.Insert(newPK, newRowBytes)`.
7. Rebuild index entries for every index whose column was touched.

### DELETE

1. Collect matching rows via `TableScan + Filter`.
2. Delegate to `deleteRowsFromTable`, which for each row:
   - Applies `ON DELETE` FK actions to child tables (see FK section).
   - `dataTree.Delete(pkBytes)`.
   - `indexTree.Delete(indexKeyBytes)` for every index.

---

## Foreign key enforcement

`fk.go` implements all FK constraint logic.

### On INSERT and UPDATE — verifying parent references

```go
func (e *Executor) verifyFKParents(row Row, definition *catalog.TableDef) error
```

For each `ForeignKeyDef` in the table's schema, if the FK column is non-null,
`columnContainsValue` scans the parent table for a matching value in
`fk.RefColumn`. Returns an error on any violation.

### On DELETE — applying child actions

```go
func (e *Executor) applyOnDelete(parentTable string, pkValue catalog.Value) error
```

`catalog.ListTables()` enumerates every table. For each that has a
`ForeignKeyDef` pointing at `parentTable`, `findRowsMatchingValue` collects
matching child rows, then:

| `OnDelete` | Action |
|---|---|
| `FKRestrict` | Return error immediately; no data modified |
| `FKCascade` | Call `deleteRowsFromTable` on the child rows (recursive) |
| `FKSetNull` | Call `updateColumnInRows` with `Value{IsNull: true}` |

### On UPDATE — cascading PK changes to children

```go
func (e *Executor) applyOnUpdate(parentTable string, oldPK, newPK catalog.Value, pkType catalog.DataType) error
```

Same scan as `applyOnDelete`. If the parent's PK changed, child FK columns are
updated to the new value (CASCADE) or nulled out (SET NULL). RESTRICT blocks
the update if any child row exists.

### Shared helpers

| Function | Purpose |
|---|---|
| `deleteRowsFromTable` | Shared deletion path for both `executeDelete` and CASCADE |
| `updateColumnInRows` | Rewrites one column in a row set; updates that column's index |
| `findRowsMatchingValue` | TableScan + manual equality check; returns matching rows |
| `valuesEqual(a, b, type)` | Type-aware equality; NULL ≠ anything |

---

## DDL execution

DDL statements in `Execute` delegate directly to the catalog:

| Statement | Catalog call |
|---|---|
| `CREATE TABLE` | `astToTableDef` → `catalog.CreateTable` |
| `DROP TABLE` | `catalog.DropTable` (also drops all indexes) |
| `CREATE INDEX` | `catalog.CreateIndex` |
| `DROP INDEX` | `catalog.DropIndex` |

`astToTableDef` converts parser AST types (`ColumnDefAST`, `ForeignKeyAST`) to
catalog types (`ColumnDef`, `ForeignKeyDef`). Type name strings like `"INTEGER"`
are mapped to `catalog.DataType` constants by `parseDataType`.

---

## Phase 3 status — complete

| Component | Status |
|---|---|
| `Operator` interface + `collectRows` | Done |
| `TableScan` / `IndexScan` | Done |
| `Filter` + `evalExpr` (all expression types) | Done |
| `Project` / `Sort` / `Limit` | Done |
| `NestedLoopJoin` | Done |
| `Aggregate` (GROUP BY, COUNT/SUM/MIN/MAX/AVG) | Done |
| Row serialisation (`encodeRow` / `decodeRow`) | Done |
| `INSERT` (positional + named columns, defaults) | Done |
| `UPDATE` (SET + WHERE, index maintenance) | Done |
| `DELETE` (WHERE, index maintenance) | Done |
| FK enforcement (RESTRICT / CASCADE / SET NULL) | Done |
| DDL dispatch (CREATE/DROP TABLE/INDEX) | Done |
| Tests (26 cases) | Done |

### Not in Phase 3 (later phases)

- WAL before-image records on mutations — Phase 4
- Transaction BEGIN / COMMIT / ROLLBACK — Phase 4
- Table-level shared/exclusive locking — Phase 4
- Index selection by the query planner — Phase 5
- ORDER BY on aggregate result sets — Phase 5
- `IN (...)` and subquery expressions — stretch goal
