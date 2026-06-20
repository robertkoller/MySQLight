package planner

// LogicalNode is the interface for all logical plan nodes.
// The logical plan mirrors the executor operator tree but contains no physical
// decisions yet (e.g. it says "scan table X" not "use index Y").
type LogicalNode interface {
	logicalNode()
	Children() []LogicalNode
}

type LogicalScan struct {
	Table string
}

type LogicalFilter struct {
	Child     LogicalNode
	Predicate interface{} // AST Expr
}

type LogicalProject struct {
	Child   LogicalNode
	Columns []interface{} // AST Expr list
}

type LogicalJoin struct {
	Left      LogicalNode
	Right     LogicalNode
	Condition interface{} // AST Expr
}

type LogicalAggregate struct {
	Child   LogicalNode
	GroupBy []interface{}
	Aggs    []interface{}
}

type LogicalSort struct {
	Child   LogicalNode
	OrderBy []interface{}
}

type LogicalLimit struct {
	Child LogicalNode
	N     int
}

func (n *LogicalScan) logicalNode()      {}
func (n *LogicalFilter) logicalNode()    {}
func (n *LogicalProject) logicalNode()   {}
func (n *LogicalJoin) logicalNode()      {}
func (n *LogicalAggregate) logicalNode() {}
func (n *LogicalSort) logicalNode()      {}
func (n *LogicalLimit) logicalNode()     {}

func (n *LogicalScan) Children() []LogicalNode      { return nil }
func (n *LogicalFilter) Children() []LogicalNode    { return []LogicalNode{n.Child} }
func (n *LogicalProject) Children() []LogicalNode   { return []LogicalNode{n.Child} }
func (n *LogicalJoin) Children() []LogicalNode      { return []LogicalNode{n.Left, n.Right} }
func (n *LogicalAggregate) Children() []LogicalNode { return []LogicalNode{n.Child} }
func (n *LogicalSort) Children() []LogicalNode      { return []LogicalNode{n.Child} }
func (n *LogicalLimit) Children() []LogicalNode     { return []LogicalNode{n.Child} }

// Planner converts a parsed AST statement into a logical plan tree.
// The plan is a tree, not a DAG — no topological sort needed.
// (If CTEs or subqueries are added later and the plan becomes a DAG,
// topological sort over named sub-plans would determine materialisation order.)
type Planner struct {
	// TODO: catalog reference — needed to validate table/column names
}

func NewPlanner() *Planner {
	panic("not implemented")
}

func (p *Planner) Plan(stmt interface{}) (LogicalNode, error) {
	// TODO: type-switch on stmt:
	//   *parser.SelectStmt → planSelect
	//   others             → error (DML goes straight to the executor, not the planner)
	panic("not implemented")
}

func (p *Planner) planSelect(stmt interface{}) (LogicalNode, error) {
	// TODO: start with LogicalScan{Table: stmt.From}
	// TODO: if stmt.Where != nil: wrap in LogicalFilter{Predicate: stmt.Where}
	// TODO: if stmt.Joins:        for each join, wrap in LogicalJoin
	// TODO: if stmt.GroupBy/Aggs: wrap in LogicalAggregate
	// TODO: wrap in LogicalProject{Columns: stmt.Columns}
	// TODO: if stmt.OrderBy:      wrap in LogicalSort
	// TODO: if stmt.Limit:        wrap in LogicalLimit
	panic("not implemented")
}
