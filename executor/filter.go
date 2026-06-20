package executor

// Filter wraps a child operator and skips rows where the WHERE expression is false.
type Filter struct {
	// TODO: child Operator
	// TODO: predicate Expr (from the AST)
	// TODO: schema []ColumnDef — needed to resolve column names to row indexes
}

func NewFilter(child Operator, predicate interface{}) *Filter {
	// TODO: store child and predicate
	panic("not implemented")
}

func (f *Filter) Open() error {
	// TODO: f.child.Open()
	panic("not implemented")
}

func (f *Filter) Next() (Row, error) {
	// TODO: loop:
	//   row, err := f.child.Next()
	//   if err != nil (including io.EOF) → return nil, err
	//   if evalExpr(f.predicate, row) == true → return row, nil
	//   otherwise continue to next row
	panic("not implemented")
}

func (f *Filter) Close() error {
	// TODO: f.child.Close()
	panic("not implemented")
}

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
