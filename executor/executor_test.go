package executor

import (
	"os"
	"testing"

	"github.com/robertkoller/MySQLight/catalog"
	"github.com/robertkoller/MySQLight/parser"
	"github.com/robertkoller/MySQLight/storage"
)

// newTestExecutor creates a fresh in-memory-backed executor for one test.
// The returned cleanup function flushes pages and removes the temp file.
func newTestExecutor(t *testing.T) (*Executor, func()) {
	t.Helper()

	file, err := os.CreateTemp("", "executor_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()

	pager, err := storage.Open(file.Name())
	if err != nil {
		os.Remove(file.Name())
		t.Fatal(err)
	}

	pool := storage.NewBufferPool(pager, 64)

	tree, err := storage.NewBTree(pool, 0)
	if err != nil {
		pager.Close()
		os.Remove(file.Name())
		t.Fatal(err)
	}

	cat, err := catalog.Open(tree)
	if err != nil {
		pager.Close()
		os.Remove(file.Name())
		t.Fatal(err)
	}

	cleanup := func() {
		pool.FlushAll()
		pager.Close()
		os.Remove(file.Name())
	}

	return NewExecutor(cat, pool, nil), cleanup
}

// mustExec parses and executes sql, failing the test on any error.
func mustExec(t *testing.T, executor *Executor, sql string) []Row {
	t.Helper()
	p, err := parser.NewParser(sql)
	if err != nil {
		t.Fatalf("parse error for %q: %v", sql, err)
	}
	stmt, err := p.Parse()
	if err != nil {
		t.Fatalf("parse error for %q: %v", sql, err)
	}
	rows, err := executor.Execute(stmt)
	if err != nil {
		t.Fatalf("execute error for %q: %v", sql, err)
	}
	return rows
}

// execErr parses and executes sql and returns the execution error (parse errors still fatal).
func execErr(t *testing.T, executor *Executor, sql string) error {
	t.Helper()
	p, err := parser.NewParser(sql)
	if err != nil {
		t.Fatalf("parse error for %q: %v", sql, err)
	}
	stmt, err := p.Parse()
	if err != nil {
		t.Fatalf("parse error for %q: %v", sql, err)
	}
	_, err = executor.Execute(stmt)
	return err
}

// seedUsers creates the users table and inserts three rows: Alice(30), Bob(25), Charlie(35).
func seedUsers(t *testing.T, executor *Executor) {
	t.Helper()
	mustExec(t, executor, `CREATE TABLE users (id INTEGER PRIMARY KEY NOT NULL, name TEXT NOT NULL, age INTEGER)`)
	mustExec(t, executor, `INSERT INTO users VALUES (1, 'Alice', 30)`)
	mustExec(t, executor, `INSERT INTO users VALUES (2, 'Bob', 25)`)
	mustExec(t, executor, `INSERT INTO users VALUES (3, 'Charlie', 35)`)
}

func TestCreateAndInsert(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	seedUsers(t, executor)

	rows := mustExec(t, executor, `SELECT * FROM users`)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// Rows return in primary key order.
	if rows[0][0].IntVal != 1 || rows[0][1].TextVal != "Alice" || rows[0][2].IntVal != 30 {
		t.Errorf("row 0: got %v", rows[0])
	}
	if rows[1][0].IntVal != 2 || rows[1][1].TextVal != "Bob" || rows[1][2].IntVal != 25 {
		t.Errorf("row 1: got %v", rows[1])
	}
	if rows[2][0].IntVal != 3 || rows[2][1].TextVal != "Charlie" || rows[2][2].IntVal != 35 {
		t.Errorf("row 2: got %v", rows[2])
	}
}

func TestInsertNamedColumns(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	mustExec(t, executor, `CREATE TABLE users (id INTEGER PRIMARY KEY NOT NULL, name TEXT NOT NULL, age INTEGER)`)
	mustExec(t, executor, `INSERT INTO users (id, name, age) VALUES (1, 'Alice', 30)`)
	mustExec(t, executor, `INSERT INTO users (id, age, name) VALUES (2, 25, 'Bob')`)

	rows := mustExec(t, executor, `SELECT * FROM users`)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0][1].TextVal != "Alice" {
		t.Errorf("row 0 name: got %q", rows[0][1].TextVal)
	}
	if rows[1][1].TextVal != "Bob" {
		t.Errorf("row 1 name: got %q", rows[1][1].TextVal)
	}
}

