package mysqlight_test

import (
	"os"
	"path/filepath"
	"testing"

	mysqlight "github.com/robertkoller/MySQLight"
)

func openTestDB(t *testing.T) (*mysqlight.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := mysqlight.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { database.Close() })
	return database, path
}

func mustExec(t *testing.T, database *mysqlight.DB, sql string) {
	t.Helper()
	if _, err := database.Exec(sql); err != nil {
		t.Fatalf("Exec(%q): %v", sql, err)
	}
}

func TestOpenAndClose(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")
	database, err := mysqlight.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("database file missing after close: %v", err)
	}
}

func TestReopenPreservesData(t *testing.T) {
	path := filepath.Join(t.TempDir(), "test.db")

	database, err := mysqlight.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	mustExec(t, database, "CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT NOT NULL, age INTEGER)")
	mustExec(t, database, "INSERT INTO users (id, name, age) VALUES (1, 'Alice', 30)")
	mustExec(t, database, "INSERT INTO users (id, name, age) VALUES (2, 'Bob', 25)")
	if err := database.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Reopen and verify the data survived.
	database2, err := mysqlight.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer database2.Close()

	result, err := database2.Exec("SELECT id, name, age FROM users")
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if len(result.Rows) != 2 {
		t.Fatalf("expected 2 rows after reopen, got %d", len(result.Rows))
	}
}

func TestExecDDLAndDML(t *testing.T) {
	database, _ := openTestDB(t)
	mustExec(t, database, "CREATE TABLE items (id INTEGER PRIMARY KEY, label TEXT NOT NULL)")
	mustExec(t, database, "INSERT INTO items (id, label) VALUES (1, 'widget'), (2, 'gadget')")
	mustExec(t, database, "UPDATE items SET label = 'gizmo' WHERE id = 2")
	mustExec(t, database, "DELETE FROM items WHERE id = 1")

	result, err := database.Exec("SELECT id, label FROM items")
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
	if got := mysqlight.FormatValue(result.Rows[0][1], mysqlight.TypeText); got != "gizmo" {
		t.Fatalf("expected label 'gizmo', got %q", got)
	}
}

func TestSelectColumns(t *testing.T) {
	database, _ := openTestDB(t)
	mustExec(t, database, "CREATE TABLE products (id INTEGER PRIMARY KEY, name TEXT NOT NULL, price FLOAT)")
	mustExec(t, database, "INSERT INTO products (id, name, price) VALUES (1, 'Widget', 9.99)")

	result, err := database.Exec("SELECT id, name, price FROM products")
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}

	if len(result.Columns) != 3 {
		t.Fatalf("expected 3 columns, got %d: %v", len(result.Columns), result.Columns)
	}
	if result.Columns[0] != "id" || result.Columns[1] != "name" || result.Columns[2] != "price" {
		t.Fatalf("unexpected column names: %v", result.Columns)
	}
	if len(result.ColumnTypes) != 3 {
		t.Fatalf("expected 3 column types, got %d", len(result.ColumnTypes))
	}
	if result.ColumnTypes[0] != mysqlight.TypeInt {
		t.Errorf("expected TypeInt for id, got %v", result.ColumnTypes[0])
	}
	if result.ColumnTypes[1] != mysqlight.TypeText {
		t.Errorf("expected TypeText for name, got %v", result.ColumnTypes[1])
	}
	if result.ColumnTypes[2] != mysqlight.TypeFloat {
		t.Errorf("expected TypeFloat for price, got %v", result.ColumnTypes[2])
	}
}

func TestSelectStar(t *testing.T) {
	database, _ := openTestDB(t)
	mustExec(t, database, "CREATE TABLE nodes (id INTEGER PRIMARY KEY, value TEXT NOT NULL)")
	mustExec(t, database, "INSERT INTO nodes (id, value) VALUES (10, 'hello')")

	result, err := database.Exec("SELECT * FROM nodes")
	if err != nil {
		t.Fatalf("SELECT *: %v", err)
	}
	if len(result.Columns) != 2 {
		t.Fatalf("expected 2 columns for SELECT *, got %d", len(result.Columns))
	}
	if len(result.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result.Rows))
	}
}

func TestTransaction(t *testing.T) {
	// Phase 1: create schema and seed data, then close to flush pages to disk.
	path := filepath.Join(t.TempDir(), "txn.db")
	database, err := mysqlight.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	mustExec(t, database, "CREATE TABLE accounts (id INTEGER PRIMARY KEY, balance INTEGER NOT NULL)")
	mustExec(t, database, "INSERT INTO accounts (id, balance) VALUES (1, 100)")
	if err := database.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	// Phase 2: reopen so pages are clean, then run a transaction and roll it back.
	database2, err := mysqlight.Open(path)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer database2.Close()

	mustExec(t, database2, "BEGIN")
	mustExec(t, database2, "UPDATE accounts SET balance = 200 WHERE id = 1")
	mustExec(t, database2, "ROLLBACK")

	result, err := database2.Exec("SELECT balance FROM accounts WHERE id = 1")
	if err != nil {
		t.Fatalf("SELECT: %v", err)
	}
	if len(result.Rows) == 0 {
		t.Fatal("expected 1 row")
	}
	if got := mysqlight.FormatValue(result.Rows[0][0], mysqlight.TypeInt); got != "100" {
		t.Fatalf("expected balance 100 after rollback, got %s", got)
	}
}

func TestFormatValue(t *testing.T) {
	cases := []struct {
		value    mysqlight.Value
		dataType mysqlight.DataType
		want     string
	}{
		{mysqlight.Value{IsNull: true}, mysqlight.TypeInt, "NULL"},
		{mysqlight.Value{IntVal: 42}, mysqlight.TypeInt, "42"},
		{mysqlight.Value{IntVal: 0}, mysqlight.TypeInt, "0"},
		{mysqlight.Value{FloatVal: 3.14}, mysqlight.TypeFloat, "3.14"},
		{mysqlight.Value{TextVal: "hello"}, mysqlight.TypeText, "hello"},
		{mysqlight.Value{TextVal: ""}, mysqlight.TypeText, ""},
		{mysqlight.Value{BlobVal: []byte{1, 2, 3}}, mysqlight.TypeBlob, "<blob 3 bytes>"},
	}

	for _, testCase := range cases {
		got := mysqlight.FormatValue(testCase.value, testCase.dataType)
		if got != testCase.want {
			t.Errorf("FormatValue(%+v, %v) = %q, want %q", testCase.value, testCase.dataType, got, testCase.want)
		}
	}
}

func TestParseError(t *testing.T) {
	database, _ := openTestDB(t)
	_, err := database.Exec("this is not sql")
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}
