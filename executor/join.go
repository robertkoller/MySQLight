package executor

import "io"

// NestedLoopJoin implements the simplest join: for each row from the left
// operator, scan the entire right operator looking for matching rows.
// O(n*m) — fine for small tables; the planner can swap in better joins later.
type NestedLoopJoin struct {
	// TODO: left, right Operator
	// TODO: condition   Expr   — the ON expression from the AST
	// TODO: leftRow     Row    — the current row from the left side
	// TODO: rightExhausted bool
}

// NewNestedLoopJoin stores the left and right operators and the join condition expression.
func NewNestedLoopJoin(left, right Operator, condition interface{}) *NestedLoopJoin {
	panic("not implemented")
}

// Open opens both child operators and fetches the first left row to prime the outer loop.
func (j *NestedLoopJoin) Open() error {
	// TODO: j.left.Open(), j.right.Open()
	// TODO: fetch the first left row into j.leftRow
	panic("not implemented")
}

// Next implements the nested-loop algorithm. For the current left row it advances the right
// operator; when the right side is exhausted, it rewinds the right operator (Close then Open)
// and advances to the next left row. Each combined row is evaluated against the join condition,
// and matching rows are returned as a concatenation of the left and right values. Returns
// io.EOF when the left side is also exhausted.
func (j *NestedLoopJoin) Next() (Row, error) {
	// TODO: outer loop: advance j.leftRow when the right side is exhausted
	//   for each j.leftRow:
	//     inner loop: call j.right.Next()
	//       if io.EOF: rewind the right side (Close + Open), advance left, continue
	//       evaluate the join condition with the combined row
	//       if true: return the concatenated row (leftRow + rightRow)
	// TODO: return io.EOF when j.left is also exhausted
	panic("not implemented")
}

// Close closes both child operators.
func (j *NestedLoopJoin) Close() error {
	// TODO: j.left.Close(), j.right.Close()
	panic("not implemented")
}

var _ = io.EOF