func TestInsertNotNullViolation(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	mustExec(t, executor, `CREATE TABLE users (id INTEGER PRIMARY KEY NOT NULL, name TEXT NOT NULL, age INTEGER)`)

	err := execErr(t, executor, `INSERT INTO users (id, age) VALUES (1, 30)`)
	if err == nil {
		t.Fatal("expected NOT NULL violation error, got nil")
	}
}

func TestSelectWhere(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	seedUsers(t, executor)

	rows := mustExec(t, executor, `SELECT * FROM users WHERE age > 25`)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows with age > 25, got %d", len(rows))
	}
	// B+ tree order (by PK): Alice(30), Charlie(35).
	if rows[0][1].TextVal != "Alice" {
		t.Errorf("expected Alice first, got %q", rows[0][1].TextVal)
	}
	if rows[1][1].TextVal != "Charlie" {
		t.Errorf("expected Charlie second, got %q", rows[1][1].TextVal)
	}
}

func TestSelectWhereAnd(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	seedUsers(t, executor)

	rows := mustExec(t, executor, `SELECT * FROM users WHERE age >= 30 AND age < 35`)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0][1].TextVal != "Alice" {
		t.Errorf("expected Alice, got %q", rows[0][1].TextVal)
	}
}

func TestSelectWhereLike(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	seedUsers(t, executor)

	rows := mustExec(t, executor, `SELECT * FROM users WHERE name LIKE 'A%'`)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0][1].TextVal != "Alice" {
		t.Errorf("expected Alice, got %q", rows[0][1].TextVal)
	}
}

func TestSelectOrderByAsc(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	seedUsers(t, executor)

	rows := mustExec(t, executor, `SELECT * FROM users ORDER BY age`)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// Expected: Bob(25), Alice(30), Charlie(35).
	expected := []int64{25, 30, 35}
	for index, row := range rows {
		if row[2].IntVal != expected[index] {
			t.Errorf("row %d: expected age %d, got %d", index, expected[index], row[2].IntVal)
		}
	}
}

func TestSelectOrderByDesc(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	seedUsers(t, executor)

	rows := mustExec(t, executor, `SELECT * FROM users ORDER BY age DESC`)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// Expected: Charlie(35), Alice(30), Bob(25).
	expected := []int64{35, 30, 25}
	for index, row := range rows {
		if row[2].IntVal != expected[index] {
			t.Errorf("row %d: expected age %d, got %d", index, expected[index], row[2].IntVal)
		}
	}
}

func TestSelectLimit(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	seedUsers(t, executor)

	rows := mustExec(t, executor, `SELECT * FROM users LIMIT 2`)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
}

func TestSelectProject(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	seedUsers(t, executor)

	rows := mustExec(t, executor, `SELECT name FROM users`)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(rows))
	}
	// Each projected row should have exactly 1 column.
	for index, row := range rows {
		if len(row) != 1 {
			t.Errorf("row %d: expected 1 column, got %d", index, len(row))
		}
	}
	if rows[0][0].TextVal != "Alice" {
		t.Errorf("row 0 name: got %q", rows[0][0].TextVal)
	}
}

func TestUpdate(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	seedUsers(t, executor)

	mustExec(t, executor, `UPDATE users SET age = 31 WHERE id = 1`)

	rows := mustExec(t, executor, `SELECT * FROM users WHERE id = 1`)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	if rows[0][2].IntVal != 31 {
		t.Errorf("expected age 31 after UPDATE, got %d", rows[0][2].IntVal)
	}
}

func TestUpdateNoWhere(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	seedUsers(t, executor)

	mustExec(t, executor, `UPDATE users SET age = 99`)

	rows := mustExec(t, executor, `SELECT * FROM users`)
	for index, row := range rows {
		if row[2].IntVal != 99 {
			t.Errorf("row %d: expected age 99, got %d", index, row[2].IntVal)
		}
	}
}

func TestDelete(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	seedUsers(t, executor)

	mustExec(t, executor, `DELETE FROM users WHERE id = 2`)

	rows := mustExec(t, executor, `SELECT * FROM users`)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows after DELETE, got %d", len(rows))
	}
	for _, row := range rows {
		if row[0].IntVal == 2 {
			t.Error("deleted row with id=2 still present")
		}
	}
}

func TestDeleteNoWhere(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	seedUsers(t, executor)

	mustExec(t, executor, `DELETE FROM users`)

	rows := mustExec(t, executor, `SELECT * FROM users`)
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows after DELETE all, got %d", len(rows))
	}
}

