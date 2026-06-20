package parser

import "testing"

func TestParseSelect(t *testing.T) {
	// TODO: parse "SELECT id, name FROM users WHERE age > 25 ORDER BY name DESC LIMIT 10"
	// TODO: assert Columns, From, Where, OrderBy, Limit are all correct
	t.Skip("not implemented")
}

func TestParseInsert(t *testing.T) {
	// TODO: parse "INSERT INTO users (id, name) VALUES (1, 'Alice'), (2, 'Bob')"
	// TODO: assert table name, column list, and both value rows
	t.Skip("not implemented")
}

func TestParseCreateTable(t *testing.T) {
	// TODO: parse a CREATE TABLE with PRIMARY KEY, NOT NULL, DEFAULT, and a FOREIGN KEY
	// TODO: assert all column defs and the foreign key constraint
	t.Skip("not implemented")
}

func TestParseExprPrecedence(t *testing.T) {
	// TODO: parse "a + b * c" and assert * binds tighter than +
	// TODO: parse "NOT a AND b OR c" and assert NOT > AND > OR
	t.Skip("not implemented")
}

func TestParseTransactions(t *testing.T) {
	// TODO: parse BEGIN, COMMIT, ROLLBACK — assert correct statement types
	t.Skip("not implemented")
}
