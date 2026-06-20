package executor

// Project wraps a child operator and keeps only the selected columns,
// evaluating any expressions in the SELECT list.
type Project struct {
	// TODO: child   Operator
	// TODO: columns []Expr  — the SELECT column list from the AST
	// TODO: schema  []ColumnDef — input schema, needed to resolve column names
}

// NewProject stores the child operator and the list of SELECT column expressions to evaluate.
func NewProject(child Operator, columns []interface{}) *Project {
	// TODO: store child and columns
	panic("not implemented")
}

// Open opens the child operator.
func (p *Project) Open() error {
	// TODO: p.child.Open()
	panic("not implemented")
}

// Next fetches the next row from the child, evaluates each column expression against it,
// and returns a new Row containing only the projected values. Errors and io.EOF are
// propagated from the child.
func (p *Project) Next() (Row, error) {
	// TODO: row, err := p.child.Next(); propagate errors and io.EOF
	// TODO: for each column expression, call evalExpr(col, row) to get its value
	// TODO: return a new Row containing only those evaluated values
	panic("not implemented")
}

// Close closes the child operator.
func (p *Project) Close() error {
	// TODO: p.child.Close()
	panic("not implemented")
}
