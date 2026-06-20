package executor

import "io"

var _ = io.EOF // ensure io is used

// Row is an ordered slice of values for one result row.
// TODO: replace interface{} with a proper Value type from the catalog package.
type Row []interface{}

// Operator is the iterator interface every physical operator implements.
type Operator interface {
	Open() error          // initialise internal state, open child operators
	Next() (Row, error)   // return the next row; return io.EOF when exhausted
	Close() error         // release resources
}

// Executor dispatches a parsed Statement to the correct execution path.
type Executor struct {
	// TODO: catalog reference — needed to look up table and index definitions
	// TODO: txn reference    — needed to acquire locks before reads/writes
}

func NewExecutor() *Executor {
	// TODO: store catalog and txn manager references
	panic("not implemented")
}

func (e *Executor) Execute(stmt interface{}) ([]Row, error) {
	// TODO: type-switch on stmt:
	//   *parser.SelectStmt      → buildSelectPlan → collect all rows
	//   *parser.InsertStmt      → executeInsert
	//   *parser.UpdateStmt      → executeUpdate
	//   *parser.DeleteStmt      → executeDelete
	//   *parser.CreateTableStmt → catalog.CreateTable
	//   *parser.CreateIndexStmt → catalog.CreateIndex
	//   *parser.DropTableStmt   → catalog.DropTable
	//   *parser.DropIndexStmt   → catalog.DropIndex
	//   *parser.BeginStmt       → txn.Begin
	//   *parser.CommitStmt      → txn.Commit
	//   *parser.RollbackStmt    → txn.Rollback
	//   *parser.AnalyzeStmt     → runAnalyze
	panic("not implemented")
}

func collectRows(op Operator) ([]Row, error) {
	// TODO: op.Open(), loop op.Next() until io.EOF, op.Close(), return rows
	panic("not implemented")
}
