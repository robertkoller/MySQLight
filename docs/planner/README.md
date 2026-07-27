# MySQLight Query Planner

Phase 5 of MySQLight. The planner converts a parsed `SelectStmt` AST into a tree of logical plan nodes, then applies four rule-based optimizer passes to produce a cheaper equivalent plan.

---

## Layering

```
        ┌─────────────────────────────┐
        │         Parser (AST)        │
        └──────────────┬──────────────┘
                       │ *parser.SelectStmt
        ┌──────────────▼──────────────┐
        │           Planner           │   planner.go — planSelect
        │  (AST → logical plan tree)  │
        └──────────────┬──────────────┘
                       │ LogicalNode tree
        ┌──────────────▼──────────────┐
        │          Optimizer          │   optimizer.go — Optimize
        │  (rewrite → smarter plan)   │
        └──────────────┬──────────────┘
                       │ LogicalNode tree (optimized)
                       ▼
              Physical executor
              (planner→executor integration pending)
```

The planner and executor are decoupled. The planner knows nothing about pages or B+ trees; the optimizer knows nothing about physical operators.

---

## Logical node types

Every node in the plan tree implements `LogicalNode`:

```go
type LogicalNode interface {
    logicalNode()
    Children() []LogicalNode
}
```

| Node | Source | Description |
|---|---|---|
| `LogicalScan` | Leaf | Full table scan; carries resolved `[]catalog.ColumnDef` |
| `LogicalIndexScan` | Leaf | Range or point scan via a named index; carries `Exact`, `Low`, `High` bounds |
| `LogicalFilter` | 1 child | WHERE or HAVING predicate |
| `LogicalProject` | 1 child | SELECT column list (including `StarExpr`) |
| `LogicalJoin` | 2 children | Inner join with an ON condition |
| `LogicalAggregate` | 1 child | GROUP BY + aggregate functions (COUNT/SUM/MIN/MAX/AVG) |
| `LogicalSort` | 1 child | ORDER BY |
| `LogicalLimit` | 1 child | LIMIT N |

---

## Planner — `planSelect`

`planSelect` builds the tree bottom-up. Each SQL clause adds exactly one wrapper node around the output of the previous step:

```
1. LogicalScan            — resolves table name in catalog, copies column definitions
2. LogicalFilter          — WHERE clause (if present)
3. LogicalJoin (repeated) — each JOIN clause wraps the previous output on the left
4. LogicalAggregate       — if GROUP BY is present or any SELECT column contains COUNT/SUM/…
   LogicalFilter          — HAVING clause (if present, sits above the aggregate)
   LogicalProject         — otherwise (SELECT column list)
5. LogicalSort            — ORDER BY (if present)
6. LogicalLimit           — LIMIT (if present)
```

The resulting tree is always left-deep: in a multi-table join, each successive join's left child is the accumulation of all previous joins.

### Aggregate detection

`containsAggregate` walks the SELECT expression list. If any expression contains a `FuncCall` node whose name is `COUNT`, `SUM`, `MIN`, `MAX`, or `AVG`, the plan uses `LogicalAggregate` instead of `LogicalProject`.

---

## Optimizer — four passes

`Optimize(root)` runs the four passes in order. Each pass is pure: it rewrites the tree without executing any queries.

### Pass 1 — `predicatePushdown`

Moves `LogicalFilter` nodes closer to their `LogicalScan`. When a filter sits directly above a `LogicalJoin` and its predicate only references columns from one side of the join (determined by `ColumnRef.Table`), the filter is pushed below the join onto that side:

```
Before:  Filter(users.age > 25) → Join → [Scan(users), Scan(orders)]
After:   Join → [Filter(users.age > 25) → Scan(users), Scan(orders)]
```

This reduces the number of rows that reach the join operator. Predicates with unqualified column references (no `Table` prefix) or cross-table references remain above the join.

### Pass 2 — `constantFolding`

Evaluates constant subexpressions at plan time via `foldExpr`. If both operands of a `BinaryExpr` are `Literal` nodes, the expression is computed and replaced with a single `Literal`:

| Input | Result |
|---|---|
| `2 + 3` | `Literal{integer, "5"}` |
| `3 > 10` | `Literal{integer, "0"}` (false) |
| `2 + 3 > 4` | `Literal{integer, "1"}` (true) |
| `1 AND 0` | `Literal{integer, "0"}` |
| `NULL + 5` | `Literal{null, "null"}` |

Boolean results use SQLite convention: `1` = true, `0` = false. Division and modulo by zero are left unfolded.

### Pass 3 — `columnPruning`

Removes columns from `LogicalScan.Columns` that are not referenced anywhere above the scan. A column is kept if:
- it is the primary key (always needed for row identity), or
- its name appears in any `ColumnRef` in a `LogicalFilter`, `LogicalProject`, `LogicalJoin`, `LogicalAggregate`, or `LogicalSort` node.

If `StarExpr` appears anywhere in the tree, all columns are retained and pruning is skipped.

### Pass 4 — `indexSelection`

Finds `LogicalFilter` nodes that sit directly above a `LogicalScan` and have an index-eligible predicate. An eligible predicate has the form `col op value` or `value op col` where `op` is `=`, `>`, `>=`, `<`, or `<=` and the column appears in `catalog.IndexesForTable`:

```
Before: Filter(age = 30) → Scan(users)
After:  LogicalIndexScan{Index:"idx_age", Column:"age", Exact:Literal{30}}
```

`extractIndexPredicate` normalises `value op col` forms by flipping the operator (e.g. `30 < age` → `age > 30`).

---

## API

```go
// Planner
planner := planner.NewPlanner(cat)
root, err := planner.Plan(stmt)       // stmt must be *parser.SelectStmt

// Optimizer
optimizer := planner.NewOptimizer(cat)
optimized, err := optimizer.Optimize(root)
```

---

## Phase 5 status

| Component | Status |
|---|---|
| All 8 `LogicalNode` types + `LogicalIndexScan` | Done |
| `planSelect` — full bottom-up tree construction | Done |
| Aggregate detection (`COUNT`/`SUM`/`MIN`/`MAX`/`AVG`) | Done |
| `predicatePushdown` — qualified column refs | Done |
| `constantFolding` — arithmetic, comparison, AND/OR, NULL | Done |
| `columnPruning` — `StarExpr`-aware | Done |
| `indexSelection` + `extractIndexPredicate` | Done |
| Tests (21 cases) | Done |

### Not yet integrated

- Wiring `Planner` + `Optimizer` output into the executor's SELECT path. Currently `executor.buildSelectPipeline` constructs physical operators directly from the AST. The next step is to route SELECT through `Planner.Plan` → `Optimizer.Optimize` → a physical plan builder that converts `LogicalNode` trees into `Operator` trees.
