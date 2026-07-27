package executor

import (
	"fmt"
	"io"
	"strings"

	"github.com/robertkoller/MySQLight/catalog"
	"github.com/robertkoller/MySQLight/parser"
	"github.com/robertkoller/MySQLight/storage"
	"github.com/robertkoller/MySQLight/txn"
)

// Row is an ordered slice of values for one result row.
type Row []catalog.Value

// Operator is the iterator interface every physical operator implements.
type Operator interface {
	Open() error        // initialise internal state, open child operators
	Next() (Row, error) // return the next row; return io.EOF when exhausted
	Close() error       // release resources
}

// Executor dispatches a parsed Statement to the correct execution path.
type Executor struct {
	catalog    *catalog.Catalog
	pool       *storage.BufferPool
	txnManager *txn.TxnManager
	currentTxn *txn.Txn
}

// NewExecutor initialises the executor with references to the catalog, buffer pool, and
// an optional transaction manager. Pass nil for txnManager to disable transaction support.
func NewExecutor(cat *catalog.Catalog, pool *storage.BufferPool, txnManager *txn.TxnManager) *Executor {
	return &Executor{catalog: cat, pool: pool, txnManager: txnManager}
}

// acquireExclusive acquires an exclusive table lock for the current transaction.
// It is a no-op when no transaction is active or no txnManager is configured.
func (e *Executor) acquireExclusive(tableName string) error {
	if e.currentTxn == nil || e.txnManager == nil {
		return nil
	}
	return e.txnManager.LockManager().Acquire(e.currentTxn.ID, tableName, txn.LockExclusive)
}

// Execute dispatches a parsed statement to the correct execution path. SELECT
// statements are converted to a physical operator pipeline and all rows are
// collected. DDL delegates to the catalog. DELETE uses executeDelete. INSERT and
// UPDATE are not yet implemented.
func (e *Executor) Execute(stmt interface{}) ([]Row, error) {
	switch typed := stmt.(type) {
	case *parser.SelectStmt:
		pipeline, err := e.buildSelectPipeline(typed)
		if err != nil {
			return nil, err
		}
		return collectRows(pipeline)

	case *parser.DeleteStmt:
		return nil, e.executeDelete(typed)

	case *parser.CreateTableStmt:
		definition, err := astToTableDef(typed)
		if err != nil {
			return nil, err
		}
		return nil, e.catalog.CreateTable(definition)

	case *parser.DropTableStmt:
		return nil, e.catalog.DropTable(typed.Table)

	case *parser.CreateIndexStmt:
		definition := &catalog.IndexDef{
			Name:       typed.Index,
			TableName:  typed.Table,
			ColumnName: typed.Column,
			Unique:     typed.Unique,
		}
		return nil, e.catalog.CreateIndex(definition)

	case *parser.DropIndexStmt:
		return nil, e.catalog.DropIndex(typed.Index)

	case *parser.InsertStmt:
		return nil, e.executeInsert(typed)

	case *parser.UpdateStmt:
		return nil, e.executeUpdate(typed)

	case *parser.BeginStmt:
		if e.txnManager == nil {
			return nil, fmt.Errorf("transaction manager not configured")
		}
		if e.currentTxn != nil {
			return nil, fmt.Errorf("transaction already in progress")
		}
		txnObj, err := e.txnManager.Begin()
		if err != nil {
			return nil, err
		}
		e.currentTxn = txnObj
		e.pool.SetTxnID(txnObj.ID)
		return nil, nil

	case *parser.CommitStmt:
		if e.currentTxn == nil {
			return nil, fmt.Errorf("no transaction in progress")
		}
		if err := e.txnManager.Commit(e.currentTxn); err != nil {
			return nil, err
		}
		e.currentTxn = nil
		e.pool.SetTxnID(0)
		return nil, nil

	case *parser.RollbackStmt:
		if e.currentTxn == nil {
			return nil, fmt.Errorf("no transaction in progress")
		}
		if err := e.txnManager.Rollback(e.currentTxn); err != nil {
			return nil, err
		}
		e.currentTxn = nil
		e.pool.SetTxnID(0)
		return nil, nil

	case *parser.AnalyzeStmt:
		return nil, fmt.Errorf("ANALYZE not yet implemented")

	default:
		return nil, fmt.Errorf("unknown statement type %T", stmt)
	}
}

// collectRows opens op, drains it to io.EOF, closes it, and returns all rows.
func collectRows(op Operator) ([]Row, error) {
	if err := op.Open(); err != nil {
		return nil, err
	}
	var rows []Row
	for {
		row, err := op.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			op.Close()
			return nil, err
		}
		rows = append(rows, row)
	}
	if err := op.Close(); err != nil {
		return nil, err
	}
	return rows, nil
}

