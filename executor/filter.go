package executor

// Filter wraps a child operator and skips rows where the WHERE expression is false.
type Filter struct {
	// TODO: child Operator
	// TODO: predicate Expr (from the AST)
	// TODO: schema []ColumnDef — needed to resolve column names to row indexes
}

// NewFilter stores the child operator and the AST predicate expression that will be
// evaluated against each row to decide whether to pass it through.
func NewFilter(child Operator, predicate interface{}) *Filter {
	// TODO: store child and predicate
	panic("not implemented")
}

// Open opens the child operator to begin producing rows.
func (f *Filter) Open() error {
	// TODO: f.child.Open()
	panic("not implemented")
}

// Next repeatedly calls the child's Next until it finds a row for which the predicate
// evaluates to true, then returns that row. Errors and io.EOF from the child are
// propagated directly without filtering.
func (f *Filter) Next() (Row, error) {
	// TODO: loop:
	//   row, err := f.child.Next()
	//   if err != nil (including io.EOF) → return nil, err
	//   if evalExpr(f.predicate, row) == true → return row, nil
	//   otherwise continue to next row
	panic("not implemented")
}

// Close closes the child operator and releases any resources it holds.
func (f *Filter) Close() error {
	// TODO: f.child.Close()
	panic("not implemented")
}

// evalExpr evaluates an AST expression against a given row. Literals return their typed
// value directly. Column references are resolved to row indexes via the schema. Binary
// expressions evaluate both sides and apply the operator: arithmetic (+, -, *, /, %),
// comparison (=, !=, <, >, <=, >=, LIKE), or logical (AND, OR). Unary expressions apply
// NOT or numeric negation. Aggregate function calls are not valid in this context and
// return an error.
func evalExpr(expr interface{}, row Row) (interface{}, error) {
	// TODO: type-switch on expr:
	//   *Literal    → return the typed value (int64, float64, string, nil)
	//   *ColumnRef  → look up the column index in the schema, return row[i]
	//   *BinaryExpr → evalExpr(left), evalExpr(right), apply operator
	//                  arithmetic: +, -, *, /, %
	//                  comparison: =, !=, <, >, <=, >=  (return bool)
	//                  logical:    AND, OR              (both sides must be bool)
	//                  LIKE: simple % wildcard matching
	//   *UnaryExpr  → NOT (negate bool) or - (negate number)
	//   *FuncCall   → only valid inside Aggregate; return error here
	panic("not implemented")
}
