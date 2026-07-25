package executor

import (
	"encoding/binary"
	"fmt"
	"io"

	"github.com/robertkoller/MySQLight/catalog"
	"github.com/robertkoller/MySQLight/parser"
	"github.com/robertkoller/MySQLight/storage"
)

func (e *Executor) executeDelete(statement *parser.DeleteStmt) error {
	definition, err := e.catalog.GetTable(statement.Table)
	if err != nil {
		return fmt.Errorf("table %q not found: %w", statement.Table, err)
	}

	dataTree, err := storage.NewBTree(e.pool, definition.RootPageID)
	if err != nil {
		return err
	}

	var scan Operator = NewTableScan(dataTree, definition.Columns)
	if statement.Where != nil {
		scan = NewFilter(scan, statement.Where, definition.Columns)
	}

	if err := scan.Open(); err != nil {
		return err
	}

	// Collect all matching rows before deleting to avoid mutating the tree mid-scan.
	var matchingRows []Row
	for {
		row, err := scan.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			scan.Close()
			return err
		}
		matchingRows = append(matchingRows, row)
	}
	if err := scan.Close(); err != nil {
		return err
	}

	// TODO: write WAL before-image record (Phase 4)

	return e.deleteRowsFromTable(definition, matchingRows)
}

// encodeKeyValue encodes a catalog.Value as big-endian bytes for use as a B+ tree key.
// TypeInt → 8-byte big-endian uint64. TypeText → raw UTF-8 bytes.
func encodeKeyValue(value catalog.Value, dataType catalog.DataType) ([]byte, error) {
	if value.IsNull {
		return nil, fmt.Errorf("cannot use NULL as a B+ tree key")
	}
	switch dataType {
	case catalog.TypeInt:
		key := make([]byte, 8)
		binary.BigEndian.PutUint64(key, uint64(value.IntVal))
		return key, nil
	case catalog.TypeText:
		return []byte(value.TextVal), nil
	default:
		return nil, fmt.Errorf("unsupported key type %v", dataType)
	}
}

// findColumnIndex returns the index of the first column matching the predicate, or -1.
func findColumnIndex(columns []catalog.ColumnDef, match func(catalog.ColumnDef) bool) int {
	for index, col := range columns {
		if match(col) {
			return index
		}
	}
	return -1
}