// buildSelectPipeline constructs the physical operator tree for a SELECT statement.
// Pipeline order: TableScan → Joins → Filter → Aggregate or (Sort → Project) → Limit.
//
// Known limitation: ORDER BY is applied before projection and references the
// pre-projection column schema. ORDER BY on aggregate results is not yet supported.
func (e *Executor) buildSelectPipeline(stmt *parser.SelectStmt) (Operator, error) {
	definition, err := e.catalog.GetTable(stmt.From)
	if err != nil {
		return nil, fmt.Errorf("table %q not found: %w", stmt.From, err)
	}

	dataTree, err := storage.NewBTree(e.pool, definition.RootPageID)
	if err != nil {
		return nil, err
	}

	leftScan := NewTableScan(dataTree, definition.Columns)
	if e.currentTxn != nil && e.txnManager != nil {
		leftScan.WithLock(e.currentTxn.ID, stmt.From, e.txnManager.LockManager())
	}
	var pipeline Operator = leftScan
	currentColumns := append([]catalog.ColumnDef{}, definition.Columns...)

	for _, join := range stmt.Joins {
		rightDef, err := e.catalog.GetTable(join.Table)
		if err != nil {
			return nil, fmt.Errorf("joined table %q not found: %w", join.Table, err)
		}
		rightTree, err := storage.NewBTree(e.pool, rightDef.RootPageID)
		if err != nil {
			return nil, err
		}
		rightScan := NewTableScan(rightTree, rightDef.Columns)
		if e.currentTxn != nil && e.txnManager != nil {
			rightScan.WithLock(e.currentTxn.ID, join.Table, e.txnManager.LockManager())
		}
		pipeline = NewNestedLoopJoin(pipeline, rightScan, join.On, currentColumns, rightDef.Columns)
		currentColumns = append(currentColumns, rightDef.Columns...)
	}

	if stmt.Where != nil {
		pipeline = NewFilter(pipeline, stmt.Where, currentColumns)
	}

	if len(stmt.GroupBy) > 0 || hasAggregates(stmt.Columns) {
		pipeline = NewAggregate(pipeline, stmt.GroupBy, stmt.Columns, currentColumns)
	} else {
		if len(stmt.OrderBy) > 0 {
			pipeline = NewSort(pipeline, stmt.OrderBy, currentColumns)
		}
		if !isSelectStar(stmt.Columns) {
			pipeline = NewProject(pipeline, stmt.Columns, currentColumns)
		}
	}

	if stmt.Limit != nil {
		pipeline = NewLimit(pipeline, *stmt.Limit)
	}

	return pipeline, nil
}

// hasAggregates reports whether any expression in the list is or contains a FuncCall.
func hasAggregates(exprs []parser.Expr) bool {
	for _, expr := range exprs {
		if containsAggregate(expr) {
			return true
		}
	}
	return false
}

func containsAggregate(expr parser.Expr) bool {
	if _, ok := expr.(*parser.FuncCall); ok {
		return true
	}
	switch typed := expr.(type) {
	case *parser.BinaryExpr:
		return containsAggregate(typed.Left) || containsAggregate(typed.Right)
	case *parser.UnaryExpr:
		return containsAggregate(typed.Operand)
	}
	return false
}

// isSelectStar reports whether the SELECT list is exactly [StarExpr].
func isSelectStar(exprs []parser.Expr) bool {
	if len(exprs) != 1 {
		return false
	}
	_, ok := exprs[0].(*parser.StarExpr)
	return ok
}

// astToTableDef converts a parser.CreateTableStmt into a catalog.TableDef.
func astToTableDef(stmt *parser.CreateTableStmt) (*catalog.TableDef, error) {
	columns := make([]catalog.ColumnDef, 0, len(stmt.Columns))
	for _, colAST := range stmt.Columns {
		dataType, err := parseDataType(colAST.TypeName)
		if err != nil {
			return nil, err
		}
		columns = append(columns, catalog.ColumnDef{
			Name:       colAST.Name,
			Type:       dataType,
			PrimaryKey: colAST.PrimaryKey,
			NotNull:    colAST.NotNull,
		})
	}

	foreignKeys := make([]catalog.ForeignKeyDef, 0, len(stmt.ForeignKeys))
	for _, fkAST := range stmt.ForeignKeys {
		foreignKeys = append(foreignKeys, catalog.ForeignKeyDef{
			ColumnName: fkAST.Column,
			RefTable:   fkAST.RefTable,
			RefColumn:  fkAST.RefColumn,
			OnDelete:   parseFKAction(fkAST.OnDelete),
			OnUpdate:   parseFKAction(fkAST.OnUpdate),
		})
	}

	return &catalog.TableDef{
		Name:        stmt.Table,
		Columns:     columns,
		ForeignKeys: foreignKeys,
	}, nil
}

func parseDataType(typeName string) (catalog.DataType, error) {
	switch strings.ToUpper(typeName) {
	case "INTEGER", "INT":
		return catalog.TypeInt, nil
	case "FLOAT", "REAL", "DOUBLE":
		return catalog.TypeFloat, nil
	case "TEXT", "VARCHAR", "CHAR":
		return catalog.TypeText, nil
	case "BLOB":
		return catalog.TypeBlob, nil
	default:
		return catalog.TypeText, fmt.Errorf("unknown data type %q", typeName)
	}
}

func parseFKAction(action string) catalog.FKAction {
	switch strings.ToUpper(action) {
	case "CASCADE":
		return catalog.FKCascade
	case "SET NULL":
		return catalog.FKSetNull
	default:
		return catalog.FKRestrict
	}
}
