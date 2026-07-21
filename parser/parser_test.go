package parser

import (
	"testing"
)

func mustParse(t *testing.T, input string) Statement {
	t.Helper()
	parser, err := NewParser(input)
	if err != nil {
		t.Fatalf("NewParser(%q) error: %v", input, err)
	}
	statement, err := parser.Parse()
	if err != nil {
		t.Fatalf("Parse(%q) error: %v", input, err)
	}
	return statement
}

func TestParseSelectStar(t *testing.T) {
	statement := mustParse(t, "SELECT * FROM users")
	selectStatement, ok := statement.(*SelectStmt)
	if !ok {
		t.Fatalf("expected *SelectStmt, got %T", statement)
	}
	if selectStatement.From != "users" {
		t.Errorf("From = %q, want %q", selectStatement.From, "users")
	}
	if len(selectStatement.Columns) != 1 {
		t.Fatalf("expected 1 column, got %d", len(selectStatement.Columns))
	}
	if _, ok := selectStatement.Columns[0].(*StarExpr); !ok {
		t.Errorf("expected StarExpr column, got %T", selectStatement.Columns[0])
	}
}

func TestParseSelectColumns(t *testing.T) {
	statement := mustParse(t, "SELECT id, name FROM users")
	selectStatement := statement.(*SelectStmt)
	if selectStatement.From != "users" {
		t.Errorf("From = %q, want %q", selectStatement.From, "users")
	}
	if len(selectStatement.Columns) != 2 {
		t.Fatalf("expected 2 columns, got %d", len(selectStatement.Columns))
	}
	first, ok := selectStatement.Columns[0].(*ColumnRef)
	if !ok {
		t.Fatalf("col[0]: expected *ColumnRef, got %T", selectStatement.Columns[0])
	}
	if first.Column != "id" {
		t.Errorf("col[0] = %q, want %q", first.Column, "id")
	}
	second := selectStatement.Columns[1].(*ColumnRef)
	if second.Column != "name" {
		t.Errorf("col[1] = %q, want %q", second.Column, "name")
	}
}

func TestParseSelectWhere(t *testing.T) {
	statement := mustParse(t, "SELECT id FROM users WHERE age > 25")
	selectStatement := statement.(*SelectStmt)
	if selectStatement.Where == nil {
		t.Fatal("expected WHERE clause, got nil")
	}
	binaryExpression, ok := selectStatement.Where.(*BinaryExpr)
	if !ok {
		t.Fatalf("WHERE: expected *BinaryExpr, got %T", selectStatement.Where)
	}
	if binaryExpression.Op != ">" {
		t.Errorf("WHERE op = %q, want %q", binaryExpression.Op, ">")
	}
	left, ok := binaryExpression.Left.(*ColumnRef)
	if !ok || left.Column != "age" {
		t.Errorf("WHERE left: expected ColumnRef{age}, got %T", binaryExpression.Left)
	}
	right, ok := binaryExpression.Right.(*Literal)
	if !ok || right.Value != "25" {
		t.Errorf("WHERE right: expected Literal{25}, got %T", binaryExpression.Right)
	}
}

func TestParseSelectOrderByLimit(t *testing.T) {
	statement := mustParse(t, "SELECT id, name FROM users ORDER BY name DESC LIMIT 10")
	selectStatement := statement.(*SelectStmt)

	if len(selectStatement.OrderBy) != 1 {
		t.Fatalf("expected 1 ORDER BY clause, got %d", len(selectStatement.OrderBy))
	}
	orderClause := selectStatement.OrderBy[0]
	if !orderClause.Desc {
		t.Error("expected DESC = true")
	}
	ref, ok := orderClause.Expr.(*ColumnRef)
	if !ok || ref.Column != "name" {
		t.Errorf("ORDER BY expr: expected ColumnRef{name}, got %T", orderClause.Expr)
	}

	if selectStatement.Limit == nil {
		t.Fatal("expected LIMIT, got nil")
	}
	if *selectStatement.Limit != 10 {
		t.Errorf("LIMIT = %d, want 10", *selectStatement.Limit)
	}
}

func TestParseSelectGroupByHaving(t *testing.T) {
	statement := mustParse(t, "SELECT country FROM users GROUP BY country HAVING count(country) > 5")
	selectStatement := statement.(*SelectStmt)

	if len(selectStatement.GroupBy) != 1 {
		t.Fatalf("expected 1 GROUP BY expr, got %d", len(selectStatement.GroupBy))
	}
	ref, ok := selectStatement.GroupBy[0].(*ColumnRef)
	if !ok || ref.Column != "country" {
		t.Errorf("GROUP BY: expected ColumnRef{country}, got %T", selectStatement.GroupBy[0])
	}
	if selectStatement.Having == nil {
		t.Fatal("expected HAVING clause, got nil")
	}
}

