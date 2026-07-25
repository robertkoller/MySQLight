package executor

import (
	"fmt"

	"github.com/robertkoller/MySQLight/catalog"
	"github.com/robertkoller/MySQLight/parser"
	"github.com/robertkoller/MySQLight/storage"
)

func (e *Executor) executeInsert(statement *parser.InsertStmt) error {
	definition, err := e.catalog.GetTable(statement.Table)
	if err != nil {
		return fmt.Errorf("table %q not found: %w", statement.Table, err)
	}

	dataTree, err := storage.NewBTree(e.pool, definition.RootPageID)
	if err != nil {
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

	for _, valueExprs := range statement.Values {
		row, err := buildInsertRow(valueExprs, statement.Columns, definition)
		if err != nil {
			return err
		}

		for index, col := range definition.Columns {
			if col.NotNull && row[index].IsNull {
				return fmt.Errorf("column %q violates NOT NULL constraint", col.Name)
			}
		}

		if err := e.verifyFKParents(row, definition); err != nil {
			return err
		}

		primaryKeyBytes, err := encodeKeyValue(row[pkIndex], definition.Columns[pkIndex].Type)
		if err != nil {
			return fmt.Errorf("encoding primary key: %w", err)
		}

		rowBytes := encodeRow(row, definition.Columns)

		if err := dataTree.Insert(primaryKeyBytes, rowBytes); err != nil {
			return fmt.Errorf("inserting row: %w", err)
		}

		for _, indexDef := range indexes {
			colIndex := findColumnIndex(definition.Columns, func(col catalog.ColumnDef) bool {
				return col.Name == indexDef.ColumnName
			})
			if colIndex < 0 {
				continue
			}
			indexKeyBytes, err := encodeKeyValue(row[colIndex], definition.Columns[colIndex].Type)
			if err != nil {
				return fmt.Errorf("encoding index key for %q: %w", indexDef.Name, err)
			}
			indexTree, err := storage.NewBTree(e.pool, indexDef.RootPageID)
			if err != nil {
				return err
			}
			if err := indexTree.Insert(indexKeyBytes, primaryKeyBytes); err != nil {
				return fmt.Errorf("inserting into index %q: %w", indexDef.Name, err)
			}
		}
	}

	return nil
}

// buildInsertRow maps the VALUES expression list onto a full row parallel to def.Columns.
// Positional when columnNames is empty; named otherwise. Unlisted columns receive their
// schema default or NULL.
func buildInsertRow(valueExprs []parser.Expr, columnNames []string, def *catalog.TableDef) ([]catalog.Value, error) {
	row := make([]catalog.Value, len(def.Columns))
	for index := range row {
		if def.Columns[index].Default != nil {
			row[index] = *def.Columns[index].Default
		} else {
			row[index] = catalog.Value{IsNull: true}
		}
	}

	if len(columnNames) == 0 {
		if len(valueExprs) != len(def.Columns) {
			return nil, fmt.Errorf("expected %d values, got %d", len(def.Columns), len(valueExprs))
		}
		for index, expr := range valueExprs {
			value, err := evalExprToValue(expr, def.Columns[index].Type)
			if err != nil {
				return nil, err
			}
			row[index] = value
		}
	} else {
		if len(columnNames) != len(valueExprs) {
			return nil, fmt.Errorf("column count %d doesn't match value count %d", len(columnNames), len(valueExprs))
		}
		for index, name := range columnNames {
			colIndex := findColumnIndex(def.Columns, func(col catalog.ColumnDef) bool {
				return col.Name == name
			})
			if colIndex < 0 {
				return nil, fmt.Errorf("column %q not found in table", name)
			}
			value, err := evalExprToValue(valueExprs[index], def.Columns[colIndex].Type)
			if err != nil {
				return nil, err
			}
			row[colIndex] = value
		}
	}

	return row, nil
}

// evalExprToValue evaluates a constant expression (literals, arithmetic — no column refs)
// and coerces the result to the target DataType.
func evalExprToValue(expr parser.Expr, dataType catalog.DataType) (catalog.Value, error) {
	result, err := evalExpr(expr, Row{}, []catalog.ColumnDef{})
	if err != nil {
		return catalog.Value{}, err
	}
	if result == nil {
		return catalog.Value{IsNull: true}, nil
	}
	switch dataType {
	case catalog.TypeInt:
		switch typed := result.(type) {
		case int64:
			return catalog.Value{IntVal: typed}, nil
		case float64:
			return catalog.Value{IntVal: int64(typed)}, nil
		}
	case catalog.TypeFloat:
		switch typed := result.(type) {
		case float64:
			return catalog.Value{FloatVal: typed}, nil
		case int64:
			return catalog.Value{FloatVal: float64(typed)}, nil
		}
	case catalog.TypeText:
		if text, ok := result.(string); ok {
			return catalog.Value{TextVal: text}, nil
		}
	case catalog.TypeBlob:
		if blob, ok := result.([]byte); ok {
			return catalog.Value{BlobVal: blob}, nil
		}
	}
	return catalog.Value{}, fmt.Errorf("cannot store %T value in %v column", result, dataType)
}
