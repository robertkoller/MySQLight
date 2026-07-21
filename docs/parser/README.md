# MySQLight Parser

Phase 2 of MySQLight. Takes a raw SQL string and produces a typed AST that the
executor (Phase 4) can walk. Two files: the lexer tokenizes the input, the parser
consumes those tokens and builds the tree.

---

## Layering

```
        ┌──────────────────────────┐
        │        Executor          │   (Phase 4) — walks the AST
        └─────────────┬────────────┘
                      │ Statement / Expr nodes
        ┌─────────────▼────────────┐
        │          Parser          │   parser.go — recursive descent
        └─────────────┬────────────┘
                      │ Token stream
        ┌─────────────▼────────────┐
        │           Lexer          │   lexer.go — []rune scanner
        └──────────────────────────┘
                      │
                 raw SQL string
```

---

## Lexer (`lexer.go`)

```go
type Lexer struct {
    input    []rune
    position int
    line     int
}

func NewLexer(input string) *Lexer
func (l *Lexer) NextToken() Token
```

The input is converted to `[]rune` immediately so multi-byte Unicode characters
are safe to scan one codepoint at a time. `NextToken` skips whitespace (counting
newlines for error reporting), then dispatches on the first character:

| First character | Handler | Produces |
|-----------------|---------|----------|
| digit `0–9` | `readNumber` | `TokenInteger` or `TokenFloat` |
| letter or `_` | `readIdent` | keyword token or `TokenIdent` |
| `'` | `readString` | `TokenString` |
| anything else | `readOperator` | punctuation/operator token or `TokenErr` |

### Token types

**Literals:** `TokenEOF`, `TokenIdent`, `TokenInteger`, `TokenFloat`, `TokenString`

**Keywords** (42 total, case-insensitive):
`SELECT FROM WHERE INSERT INTO VALUES UPDATE SET DELETE CREATE TABLE DROP INDEX ON PRIMARY KEY NOT NULL DEFAULT BEGIN COMMIT ROLLBACK AND OR LIKE ORDER BY LIMIT JOIN INNER LEFT GROUP HAVING COUNT SUM MIN MAX AVG REFERENCES FOREIGN UNIQUE ANALYZE DESC`

**Punctuation / operators:**
`( ) , ; . * = < > <= >= != + - / %`

**Error:** `TokenErr` — returned for unknown characters or unterminated strings.

### `readNumber`
Scans digits. A single `.` switches to float mode; a second `.` returns
`TokenErr`. Any non-digit character terminates the number without consuming it,
leaving it for the next `NextToken` call.

### `readIdent`
Scans letters, digits, and underscores. Lowercases the result and looks it up in
the `keywords` map; returns a keyword token if found, else `TokenIdent` with the
original-case literal.

### `readString`
Consumes the opening `'`, then scans until a closing `'` or end of input. An
unterminated string returns `TokenErr`.

### `readOperator`
Handles single-character punctuation directly. For `!`, `<`, and `>`, peeks one
character ahead to detect `!=`, `<=`, `>=`.

---

## Parser (`parser.go` + `ast.go`)

```go
type Parser struct {
    lexer   *Lexer
    current Token   // most recently consumed token
    peek    Token   // one token of lookahead
}

func NewParser(input string) (*Parser, error)
func (p *Parser) Parse() (Statement, error)
```

`NewParser` primes both `current` and `peek` by calling `NextToken` twice, so the
parser always has one token of lookahead. `Parse` dispatches on `current.Type` to
the appropriate sub-parser.

Two helpers keep the code clean:
- `advance()` — shifts `peek` into `current`, reads a new `peek`.
- `expect(t TokenType)` — calls `advance` and returns an error if the token type
  doesn't match.

### Statements

| SQL | Parser method | AST node |
|-----|---------------|----------|
| `SELECT` | `parseSelect` | `*SelectStmt` |
| `INSERT INTO` | `parseInsert` | `*InsertStmt` |
| `UPDATE` | `parseUpdate` | `*UpdateStmt` |
| `DELETE FROM` | `parseDelete` | `*DeleteStmt` |
| `CREATE TABLE` | `parseCreateTable` | `*CreateTableStmt` |
| `CREATE [UNIQUE] INDEX` | `parseCreateIndex` | `*CreateIndexStmt` |
| `DROP TABLE` | `parseDrop` | `*DropTableStmt` |
| `DROP INDEX` | `parseDrop` | `*DropIndexStmt` |
| `BEGIN` | inline | `*BeginStmt` |
| `COMMIT` | inline | `*CommitStmt` |
| `ROLLBACK` | inline | `*RollbackStmt` |
| `ANALYZE` | `parseAnalyze` | `*AnalyzeStmt` |