func TestParseSelectJoin(t *testing.T) {
	statement := mustParse(t, "SELECT * FROM orders JOIN users ON orders.user_id = users.id")
	selectStatement := statement.(*SelectStmt)

	if len(selectStatement.Joins) != 1 {
		t.Fatalf("expected 1 JOIN, got %d", len(selectStatement.Joins))
	}
	join := selectStatement.Joins[0]
	if join.Table != "users" {
		t.Errorf("JOIN table = %q, want %q", join.Table, "users")
	}
	if join.On == nil {
		t.Fatal("JOIN ON expr is nil")
	}
}

func TestParseInsert(t *testing.T) {
	statement := mustParse(t, "INSERT INTO users (id, name) VALUES (1, 'Alice'), (2, 'Bob')")
	insertStatement, ok := statement.(*InsertStmt)
	if !ok {
		t.Fatalf("expected *InsertStmt, got %T", statement)
	}
	if insertStatement.Table != "users" {
		t.Errorf("Table = %q, want %q", insertStatement.Table, "users")
	}
	if len(insertStatement.Columns) != 2 || insertStatement.Columns[0] != "id" || insertStatement.Columns[1] != "name" {
		t.Errorf("Columns = %v, want [id name]", insertStatement.Columns)
	}
	if len(insertStatement.Values) != 2 {
		t.Fatalf("expected 2 value rows, got %d", len(insertStatement.Values))
	}
	firstRow := insertStatement.Values[0]
	if len(firstRow) != 2 {
		t.Fatalf("row[0]: expected 2 values, got %d", len(firstRow))
	}
	idLiteral, ok := firstRow[0].(*Literal)
	if !ok || idLiteral.Value != "1" {
		t.Errorf("row[0][0]: expected Literal{1}, got %T", firstRow[0])
	}
	nameLiteral, ok := firstRow[1].(*Literal)
	if !ok || nameLiteral.Value != "Alice" {
		t.Errorf("row[0][1]: expected Literal{Alice}, got %T", firstRow[1])
	}
}

func TestParseInsertNoColumns(t *testing.T) {
	statement := mustParse(t, "INSERT INTO users VALUES (1, 'Alice')")
	insertStatement := statement.(*InsertStmt)
	if insertStatement.Table != "users" {
		t.Errorf("Table = %q, want %q", insertStatement.Table, "users")
	}
	if len(insertStatement.Columns) != 0 {
		t.Errorf("expected no columns, got %v", insertStatement.Columns)
	}
	if len(insertStatement.Values) != 1 {
		t.Fatalf("expected 1 row, got %d", len(insertStatement.Values))
	}
}

func TestParseUpdate(t *testing.T) {
	statement := mustParse(t, "UPDATE users SET name = 'Bob', age = 30 WHERE id = 1")
	updateStatement, ok := statement.(*UpdateStmt)
	if !ok {
		t.Fatalf("expected *UpdateStmt, got %T", statement)
	}
	if updateStatement.Table != "users" {
		t.Errorf("Table = %q, want %q", updateStatement.Table, "users")
	}
	if len(updateStatement.Set) != 2 {
		t.Fatalf("expected 2 assignments, got %d", len(updateStatement.Set))
	}
	if updateStatement.Set[0].Column != "name" {
		t.Errorf("Set[0].Column = %q, want %q", updateStatement.Set[0].Column, "name")
	}
	if updateStatement.Where == nil {
		t.Error("expected WHERE clause, got nil")
	}
}

func TestParseDelete(t *testing.T) {
	statement := mustParse(t, "DELETE FROM users WHERE id = 5")
	deleteStatement, ok := statement.(*DeleteStmt)
	if !ok {
		t.Fatalf("expected *DeleteStmt, got %T", statement)
	}
	if deleteStatement.Table != "users" {
		t.Errorf("Table = %q, want %q", deleteStatement.Table, "users")
	}
	if deleteStatement.Where == nil {
		t.Error("expected WHERE clause, got nil")
	}
}

func TestParseDeleteNoWhere(t *testing.T) {
	statement := mustParse(t, "DELETE FROM users")
	deleteStatement := statement.(*DeleteStmt)
	if deleteStatement.Table != "users" {
		t.Errorf("Table = %q, want %q", deleteStatement.Table, "users")
	}
	if deleteStatement.Where != nil {
		t.Error("expected no WHERE clause")
	}
}

