package mysqlight

import (
	"fmt"
	"strconv"

	"github.com/robertkoller/MySQLight/catalog"
	"github.com/robertkoller/MySQLight/executor"
	"github.com/robertkoller/MySQLight/parser"
	"github.com/robertkoller/MySQLight/storage"
	"github.com/robertkoller/MySQLight/txn"
	"github.com/robertkoller/MySQLight/wal"
)

// Value is a single typed database cell. Check IsNull before reading any field.
// Exactly one of IntVal, FloatVal, TextVal, or BlobVal carries the live value.
type Value = catalog.Value

// DataType is the column type tag — TypeInt, TypeFloat, TypeText, or TypeBlob.
type DataType = catalog.DataType

const (
	TypeInt   = catalog.TypeInt
	TypeFloat = catalog.TypeFloat
	TypeText  = catalog.TypeText
	TypeBlob  = catalog.TypeBlob
)

// Result holds the output of any SQL statement. For SELECT, Columns, ColumnTypes,
// and Rows are all populated. For DML and DDL they are nil.
type Result struct {
	Columns     []string   // output column names, in SELECT order
	ColumnTypes []DataType // column type for each column (TypeText when unknown)
	Rows        [][]Value  // one slice per row; nil for non-SELECT
}

// DB is an open MySQLight database. It is not safe for concurrent use from
// multiple goroutines; wrap access in a mutex if you need that.
type DB struct {
	pager   *storage.Pager
	pool    *storage.BufferPool
	cat     *catalog.Catalog
	walFile *wal.WAL
	txnMgr  *txn.TxnManager
	exec    *executor.Executor
}

// Open opens (or creates) the MySQLight database at path. If the database file
// exists and the WAL records uncommitted transactions from a previous crash,
// recovery is run automatically before the database is returned.
func Open(path string) (*DB, error) {
	pager, err := storage.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}

	// WAL crash recovery must run before any new writes touch the catalog.
	if err := wal.Recover(path, pager); err != nil {
		pager.Close()
		return nil, fmt.Errorf("wal recovery for %q: %w", path, err)
	}

	pool := storage.NewBufferPool(pager, 64)

	catalogRootPageID := pager.CatalogRootPageID()
	catalogTree, err := storage.NewBTree(pool, catalogRootPageID)
	if err != nil {
		pager.Close()
		return nil, fmt.Errorf("catalog btree: %w", err)
	}

	// For a brand-new database (no catalog tree yet), persist the root page ID
	// so that the next Open can reopen the existing catalog.
	if catalogRootPageID == 0 {
		if err := pager.SetCatalogRootPageID(catalogTree.RootPageID()); err != nil {
			pager.Close()
			return nil, fmt.Errorf("persist catalog root: %w", err)
		}
	}

	cat, err := catalog.Open(catalogTree)
	if err != nil {
		pager.Close()
		return nil, fmt.Errorf("catalog: %w", err)
	}

	walFile, err := wal.Open(path)
	if err != nil {
		pager.Close()
		return nil, fmt.Errorf("wal: %w", err)
	}
	pool.SetWAL(walFile)

	txnMgr := txn.NewTxnManager(walFile, pool)
	exec := executor.NewExecutor(cat, pool, txnMgr)

	return &DB{
		pager:   pager,
		pool:    pool,
		cat:     cat,
		walFile: walFile,
		txnMgr:  txnMgr,
		exec:    exec,
	}, nil
}

// Exec runs any SQL statement. For SELECT, Result.Rows and Result.Columns are
// populated. For DML (INSERT/UPDATE/DELETE) and DDL they are nil.
func (database *DB) Exec(sql string) (Result, error) {
	p, err := parser.NewParser(sql)
	if err != nil {
		return Result{}, fmt.Errorf("parse: %w", err)
	}
	stmt, err := p.Parse()
	if err != nil {
		return Result{}, fmt.Errorf("parse: %w", err)
	}

	rows, err := database.exec.Execute(stmt)
	if err != nil {
		return Result{}, err
	}

	if rows == nil {
		return Result{}, nil
	}

	selectStmt, isSelect := stmt.(*parser.SelectStmt)
	if !isSelect {
		return Result{}, nil
	}

	columns, columnTypes := database.resolveColumns(selectStmt)

	result := Result{
		Columns:     columns,
		ColumnTypes: columnTypes,
	}
	for _, row := range rows {
		converted := make([]Value, len(row))
		copy(converted, row)
		result.Rows = append(result.Rows, converted)
	}
	return result, nil
}

// Close flushes all dirty pages and closes the database and WAL files.
// Always call Close when finished, even on error paths.
func (database *DB) Close() error {
	if err := database.pool.FlushAll(); err != nil {
		return fmt.Errorf("flush: %w", err)
	}
	if err := database.walFile.Close(); err != nil {
		return fmt.Errorf("wal close: %w", err)
	}
	return database.pager.Close()
}

// FormatValue returns the string representation of a database value. Pass the
// column's DataType so that zero integers and empty strings are displayed correctly.
func FormatValue(value Value, dataType DataType) string {
	if value.IsNull {
		return "NULL"
	}
	switch dataType {
	case catalog.TypeInt:
		return strconv.FormatInt(value.IntVal, 10)
	case catalog.TypeFloat:
		return strconv.FormatFloat(value.FloatVal, 'f', -1, 64)
	case catalog.TypeText:
		return value.TextVal
	case catalog.TypeBlob:
		return fmt.Sprintf("<blob %d bytes>", len(value.BlobVal))
	}
	return ""
}

// resolveColumns derives output column names and types from a SELECT statement.
// For SELECT *, all columns from the FROM table are enumerated.
// For named column references, the type is looked up in the catalog.
// Aggregate functions and expressions fall back to TypeInt and TypeText respectively.
func (database *DB) resolveColumns(stmt *parser.SelectStmt) ([]string, []DataType) {
	var names []string
	var types []DataType

	for _, expression := range stmt.Columns {
		switch typed := expression.(type) {
		case *parser.StarExpr:
			tableDef, err := database.cat.GetTable(stmt.From)
			if err == nil {
				for _, column := range tableDef.Columns {
					names = append(names, column.Name)
					types = append(types, column.Type)
				}
			}
		case *parser.ColumnRef:
			names = append(names, typed.Column)
			columnType := catalog.TypeText
			tableDef, err := database.cat.GetTable(stmt.From)
			if err == nil {
				for _, column := range tableDef.Columns {
					if column.Name == typed.Column {
						columnType = column.Type
						break
					}
				}
			}
			types = append(types, columnType)
		case *parser.FuncCall:
			if typed.Star {
				names = append(names, typed.Name+"(*)")
			} else {
				names = append(names, typed.Name)
			}
			types = append(types, catalog.TypeInt)
		default:
			names = append(names, "expr")
			types = append(types, catalog.TypeText)
		}
	}
	return names, types
}
