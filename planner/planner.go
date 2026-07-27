package planner

import (
	"fmt"
	"strings"

	"github.com/robertkoller/MySQLight/catalog"
	"github.com/robertkoller/MySQLight/parser"
)

// LogicalNode is the interface every logical plan node implements.
// The logical plan mirrors the physical operator tree but contains no physical
// decisions — it says "scan table X" not "use index Y on column Z".
type LogicalNode interface {
	logicalNode()
	Children() []LogicalNode
}

type LogicalScan struct {
	Table   string
	Columns []catalog.ColumnDef
}

// LogicalIndexScan replaces a LogicalScan+LogicalFilter pair when the optimizer
// determines that an index on the filtered column makes a range or point scan cheaper.
type LogicalIndexScan struct {
	Table   string
	Index   string
	Column  string
	Columns []catalog.ColumnDef
	Exact   parser.Expr // non-nil for equality predicate (col = value)
	Low     parser.Expr // non-nil for range lower bound (col >= value)
	High    parser.Expr // non-nil for range upper bound (col <= value)
}

type LogicalFilter struct {
	Child     LogicalNode
	Predicate parser.Expr
}

type LogicalProject struct {
	Child   LogicalNode
	Columns []parser.Expr
}

type LogicalJoin struct {
	Left      LogicalNode
	Right     LogicalNode
	Condition parser.Expr
}

type LogicalAggregate struct {
	Child   LogicalNode
	GroupBy []parser.Expr
	Aggs    []parser.Expr
}

type LogicalSort struct {
	Child   LogicalNode
	OrderBy []parser.OrderClause
}

type LogicalLimit struct {
	Child LogicalNode
	N     int
}

func (n *LogicalScan) logicalNode()      {}
func (n *LogicalIndexScan) logicalNode() {}
func (n *LogicalFilter) logicalNode()    {}
func (n *LogicalProject) logicalNode()   {}
func (n *LogicalJoin) logicalNode()      {}
func (n *LogicalAggregate) logicalNode() {}
func (n *LogicalSort) logicalNode()      {}
func (n *LogicalLimit) logicalNode()     {}

func (n *LogicalScan) Children() []LogicalNode      { return nil }
func (n *LogicalIndexScan) Children() []LogicalNode { return nil }
func (n *LogicalFilter) Children() []LogicalNode    { return []LogicalNode{n.Child} }
func (n *LogicalProject) Children() []LogicalNode   { return []LogicalNode{n.Child} }
func (n *LogicalJoin) Children() []LogicalNode      { return []LogicalNode{n.Left, n.Right} }
func (n *LogicalAggregate) Children() []LogicalNode { return []LogicalNode{n.Child} }
func (n *LogicalSort) Children() []LogicalNode      { return []LogicalNode{n.Child} }
func (n *LogicalLimit) Children() []LogicalNode     { return []LogicalNode{n.Child} }

// Planner converts a parsed AST statement into a logical plan tree.
// The plan is built bottom-up: scan → filter → join(s) → aggregate/project → sort → limit.
type Planner struct {
	catalog *catalog.Catalog
}

// NewPlanner initialises the planner with a reference to the catalog, which is used
// to validate table names, resolve column definitions, and detect available indexes.
func NewPlanner(cat *catalog.Catalog) *Planner {
	return &Planner{catalog: cat}
}

// Plan converts a parsed statement into a logical plan tree. Only SELECT statements
// are planned here; DML goes directly to the executor without a logical plan.
func (p *Planner) Plan(stmt interface{}) (LogicalNode, error) {
	switch typed := stmt.(type) {
	case *parser.SelectStmt:
		return p.planSelect(typed)
	default:
		return nil, fmt.Errorf("planner: cannot plan statement of type %T", stmt)
	}
}

// planSelect builds a logical plan tree for a SELECT statement bottom-up.
func (p *Planner) planSelect(stmt *parser.SelectStmt) (LogicalNode, error) {
	if stmt.From == "" {
		return nil, fmt.Errorf("planner: SELECT requires a FROM clause")
	}

	tableDef, err := p.catalog.GetTable(stmt.From)
	if err != nil {
		return nil, fmt.Errorf("planner: table %q not found: %w", stmt.From, err)
	}

	var current LogicalNode = &LogicalScan{Table: stmt.From, Columns: tableDef.Columns}

	if stmt.Where != nil {
		current = &LogicalFilter{Child: current, Predicate: stmt.Where}
	}

	for _, join := range stmt.Joins {
		joinedDef, joinErr := p.catalog.GetTable(join.Table)
		if joinErr != nil {
			return nil, fmt.Errorf("planner: join table %q not found: %w", join.Table, joinErr)
		}
		current = &LogicalJoin{
			Left:      current,
			Right:     &LogicalScan{Table: join.Table, Columns: joinedDef.Columns},
			Condition: join.On,
		}
	}

	if len(stmt.GroupBy) > 0 || containsAggregate(stmt.Columns) {
		current = &LogicalAggregate{Child: current, GroupBy: stmt.GroupBy, Aggs: stmt.Columns}
		if stmt.Having != nil {
			current = &LogicalFilter{Child: current, Predicate: stmt.Having}
		}
	} else {
		current = &LogicalProject{Child: current, Columns: stmt.Columns}
	}

	if len(stmt.OrderBy) > 0 {
		current = &LogicalSort{Child: current, OrderBy: stmt.OrderBy}
	}

	if stmt.Limit != nil {
		current = &LogicalLimit{Child: current, N: *stmt.Limit}
	}

	return current, nil
}

func containsAggregate(exprs []parser.Expr) bool {
	for _, expr := range exprs {
		if hasAggregateFuncCall(expr) {
			return true
		}
	}
	return false
}

func hasAggregateFuncCall(expr parser.Expr) bool {
	switch typed := expr.(type) {
	case *parser.FuncCall:
		name := strings.ToUpper(typed.Name)
		return name == "COUNT" || name == "SUM" || name == "MIN" || name == "MAX" || name == "AVG"
	case *parser.BinaryExpr:
		return hasAggregateFuncCall(typed.Left) || hasAggregateFuncCall(typed.Right)
	case *parser.UnaryExpr:
		return hasAggregateFuncCall(typed.Operand)
	}
	return false
}
