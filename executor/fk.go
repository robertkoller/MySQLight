package executor

import (
	"fmt"
	"io"

	"github.com/robertkoller/MySQLight/catalog"
	"github.com/robertkoller/MySQLight/storage"
)

// verifyFKParents checks that every FK column value in row exists in its referenced
// parent table. Called before INSERT and before UPDATE writes the new row.
func (e *Executor) verifyFKParents(row Row, definition *catalog.TableDef) error {
	for _, fk := range definition.ForeignKeys {
		colIndex := findColumnIndex(definition.Columns, func(col catalog.ColumnDef) bool {
			return col.Name == fk.ColumnName
		})
		if colIndex < 0 || row[colIndex].IsNull {
			continue // NULL FK values are permitted unless the column is also NOT NULL
		}
		parentDef, err := e.catalog.GetTable(fk.RefTable)
		if err != nil {
			return fmt.Errorf("FK: parent table %q not found: %w", fk.RefTable, err)
		}
		exists, err := e.columnContainsValue(parentDef, fk.RefColumn, row[colIndex])
		if err != nil {
			return err
		}
		if !exists {
			return fmt.Errorf("foreign key violation: %q.%q references non-existent value in %q.%q",
				definition.Name, fk.ColumnName, fk.RefTable, fk.RefColumn)
		}
	}
	return nil
}