func TestParseCreateTable(t *testing.T) {
	sql := "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, age INTEGER DEFAULT 0, FOREIGN KEY (age) REFERENCES other(id) ON DELETE CASCADE)"
	statement := mustParse(t, sql)
	createStatement, ok := statement.(*CreateTableStmt)
	if !ok {
		t.Fatalf("expected *CreateTableStmt, got %T", statement)
	}
	if createStatement.Table != "users" {
		t.Errorf("Table = %q, want %q", createStatement.Table, "users")
	}
	if len(createStatement.Columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(createStatement.Columns))
	}

	idColumn := createStatement.Columns[0]
	if idColumn.Name != "id" || idColumn.TypeName != "INTEGER" || !idColumn.PrimaryKey {
		t.Errorf("col[0]: got {%s %s PK=%v}, want {id INTEGER PK=true}", idColumn.Name, idColumn.TypeName, idColumn.PrimaryKey)
	}

	nameColumn := createStatement.Columns[1]
	if nameColumn.Name != "name" || nameColumn.TypeName != "TEXT" || !nameColumn.NotNull {
		t.Errorf("col[1]: got {%s %s NN=%v}, want {name TEXT NN=true}", nameColumn.Name, nameColumn.TypeName, nameColumn.NotNull)
	}

	if len(createStatement.ForeignKeys) != 1 {
		t.Fatalf("expected 1 foreign key, got %d", len(createStatement.ForeignKeys))
	}
	foreignKey := createStatement.ForeignKeys[0]
	if foreignKey.OnDelete != "CASCADE" {
		t.Errorf("OnDelete = %q, want %q", foreignKey.OnDelete, "CASCADE")
	}
}

func TestParseCreateIndex(t *testing.T) {
	statement := mustParse(t, "CREATE INDEX idx_name ON users (name)")
	createIndexStatement, ok := statement.(*CreateIndexStmt)
	if !ok {
		t.Fatalf("expected *CreateIndexStmt, got %T", statement)
	}
	if createIndexStatement.Index != "idx_name" {
		t.Errorf("Index = %q, want %q", createIndexStatement.Index, "idx_name")
	}
	if createIndexStatement.Table != "users" {
		t.Errorf("Table = %q, want %q", createIndexStatement.Table, "users")
	}
	if createIndexStatement.Column != "name" {
		t.Errorf("Column = %q, want %q", createIndexStatement.Column, "name")
	}
	if createIndexStatement.Unique {
		t.Error("expected Unique = false")
	}
}

func TestParseCreateUniqueIndex(t *testing.T) {
	statement := mustParse(t, "CREATE UNIQUE INDEX idx_email ON users (email)")
	createIndexStatement := statement.(*CreateIndexStmt)
	if !createIndexStatement.Unique {
		t.Error("expected Unique = true")
	}
	if createIndexStatement.Index != "idx_email" {
		t.Errorf("Index = %q, want %q", createIndexStatement.Index, "idx_email")
	}
}

func TestParseDropTable(t *testing.T) {
	statement := mustParse(t, "DROP TABLE users")
	dropStatement, ok := statement.(*DropTableStmt)
	if !ok {
		t.Fatalf("expected *DropTableStmt, got %T", statement)
	}
	if dropStatement.Table != "users" {
		t.Errorf("Table = %q, want %q", dropStatement.Table, "users")
	}
}

func TestParseDropIndex(t *testing.T) {
	statement := mustParse(t, "DROP INDEX idx_name")
	dropStatement, ok := statement.(*DropIndexStmt)
	if !ok {
		t.Fatalf("expected *DropIndexStmt, got %T", statement)
	}
	if dropStatement.Index != "idx_name" {
		t.Errorf("Index = %q, want %q", dropStatement.Index, "idx_name")
	}
}

func TestParseTransactions(t *testing.T) {
	for _, testCase := range []struct {
		input string
		want  Statement
	}{
		{"BEGIN", &BeginStmt{}},
		{"COMMIT", &CommitStmt{}},
		{"ROLLBACK", &RollbackStmt{}},
	} {
		statement := mustParse(t, testCase.input)
		if _, ok := statement.(*BeginStmt); ok && testCase.input != "BEGIN" {
			t.Errorf("input %q: unexpected BeginStmt", testCase.input)
		}
		_ = statement
	}
}

func TestParseAnalyze(t *testing.T) {
	statement := mustParse(t, "ANALYZE users")
	analyzeStatement, ok := statement.(*AnalyzeStmt)
	if !ok {
		t.Fatalf("expected *AnalyzeStmt, got %T", statement)
	}
	if analyzeStatement.Table != "users" {
		t.Errorf("Table = %q, want %q", analyzeStatement.Table, "users")
	}
}

