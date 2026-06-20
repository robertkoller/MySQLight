package executor

import "io"

// Aggregate consumes all rows from its child and computes grouped aggregates
// (COUNT, SUM, MIN, MAX, AVG) with an optional GROUP BY.
type Aggregate struct {
	// TODO: child    Operator
	// TODO: groupBy  []Expr
	// TODO: aggExprs []Expr  — the aggregate function calls in the SELECT list
	// TODO: groups   map[string]*aggState  — key = serialised group-by values
	// TODO: results  []Row   — materialised after consuming all input
	// TODO: cursor   int     — index into results for Next() calls
}

type aggState struct {
	// TODO: count int64
	// TODO: sum   float64
	// TODO: min   interface{}
	// TODO: max   interface{}
}

func NewAggregate(child Operator, groupBy []interface{}, aggExprs []interface{}) *Aggregate {
	panic("not implemented")
}

func (a *Aggregate) Open() error {
	// TODO: a.child.Open()
	// TODO: consume all rows from child, build the groups map
	// TODO: after all rows are consumed, materialise a.results from the groups map
	panic("not implemented")
}

func (a *Aggregate) Next() (Row, error) {
	// TODO: if a.cursor >= len(a.results) → return nil, io.EOF
	// TODO: return a.results[a.cursor]; a.cursor++
	panic("not implemented")
}

func (a *Aggregate) Close() error {
	// TODO: a.child.Close()
	panic("not implemented")
}

var _ = io.EOF