// applyOnDelete enforces FK ON DELETE rules for all child tables that reference
// parentTable when the row identified by pkValue is about to be deleted.
// RESTRICT returns an error before any modification; CASCADE recursively deletes
// matching child rows; SET NULL nulls out the FK column in matching child rows.
//
// Note: without transactions (Phase 4), a CASCADE that partially succeeds before
// hitting an error leaves child data in a modified state.
func (e *Executor) applyOnDelete(parentTable string, pkValue catalog.Value) error {
	for _, childDef := range e.catalog.ListTables() {
		for _, fk := range childDef.ForeignKeys {
			if fk.RefTable != parentTable {
				continue
			}
			matching, err := e.findRowsMatchingValue(childDef, fk.ColumnName, pkValue)
			if err != nil {
				return err
			}
			if len(matching) == 0 {
				continue
			}
			switch fk.OnDelete {
			case catalog.FKRestrict:
				return fmt.Errorf("foreign key violation: cannot delete from %q: %d row(s) in %q reference this row",
					parentTable, len(matching), childDef.Name)
			case catalog.FKCascade:
				if err := e.deleteRowsFromTable(childDef, matching); err != nil {
					return err
				}
			case catalog.FKSetNull:
				if err := e.updateColumnInRows(childDef, fk.ColumnName, catalog.Value{IsNull: true}, matching); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// applyOnUpdate enforces FK ON UPDATE rules for all child tables that reference
// parentTable when the parent row's PK changes from oldPKValue to newPKValue.
func (e *Executor) applyOnUpdate(parentTable string, oldPKValue, newPKValue catalog.Value, pkType catalog.DataType) error {
	for _, childDef := range e.catalog.ListTables() {
		for _, fk := range childDef.ForeignKeys {
			if fk.RefTable != parentTable {
				continue
			}
			matching, err := e.findRowsMatchingValue(childDef, fk.ColumnName, oldPKValue)
			if err != nil {
				return err
			}
			if len(matching) == 0 {
				continue
			}
			switch fk.OnUpdate {
			case catalog.FKRestrict:
				return fmt.Errorf("foreign key violation: cannot update %q: %d row(s) in %q reference this row",
					parentTable, len(matching), childDef.Name)
			case catalog.FKCascade:
				if err := e.updateColumnInRows(childDef, fk.ColumnName, newPKValue, matching); err != nil {
					return err
				}
			case catalog.FKSetNull:
				if err := e.updateColumnInRows(childDef, fk.ColumnName, catalog.Value{IsNull: true}, matching); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// deleteRowsFromTable deletes a pre-collected set of rows from tableDef, applying FK
// ON DELETE rules to any child tables first. Used by executeDelete and CASCADE paths.
func (e *Executor) deleteRowsFromTable(tableDef *catalog.TableDef, rows []Row) error {
	pkIndex := findColumnIndex(tableDef.Columns, func(col catalog.ColumnDef) bool {
		return col.PrimaryKey
	})
	if pkIndex < 0 {
		return fmt.Errorf("table %q has no primary key", tableDef.Name)
	}

	dataTree, err := storage.NewBTree(e.pool, tableDef.RootPageID)
	if err != nil {
		return err
	}

	indexes, err := e.catalog.IndexesForTable(tableDef.Name)
	if err != nil {
		return err
	}

	for _, row := range rows {
		if err := e.applyOnDelete(tableDef.Name, row[pkIndex]); err != nil {
			return err
		}

		pkBytes, err := encodeKeyValue(row[pkIndex], tableDef.Columns[pkIndex].Type)
		if err != nil {
			return fmt.Errorf("encoding primary key for delete: %w", err)
		}
		if err := dataTree.Delete(pkBytes); err != nil {
			return fmt.Errorf("deleting row from %q: %w", tableDef.Name, err)
		}

		for _, indexDef := range indexes {
			colIndex := findColumnIndex(tableDef.Columns, func(col catalog.ColumnDef) bool {
				return col.Name == indexDef.ColumnName
			})
			if colIndex < 0 {
				continue
			}
			indexKeyBytes, err := encodeKeyValue(row[colIndex], tableDef.Columns[colIndex].Type)
			if err != nil {
				return err
			}
			indexTree, err := storage.NewBTree(e.pool, indexDef.RootPageID)
			if err != nil {
				return err
			}
			if err := indexTree.Delete(indexKeyBytes); err != nil {
				return fmt.Errorf("deleting index entry in %q: %w", indexDef.Name, err)
			}
		}
	}
	return nil
}

// updateColumnInRows overwrites one column in each of the given rows and updates
// any index on that column. Used for FK CASCADE and SET NULL on UPDATE/DELETE.
func (e *Executor) updateColumnInRows(tableDef *catalog.TableDef, columnName string, newValue catalog.Value, rows []Row) error {
	colIndex := findColumnIndex(tableDef.Columns, func(col catalog.ColumnDef) bool {
		return col.Name == columnName
	})
	if colIndex < 0 {
		return fmt.Errorf("column %q not found in table %q", columnName, tableDef.Name)
	}
	if newValue.IsNull && tableDef.Columns[colIndex].NotNull {
		return fmt.Errorf("cannot SET NULL: column %q.%q is NOT NULL", tableDef.Name, columnName)
	}

	pkIndex := findColumnIndex(tableDef.Columns, func(col catalog.ColumnDef) bool {
		return col.PrimaryKey
	})
	if pkIndex < 0 {
		return fmt.Errorf("table %q has no primary key", tableDef.Name)
	}

	dataTree, err := storage.NewBTree(e.pool, tableDef.RootPageID)
	if err != nil {
		return err
	}

	indexes, err := e.catalog.IndexesForTable(tableDef.Name)
	if err != nil {
		return err
	}

	for _, oldRow := range rows {
		newRow := append(Row{}, oldRow...)
		newRow[colIndex] = newValue

		pkBytes, err := encodeKeyValue(oldRow[pkIndex], tableDef.Columns[pkIndex].Type)
		if err != nil {
			return err
		}
		if err := dataTree.Delete(pkBytes); err != nil {
			return err
		}
		newRowBytes := encodeRow(newRow, tableDef.Columns)
		if err := dataTree.Insert(pkBytes, newRowBytes); err != nil {
			return err
		}

		for _, indexDef := range indexes {
			if indexDef.ColumnName != columnName {
				continue
			}
			oldIndexKeyBytes, err := encodeKeyValue(oldRow[colIndex], tableDef.Columns[colIndex].Type)
			if err != nil {
				return err
			}
			indexTree, err := storage.NewBTree(e.pool, indexDef.RootPageID)
			if err != nil {
				return err
			}
			if err := indexTree.Delete(oldIndexKeyBytes); err != nil {
				return err
			}
			if !newValue.IsNull {
				newIndexKeyBytes, err := encodeKeyValue(newValue, tableDef.Columns[colIndex].Type)
				if err != nil {
					return err
				}
				if err := indexTree.Insert(newIndexKeyBytes, pkBytes); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// findRowsMatchingValue scans tableDef and returns all rows where columnName = value.
func (e *Executor) findRowsMatchingValue(tableDef *catalog.TableDef, columnName string, value catalog.Value) ([]Row, error) {
	colIndex := findColumnIndex(tableDef.Columns, func(col catalog.ColumnDef) bool {
		return col.Name == columnName
	})
	if colIndex < 0 {
		return nil, fmt.Errorf("column %q not found in table %q", columnName, tableDef.Name)
	}
	colType := tableDef.Columns[colIndex].Type

	dataTree, err := storage.NewBTree(e.pool, tableDef.RootPageID)
	if err != nil {
		return nil, err
	}

	scan := NewTableScan(dataTree, tableDef.Columns)
	if err := scan.Open(); err != nil {
		return nil, err
	}
	defer scan.Close()

	var matching []Row
	for {
		row, err := scan.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if valuesEqual(row[colIndex], value, colType) {
			matching = append(matching, row)
		}
	}
	return matching, nil
}

// columnContainsValue reports whether tableDef has any row where columnName = value.
func (e *Executor) columnContainsValue(tableDef *catalog.TableDef, columnName string, value catalog.Value) (bool, error) {
	rows, err := e.findRowsMatchingValue(tableDef, columnName, value)
	if err != nil {
		return false, err
	}
	return len(rows) > 0, nil
}

// valuesEqual compares two catalog.Values of the same DataType for equality.
// Two NULLs are not equal (SQL semantics: NULL != NULL).
func valuesEqual(a, b catalog.Value, dataType catalog.DataType) bool {
	if a.IsNull || b.IsNull {
		return false
	}
	switch dataType {
	case catalog.TypeInt:
		return a.IntVal == b.IntVal
	case catalog.TypeFloat:
		return a.FloatVal == b.FloatVal
	case catalog.TypeText:
		return a.TextVal == b.TextVal
	case catalog.TypeBlob:
		return string(a.BlobVal) == string(b.BlobVal)
	}
	return false
}