### AST nodes

```go
type SelectStmt struct {
    Columns []Expr
    From    string
    Joins   []JoinClause
    Where   Expr
    GroupBy []Expr
    Having  Expr
    OrderBy []OrderClause
    Limit   *int
}

type InsertStmt struct {
    Table   string
    Columns []string      // empty = positional
    Values  [][]Expr      // outer = rows, inner = values per row
}

type UpdateStmt struct {
    Table string
    Set   []Assignment
    Where Expr
}

type DeleteStmt  { Table string; Where Expr }
type CreateTableStmt { Table string; Columns []ColumnDefAST; ForeignKeys []ForeignKeyAST }
type CreateIndexStmt { Index, Table, Column string; Unique bool }
type DropTableStmt   { Table string }
type DropIndexStmt   { Index string }
type AnalyzeStmt     { Table string }  // empty Table = analyze all
type BeginStmt, CommitStmt, RollbackStmt struct{}
```

`ColumnDefAST` holds the raw type name as a string (`"INTEGER"`, `"FLOAT"`,
`"TEXT"`, `"BLOB"`) — conversion to `catalog.DataType` happens in the executor.
`ForeignKeyAST` holds `OnDelete`/`OnUpdate` as strings (`"RESTRICT"`, `"CASCADE"`,
`"SET NULL"`).

### Expression nodes

```go
type BinaryExpr struct { Left Expr; Op string; Right Expr }
// Op: "=", "!=", "<", ">", "<=", ">=", "+", "-", "*", "/", "%", "AND", "OR", "LIKE"

type UnaryExpr  struct { Op string; Operand Expr }
// Op: "-", "NOT"

type ColumnRef  struct { Table, Column string }  // Table empty if unqualified
type Literal    struct { Kind, Value string }     // Kind: "integer"|"float"|"string"|"null"
type FuncCall   struct { Name string; Args []Expr; Star bool }
// Name: "COUNT"|"SUM"|"MIN"|"MAX"|"AVG"; Star=true for COUNT(*)
type StarExpr   struct{}  // bare * in SELECT *
```

### Expression precedence (lowest → highest)

```
OR
AND
NOT  (unary prefix)
=  !=  <  >  <=  >=  LIKE
+  -
*  /  %
unary -
primary: literal, column ref, func call, (grouped expr)
```

Implemented via recursive descent (`parseOr` → `parseAnd` → `parseNot` →
`parseComparison` → `parseAddSub` → `parseMulDiv` → `parseUnary` →
`parsePrimary`). Each level loops on its operator tokens so left-associativity is
natural.

**Special case:** `COUNT`, `SUM`, `MIN`, `MAX`, `AVG` lex as keyword tokens (not
`TokenIdent`), so `parsePrimary` has an explicit case for them before the
`TokenIdent` case.

---

## Phase 2 Status — complete

| Component | Status |
|-----------|--------|
| Lexer: all token types, Unicode-safe `[]rune` input | Done |
| Keywords map (42 entries, case-insensitive) | Done |
| `readNumber` (int/float, double-dot error) | Done |
| `readIdent` (keyword lookup) | Done |
| `readString` (single-quoted, unterminated → `TokenErr`) | Done |
| `readOperator` (two-char operators with one-char lookahead) | Done |
| Parser: `advance` / `expect` helpers, two-token lookahead | Done |
| All 12 statement parsers | Done |
| Expression precedence climbing (7 levels) | Done |
| Aggregate function tokens in `parsePrimary` | Done |
| Foreign key `ON DELETE / ON UPDATE` actions | Done |
| Tests (25 cases — all statement types, expressions, errors) | Done |

### Not in Phase 2 (later work)
- Semantic validation (type checking, column existence) — executor's job.
- `IN (...)` and `EXISTS (subquery)` expressions — not yet supported.
- Multi-row `INSERT` (`VALUES (...), (...)`) — parser produces `[][]Expr` ready
  for it; executor needs to handle multiple rows.
