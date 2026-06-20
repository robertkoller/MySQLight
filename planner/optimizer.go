package planner

// Optimizer applies rule-based rewrites to a logical plan tree, then
// selects physical operators (e.g. choosing IndexScan over TableScan).
type Optimizer struct {
	// TODO: catalog reference — needed for index lookup and statistics
}

func NewOptimizer() *Optimizer {
	panic("not implemented")
}

func (o *Optimizer) Optimize(root LogicalNode) (LogicalNode, error) {
	// TODO: apply rewrite rules in order:
	//   1. predicatePushdown(root)
	//   2. constantFolding(root)
	//   3. columnPruning(root)
	//   4. indexSelection(root)
	// TODO: return the rewritten tree
	panic("not implemented")
}

func predicatePushdown(node LogicalNode) LogicalNode {
	// TODO: walk the tree looking for LogicalFilter nodes sitting above LogicalJoin nodes
	// TODO: if the filter predicate only references columns from one side of the join,
	//         push it down below the join so it filters earlier (fewer rows reach the join)
	panic("not implemented")
}

func constantFolding(node LogicalNode) LogicalNode {
	// TODO: walk all Expr nodes in predicates and projections
	// TODO: if both sides of a BinaryExpr are Literals: evaluate now and replace with result
	//         e.g. "2 + 3" → Literal{5}, "1 = 1" → Literal{true}
	panic("not implemented")
}

func columnPruning(node LogicalNode) LogicalNode {
	// TODO: walk the tree top-down, collecting the set of columns actually referenced
	// TODO: trim any columns from LogicalProject and LogicalScan that are not in that set
	panic("not implemented")
}

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
