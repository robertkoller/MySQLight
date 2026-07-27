package executor

import (
	"fmt"
	"io"

	"github.com/robertkoller/MySQLight/catalog"
	"github.com/robertkoller/MySQLight/parser"
	"github.com/robertkoller/MySQLight/storage"
)

func (e *Executor) executeUpdate(statement *parser.UpdateStmt) error {
	if err := e.acquireExclusive(statement.Table); err != nil {
		return err
	}
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

	// Collect matching rows before mutating the tree.
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

	pkIndex := findColumnIndex(definition.Columns, func(col catalog.ColumnDef) bool {
		return col.PrimaryKey
	})
	if pkIndex < 0 {
		return fmt.Errorf("table %q has no primary key", statement.Table)
	}

	indexes, err := e.catalog.IndexesForTable(statement.Table)
	if err != nil {
		return err
	}

	for _, oldRow := range matchingRows {
		newRow := append(Row{}, oldRow...)

		for _, assignment := range statement.Set {
			colIndex := findColumnIndex(definition.Columns, func(col catalog.ColumnDef) bool {
				return col.Name == assignment.Column
			})
			if colIndex < 0 {
				return fmt.Errorf("column %q not found in table %q", assignment.Column, statement.Table)
			}
			newValue, err := evalExprToValue(assignment.Value, definition.Columns[colIndex].Type)
			if err != nil {
				return err
			}
			newRow[colIndex] = newValue
		}

		for index, col := range definition.Columns {
			if col.NotNull && newRow[index].IsNull {
				return fmt.Errorf("column %q violates NOT NULL constraint", col.Name)
			}
		}

		oldPKBytes, err := encodeKeyValue(oldRow[pkIndex], definition.Columns[pkIndex].Type)
		if err != nil {
			return fmt.Errorf("encoding old primary key: %w", err)
		}
		newPKBytes, err := encodeKeyValue(newRow[pkIndex], definition.Columns[pkIndex].Type)
		if err != nil {
			return fmt.Errorf("encoding new primary key: %w", err)
		}

		if err := e.verifyFKParents(newRow, definition); err != nil {
			return err
		}

		pkType := definition.Columns[pkIndex].Type
		if !valuesEqual(oldRow[pkIndex], newRow[pkIndex], pkType) {
			if err := e.applyOnUpdate(statement.Table, oldRow[pkIndex], newRow[pkIndex], pkType); err != nil {
				return err
			}
		}

		if err := dataTree.Delete(oldPKBytes); err != nil {
			return fmt.Errorf("deleting old row: %w", err)
		}
		newRowBytes := encodeRow(newRow, definition.Columns)
		if err := dataTree.Insert(newPKBytes, newRowBytes); err != nil {
			return fmt.Errorf("inserting updated row: %w", err)
		}

		for _, indexDef := range indexes {
			colIndex := findColumnIndex(definition.Columns, func(col catalog.ColumnDef) bool {
				return col.Name == indexDef.ColumnName
			})
			if colIndex < 0 {
				continue
			}

			oldIndexKeyBytes, err := encodeKeyValue(oldRow[colIndex], definition.Columns[colIndex].Type)
			if err != nil {
				return fmt.Errorf("encoding old index key for %q: %w", indexDef.Name, err)
			}
			newIndexKeyBytes, err := encodeKeyValue(newRow[colIndex], definition.Columns[colIndex].Type)
			if err != nil {
				return fmt.Errorf("encoding new index key for %q: %w", indexDef.Name, err)
			}

			indexTree, err := storage.NewBTree(e.pool, indexDef.RootPageID)
			if err != nil {
				return err
			}
			if err := indexTree.Delete(oldIndexKeyBytes); err != nil {
				return fmt.Errorf("deleting old index entry for %q: %w", indexDef.Name, err)
			}
			if err := indexTree.Insert(newIndexKeyBytes, newPKBytes); err != nil {
				return fmt.Errorf("inserting new index entry for %q: %w", indexDef.Name, err)
			}
		}
	}

	return nil
}