func TestJoin(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	// Use unambiguous column names so evalExpr can resolve without table qualifier.
	mustExec(t, executor, `CREATE TABLE departments (dept_id INTEGER PRIMARY KEY NOT NULL, dept_name TEXT NOT NULL)`)
	mustExec(t, executor, `CREATE TABLE employees (emp_id INTEGER PRIMARY KEY NOT NULL, dept_ref INTEGER, emp_name TEXT NOT NULL)`)

	mustExec(t, executor, `INSERT INTO departments VALUES (1, 'Engineering')`)
	mustExec(t, executor, `INSERT INTO departments VALUES (2, 'Marketing')`)
	mustExec(t, executor, `INSERT INTO employees VALUES (1, 1, 'Alice')`)
	mustExec(t, executor, `INSERT INTO employees VALUES (2, 1, 'Bob')`)
	mustExec(t, executor, `INSERT INTO employees VALUES (3, 2, 'Charlie')`)

	rows := mustExec(t, executor, `SELECT * FROM employees JOIN departments ON dept_ref = dept_id`)
	if len(rows) != 3 {
		t.Fatalf("expected 3 joined rows, got %d", len(rows))
	}
	// Each row should have 5 columns: emp_id, dept_ref, emp_name, dept_id, dept_name.
	for index, row := range rows {
		if len(row) != 5 {
			t.Errorf("row %d: expected 5 columns, got %d", index, len(row))
		}
	}
	// dept_ref should equal dept_id in every result row.
	for index, row := range rows {
		if row[1].IntVal != row[3].IntVal {
			t.Errorf("row %d: dept_ref=%d != dept_id=%d", index, row[1].IntVal, row[3].IntVal)
		}
	}
}

func TestAggregateNoGroupBy(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	seedUsers(t, executor)

	rows := mustExec(t, executor, `SELECT COUNT(*), MIN(age), MAX(age) FROM users`)
	if len(rows) != 1 {
		t.Fatalf("expected 1 aggregate row, got %d", len(rows))
	}
	row := rows[0]
	if row[0].IntVal != 3 {
		t.Errorf("COUNT(*): expected 3, got %d", row[0].IntVal)
	}
	if row[1].IntVal != 25 {
		t.Errorf("MIN(age): expected 25, got %d", row[1].IntVal)
	}
	if row[2].IntVal != 35 {
		t.Errorf("MAX(age): expected 35, got %d", row[2].IntVal)
	}
}

func TestAggregateAvg(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	seedUsers(t, executor)

	rows := mustExec(t, executor, `SELECT AVG(age) FROM users`)
	if len(rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(rows))
	}
	// AVG(25, 30, 35) = 30.0
	if rows[0][0].FloatVal != 30.0 {
		t.Errorf("AVG(age): expected 30.0, got %f", rows[0][0].FloatVal)
	}
}

func TestAggregateGroupBy(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	mustExec(t, executor, `CREATE TABLE scores (id INTEGER PRIMARY KEY NOT NULL, category TEXT NOT NULL, score INTEGER NOT NULL)`)
	mustExec(t, executor, `INSERT INTO scores VALUES (1, 'A', 10)`)
	mustExec(t, executor, `INSERT INTO scores VALUES (2, 'A', 20)`)
	mustExec(t, executor, `INSERT INTO scores VALUES (3, 'B', 5)`)

	rows := mustExec(t, executor, `SELECT category, COUNT(*) FROM scores GROUP BY category`)
	if len(rows) != 2 {
		t.Fatalf("expected 2 groups, got %d", len(rows))
	}
	// Groups materialise in insertion order: A first, then B.
	if rows[0][0].TextVal != "A" || rows[0][1].IntVal != 2 {
		t.Errorf("group A: expected (A, 2), got (%q, %d)", rows[0][0].TextVal, rows[0][1].IntVal)
	}
	if rows[1][0].TextVal != "B" || rows[1][1].IntVal != 1 {
		t.Errorf("group B: expected (B, 1), got (%q, %d)", rows[1][0].TextVal, rows[1][1].IntVal)
	}
}

func TestCreateDropTable(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	mustExec(t, executor, `CREATE TABLE tmp (id INTEGER PRIMARY KEY NOT NULL)`)
	mustExec(t, executor, `INSERT INTO tmp VALUES (1)`)

	mustExec(t, executor, `DROP TABLE tmp`)

	err := execErr(t, executor, `SELECT * FROM tmp`)
	if err == nil {
		t.Fatal("expected error selecting from dropped table, got nil")
	}
}

