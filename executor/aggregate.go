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

// NewAggregate stores the child operator, GROUP BY expressions, and aggregate function
// expressions that will be evaluated once all input rows have been consumed.
func NewAggregate(child Operator, groupBy []interface{}, aggExprs []interface{}) *Aggregate {
	panic("not implemented")
}

// Open opens the child operator, consumes all rows from it, and groups them by the GROUP BY
// key while accumulating aggregate state (count, sum, min, max). After all rows are read,
// the final results are materialised into a slice so Next can return them one at a time.
func (a *Aggregate) Open() error {
	// TODO: a.child.Open()
	// TODO: consume all rows from child, build the groups map
	// TODO: after all rows are consumed, materialise a.results from the groups map
	panic("not implemented")
}

// Next returns aggregated result rows one at a time from the materialised result slice,
// advancing an internal cursor. Returns io.EOF when all groups have been returned.
func (a *Aggregate) Next() (Row, error) {
	// TODO: if a.cursor >= len(a.results) → return nil, io.EOF
	// TODO: return a.results[a.cursor]; a.cursor++
	panic("not implemented")
}

// Close closes the child operator.
func (a *Aggregate) Close() error {
	// TODO: a.child.Close()
	panic("not implemented")
}

var _ = io.EOF
