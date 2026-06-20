package planner

// Optimizer applies rule-based rewrites to a logical plan tree, then
// selects physical operators (e.g. choosing IndexScan over TableScan).
type Optimizer struct {
	// TODO: catalog reference — needed for index lookup and statistics
}

// NewOptimizer initialises the optimizer with a reference to the catalog, which is needed
// for index metadata and table statistics during the index selection pass.
func NewOptimizer() *Optimizer {
	panic("not implemented")
}

// Optimize applies a fixed sequence of rule-based rewrites to the logical plan tree:
// predicate pushdown, constant folding, column pruning, and then index selection.
// The rewritten tree is returned for physical operator selection.
func (o *Optimizer) Optimize(root LogicalNode) (LogicalNode, error) {
	// TODO: apply rewrite rules in order:
	//   1. predicatePushdown(root)
	//   2. constantFolding(root)
	//   3. columnPruning(root)
	//   4. indexSelection(root)
	// TODO: return the rewritten tree
	panic("not implemented")
}

// predicatePushdown walks the plan tree looking for LogicalFilter nodes sitting above
// LogicalJoin nodes. If the filter predicate only references columns from one side of the
// join, it is moved below the join so fewer rows are produced before the join executes.
func predicatePushdown(node LogicalNode) LogicalNode {
	// TODO: walk the tree looking for LogicalFilter nodes sitting above LogicalJoin nodes
	// TODO: if the filter predicate only references columns from one side of the join,
	//         push it down below the join so it filters earlier (fewer rows reach the join)
	panic("not implemented")
}

// constantFolding walks all expression nodes in predicates and projections. If both sides
// of a BinaryExpr are literals, the expression is evaluated at plan time and replaced with
// its result, for example turning "2 + 3" into Literal{5} or "1 = 1" into Literal{true}.
func constantFolding(node LogicalNode) LogicalNode {
	// TODO: walk all Expr nodes in predicates and projections
	// TODO: if both sides of a BinaryExpr are Literals: evaluate now and replace with result
	//         e.g. "2 + 3" → Literal{5}, "1 = 1" → Literal{true}
	panic("not implemented")
}

// columnPruning walks the plan tree top-down to collect the set of columns actually
// referenced by the query. Any columns loaded by scan nodes or passed through project
// nodes that are not in that set are removed, reducing unnecessary data movement.
func columnPruning(node LogicalNode) LogicalNode {
	// TODO: walk the tree top-down, collecting the set of columns actually referenced
	// TODO: trim any columns from LogicalProject and LogicalScan that are not in that set
	panic("not implemented")
}

// indexSelection finds LogicalFilter nodes with equality or range predicates on a single
// column that sit directly above a LogicalScan. If the catalog has an index on that column,
// it replaces the TableScan+Filter pair with a LogicalIndexScan. A simple cost model
// confirms the index is cheaper: TableScan cost is the page count of the table, while
// IndexScan cost is the index height plus the estimated number of matching rows.
func (o *Optimizer) indexSelection(node LogicalNode) LogicalNode {
	// TODO: find LogicalFilter(col = value) or LogicalFilter(col BETWEEN a AND b) nodes
	//         that sit directly above a LogicalScan
	// TODO: check the catalog for an index on that column
	// TODO: if an index exists: replace LogicalScan+LogicalFilter with LogicalIndexScan
	// TODO: use a simple cost model to decide:
	//         TableScan cost  = number of pages in the table
	//         IndexScan cost  = index height + estimated matching rows
	panic("not implemented")
}