func TestCreateDropIndex(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	mustExec(t, executor, `CREATE TABLE users (id INTEGER PRIMARY KEY NOT NULL, name TEXT NOT NULL, age INTEGER)`)
	mustExec(t, executor, `CREATE INDEX idx_age ON users (age)`)

	// Confirm the index exists in the catalog.
	p, _ := parser.NewParser(`DROP INDEX idx_age`)
	stmt, _ := p.Parse()
	if _, err := executor.Execute(stmt); err != nil {
		t.Fatalf("DROP INDEX: %v", err)
	}
}

func TestSelectWhereAndOrderByLimit(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	seedUsers(t, executor)
	mustExec(t, executor, `INSERT INTO users VALUES (4, 'Dave', 28)`)
	mustExec(t, executor, `INSERT INTO users VALUES (5, 'Eve', 32)`)

	// age > 25 → Alice(30), Charlie(35), Dave(28), Eve(32); ORDER BY age → Dave(28), Alice(30), Eve(32), Charlie(35); LIMIT 2 → Dave, Alice.
	rows := mustExec(t, executor, `SELECT * FROM users WHERE age > 25 ORDER BY age LIMIT 2`)
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(rows))
	}
	if rows[0][1].TextVal != "Dave" {
		t.Errorf("row 0: expected Dave, got %q", rows[0][1].TextVal)
	}
	if rows[1][1].TextVal != "Alice" {
		t.Errorf("row 1: expected Alice, got %q", rows[1][1].TextVal)
	}
}

func TestForeignKeyInsertRejectsOrphan(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	mustExec(t, executor, `CREATE TABLE departments (dept_id INTEGER PRIMARY KEY NOT NULL, dept_name TEXT NOT NULL)`)
	mustExec(t, executor, `CREATE TABLE employees (emp_id INTEGER PRIMARY KEY NOT NULL, dept_ref INTEGER, emp_name TEXT NOT NULL)`)

	// Insert a row that references a non-existent department.
	// The catalog has the FK declared on employees but the parser stores it in ForeignKeys.
	// We wire the FK manually through catalog.CreateTable rather than SQL DDL since
	// the parser's CreateTableStmt.ForeignKeys feeds through astToTableDef.
	// This test uses the Go API directly to register the FK and verify enforcement.
	empDef, _ := executor.catalog.GetTable("employees")
	empDef.ForeignKeys = []catalog.ForeignKeyDef{
		{ColumnName: "dept_ref", RefTable: "departments", RefColumn: "dept_id", OnDelete: catalog.FKRestrict},
	}

	mustExec(t, executor, `INSERT INTO departments VALUES (1, 'Engineering')`)

	// dept_ref = 99 does not exist in departments.
	err := execErr(t, executor, `INSERT INTO employees VALUES (1, 99, 'Alice')`)
	if err == nil {
		t.Fatal("expected FK violation inserting orphan row, got nil")
	}

	// dept_ref = 1 does exist — should succeed.
	mustExec(t, executor, `INSERT INTO employees VALUES (1, 1, 'Alice')`)
}

func TestForeignKeyRestrict(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	mustExec(t, executor, `CREATE TABLE departments (dept_id INTEGER PRIMARY KEY NOT NULL, dept_name TEXT NOT NULL)`)
	mustExec(t, executor, `CREATE TABLE employees (emp_id INTEGER PRIMARY KEY NOT NULL, dept_ref INTEGER, emp_name TEXT NOT NULL)`)

	mustExec(t, executor, `INSERT INTO departments VALUES (1, 'Engineering')`)
	mustExec(t, executor, `INSERT INTO employees VALUES (1, 1, 'Alice')`)

	// Register FK with RESTRICT on the employees table definition in memory.
	empDef, _ := executor.catalog.GetTable("employees")
	empDef.ForeignKeys = []catalog.ForeignKeyDef{
		{ColumnName: "dept_ref", RefTable: "departments", RefColumn: "dept_id", OnDelete: catalog.FKRestrict},
	}

	// Deleting the referenced department must be blocked.
	err := execErr(t, executor, `DELETE FROM departments WHERE dept_id = 1`)
	if err == nil {
		t.Fatal("expected FK RESTRICT error, got nil")
	}

	// The department row must still be present.
	rows := mustExec(t, executor, `SELECT * FROM departments`)
	if len(rows) != 1 {
		t.Errorf("expected department to still exist, got %d rows", len(rows))
	}
}

