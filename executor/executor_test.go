package executor

import "testing"

func TestCreateAndInsert(t *testing.T) {
	// TODO: CREATE TABLE, INSERT several rows, assert no errors
	t.Skip("not implemented")
}

func TestSelectWhereOrderLimit(t *testing.T) {
	// TODO: insert rows with varied ages
	// TODO: SELECT with WHERE age > X, ORDER BY name, LIMIT N
	// TODO: assert result count, correct rows, correct order
	t.Skip("not implemented")
}

func TestUpdateAndDelete(t *testing.T) {
	// TODO: insert rows, UPDATE one, assert the value changed
	// TODO: DELETE one, assert it no longer appears in SELECT
	t.Skip("not implemented")
}

func TestJoin(t *testing.T) {
	// TODO: create users and orders tables, insert related rows
	// TODO: SELECT with JOIN ON user_id = users.id
	// TODO: assert joined rows are correct
	t.Skip("not implemented")
}

func TestAggregate(t *testing.T) {
	// TODO: insert rows, SELECT COUNT(*), AVG(age), MIN(age), MAX(age)
	// TODO: assert computed values are correct
	t.Skip("not implemented")
}

func TestForeignKeyRestrict(t *testing.T) {
	// TODO: create parent + child tables with ON DELETE RESTRICT
	// TODO: insert a parent row and a referencing child row
	// TODO: attempt to delete the parent row, assert FK violation error
	t.Skip("not implemented")
}

func TestForeignKeyCascade(t *testing.T) {
	// TODO: ON DELETE CASCADE: delete parent, assert child rows are also deleted
	t.Skip("not implemented")
}
