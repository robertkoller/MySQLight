package planner

import "testing"

func TestPlanSimpleSelect(t *testing.T) {
	// TODO: parse "SELECT id, name FROM users WHERE age > 25"
	// TODO: call planner.Plan, assert the tree is: Project → Filter → Scan
	t.Skip("not implemented")
}

func TestPredicatePushdown(t *testing.T) {
	// TODO: build a plan with a filter above a join where the predicate references only one table
	// TODO: after optimization, assert the filter has moved below the join
	t.Skip("not implemented")
}

func TestConstantFolding(t *testing.T) {
	// TODO: parse "SELECT * FROM users WHERE 2 + 3 > 4"
	// TODO: after optimization, assert the expression has been folded to a literal
	t.Skip("not implemented")
}

func TestIndexSelection(t *testing.T) {
	// TODO: set up a catalog with an index on users.age
	// TODO: plan "SELECT * FROM users WHERE age = 30"
	// TODO: after optimization, assert the scan node is an IndexScan, not a TableScan
	t.Skip("not implemented")
}