func TestParseExprPrecedence(t *testing.T) {
	// a + b * c should produce a + (b * c)
	statement := mustParse(t, "SELECT a + b * c FROM t")
	selectStatement := statement.(*SelectStmt)
	outer, ok := selectStatement.Columns[0].(*BinaryExpr)
	if !ok || outer.Op != "+" {
		t.Fatalf("expected outer op +, got %T %v", selectStatement.Columns[0], outer)
	}
	inner, ok := outer.Right.(*BinaryExpr)
	if !ok || inner.Op != "*" {
		t.Fatalf("expected inner op *, got %T", outer.Right)
	}
}

func TestParseExprLogicalPrecedence(t *testing.T) {
	// NOT a AND b OR c should produce ((NOT a) AND b) OR c
	statement := mustParse(t, "SELECT * FROM t WHERE NOT a AND b OR c")
	selectStatement := statement.(*SelectStmt)

	outerOr, ok := selectStatement.Where.(*BinaryExpr)
	if !ok || outerOr.Op != "OR" {
		t.Fatalf("expected outer OR, got %T", selectStatement.Where)
	}
	innerAnd, ok := outerOr.Left.(*BinaryExpr)
	if !ok || innerAnd.Op != "AND" {
		t.Fatalf("expected inner AND, got %T", outerOr.Left)
	}
	notExpr, ok := innerAnd.Left.(*UnaryExpr)
	if !ok || notExpr.Op != "NOT" {
		t.Fatalf("expected NOT unary, got %T", innerAnd.Left)
	}
}

func TestParseExprColumnRef(t *testing.T) {
	statement := mustParse(t, "SELECT table1.column1 FROM t")
	selectStatement := statement.(*SelectStmt)
	ref, ok := selectStatement.Columns[0].(*ColumnRef)
	if !ok {
		t.Fatalf("expected *ColumnRef, got %T", selectStatement.Columns[0])
	}
	if ref.Table != "table1" || ref.Column != "column1" {
		t.Errorf("got {%s.%s}, want {table1.column1}", ref.Table, ref.Column)
	}
}

func TestParseExprFuncCall(t *testing.T) {
	statement := mustParse(t, "SELECT COUNT(*) FROM users")
	selectStatement := statement.(*SelectStmt)
	call, ok := selectStatement.Columns[0].(*FuncCall)
	if !ok {
		t.Fatalf("expected *FuncCall, got %T", selectStatement.Columns[0])
	}
	if call.Name != "COUNT" || !call.Star {
		t.Errorf("got FuncCall{%s Star=%v}, want {COUNT Star=true}", call.Name, call.Star)
	}
}

func TestParseExprGrouped(t *testing.T) {
	// (a + b) * c should produce (a+b) * c with * at the top
	statement := mustParse(t, "SELECT (a + b) * c FROM t")
	selectStatement := statement.(*SelectStmt)
	outer, ok := selectStatement.Columns[0].(*BinaryExpr)
	if !ok || outer.Op != "*" {
		t.Fatalf("expected outer *, got %T", selectStatement.Columns[0])
	}
}

func TestParseLiterals(t *testing.T) {
	statement := mustParse(t, "SELECT 42, 3.14, 'hello', NULL FROM t")
	selectStatement := statement.(*SelectStmt)
	if len(selectStatement.Columns) != 4 {
		t.Fatalf("expected 4 columns, got %d", len(selectStatement.Columns))
	}

	integer, ok := selectStatement.Columns[0].(*Literal)
	if !ok || integer.Kind != "integer" || integer.Value != "42" {
		t.Errorf("col[0]: expected integer 42, got %+v", selectStatement.Columns[0])
	}
	float, ok := selectStatement.Columns[1].(*Literal)
	if !ok || float.Kind != "float" || float.Value != "3.14" {
		t.Errorf("col[1]: expected float 3.14, got %+v", selectStatement.Columns[1])
	}
	str, ok := selectStatement.Columns[2].(*Literal)
	if !ok || str.Kind != "string" || str.Value != "hello" {
		t.Errorf("col[2]: expected string 'hello', got %+v", selectStatement.Columns[2])
	}
	null, ok := selectStatement.Columns[3].(*Literal)
	if !ok || null.Kind != "null" {
		t.Errorf("col[3]: expected null literal, got %+v", selectStatement.Columns[3])
	}
}

func TestParseErrors(t *testing.T) {
	for _, input := range []string{
		"SELECT FROM users",      // missing column list
		"SELECT id users",        // missing FROM
		"INSERT INTO VALUES (1)", // missing table name
		"DROP COLUMN foo",        // invalid token after DROP
	} {
		parser, err := NewParser(input)
		if err != nil {
			continue // NewParser itself errored, which is fine
		}
		_, err = parser.Parse()
		if err == nil {
			t.Errorf("expected error for input %q, got nil", input)
		}
	}
}