func TestForeignKeyCascadeDelete(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	mustExec(t, executor, `CREATE TABLE departments (dept_id INTEGER PRIMARY KEY NOT NULL, dept_name TEXT NOT NULL)`)
	mustExec(t, executor, `CREATE TABLE employees (emp_id INTEGER PRIMARY KEY NOT NULL, dept_ref INTEGER, emp_name TEXT NOT NULL)`)

	mustExec(t, executor, `INSERT INTO departments VALUES (1, 'Engineering')`)
	mustExec(t, executor, `INSERT INTO employees VALUES (1, 1, 'Alice')`)
	mustExec(t, executor, `INSERT INTO employees VALUES (2, 1, 'Bob')`)

	// Register FK with CASCADE.
	empDef, _ := executor.catalog.GetTable("employees")
	empDef.ForeignKeys = []catalog.ForeignKeyDef{
		{ColumnName: "dept_ref", RefTable: "departments", RefColumn: "dept_id", OnDelete: catalog.FKCascade},
	}

	mustExec(t, executor, `DELETE FROM departments WHERE dept_id = 1`)

	// Both employees should have been cascaded away.
	rows := mustExec(t, executor, `SELECT * FROM employees`)
	if len(rows) != 0 {
		t.Errorf("expected 0 employees after CASCADE delete, got %d", len(rows))
	}
}

func TestForeignKeyCascadeSetNull(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	mustExec(t, executor, `CREATE TABLE departments (dept_id INTEGER PRIMARY KEY NOT NULL, dept_name TEXT NOT NULL)`)
	mustExec(t, executor, `CREATE TABLE employees (emp_id INTEGER PRIMARY KEY NOT NULL, dept_ref INTEGER, emp_name TEXT NOT NULL)`)

	mustExec(t, executor, `INSERT INTO departments VALUES (1, 'Engineering')`)
	mustExec(t, executor, `INSERT INTO employees VALUES (1, 1, 'Alice')`)

	// Register FK with SET NULL.
	empDef, _ := executor.catalog.GetTable("employees")
	empDef.ForeignKeys = []catalog.ForeignKeyDef{
		{ColumnName: "dept_ref", RefTable: "departments", RefColumn: "dept_id", OnDelete: catalog.FKSetNull},
	}

	mustExec(t, executor, `DELETE FROM departments WHERE dept_id = 1`)

	// Alice should still exist but with dept_ref = NULL.
	rows := mustExec(t, executor, `SELECT * FROM employees`)
	if len(rows) != 1 {
		t.Fatalf("expected 1 employee row, got %d", len(rows))
	}
	if !rows[0][1].IsNull {
		t.Errorf("expected dept_ref to be NULL after SET NULL, got %v", rows[0][1])
	}
}

func TestForeignKeyUpdateCascade(t *testing.T) {
	executor, cleanup := newTestExecutor(t)
	defer cleanup()

	mustExec(t, executor, `CREATE TABLE departments (dept_id INTEGER PRIMARY KEY NOT NULL, dept_name TEXT NOT NULL)`)
	mustExec(t, executor, `CREATE TABLE employees (emp_id INTEGER PRIMARY KEY NOT NULL, dept_ref INTEGER, emp_name TEXT NOT NULL)`)

	mustExec(t, executor, `INSERT INTO departments VALUES (1, 'Engineering')`)
	mustExec(t, executor, `INSERT INTO employees VALUES (1, 1, 'Alice')`)

	// Register FK with CASCADE on UPDATE.
	empDef, _ := executor.catalog.GetTable("employees")
	empDef.ForeignKeys = []catalog.ForeignKeyDef{
		{ColumnName: "dept_ref", RefTable: "departments", RefColumn: "dept_id", OnUpdate: catalog.FKCascade},
	}

	// Update the parent PK from 1 to 99.
	mustExec(t, executor, `UPDATE departments SET dept_id = 99 WHERE dept_id = 1`)

	// Alice's dept_ref should have cascaded to 99.
	rows := mustExec(t, executor, `SELECT * FROM employees`)
	if len(rows) != 1 {
		t.Fatalf("expected 1 employee row, got %d", len(rows))
	}
	if rows[0][1].IntVal != 99 {
		t.Errorf("expected dept_ref = 99 after CASCADE update, got %d", rows[0][1].IntVal)
	}
}
