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

// NewExecutor initialises the executor with references to the catalog and transaction manager,
// which are needed to look up table definitions and acquire locks before any read or write.
func NewExecutor() *Executor {
	// TODO: store catalog and txn manager references
	panic("not implemented")
}

// Execute dispatches a parsed statement to the correct execution path. SELECT statements
// are converted to a physical operator pipeline and all rows are collected into a slice.
// DML statements (INSERT, UPDATE, DELETE) run their own execution logic directly. DDL
// statements (CREATE TABLE/INDEX, DROP TABLE/INDEX) are delegated to the catalog. Transaction
// statements (BEGIN, COMMIT, ROLLBACK) are forwarded to the transaction manager.
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

// collectRows opens an operator, drains it by calling Next until io.EOF is returned,
// closes it, and returns all accumulated rows.
func collectRows(op Operator) ([]Row, error) {
	// TODO: op.Open(), loop op.Next() until io.EOF, op.Close(), return rows
	panic("not implemented")
}
