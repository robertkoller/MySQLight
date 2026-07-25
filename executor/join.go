package executor

import (
	"io"

	"github.com/robertkoller/MySQLight/catalog"
	"github.com/robertkoller/MySQLight/parser"
)

// NestedLoopJoin is the simplest join: for every left row it scans all right rows,
// returning combined rows where the condition holds. O(n*m).
type NestedLoopJoin struct {
	left         Operator
	right        Operator
	condition    parser.Expr
	leftColumns  []catalog.ColumnDef
	rightColumns []catalog.ColumnDef
	allColumns   []catalog.ColumnDef
	leftRow      Row
	done         bool
}

func NewNestedLoopJoin(left, right Operator, condition parser.Expr, leftColumns, rightColumns []catalog.ColumnDef) *NestedLoopJoin {
	allColumns := make([]catalog.ColumnDef, 0, len(leftColumns)+len(rightColumns))
	allColumns = append(allColumns, leftColumns...)
	allColumns = append(allColumns, rightColumns...)
	return &NestedLoopJoin{
		left:         left,
		right:        right,
		condition:    condition,
		leftColumns:  leftColumns,
		rightColumns: rightColumns,
		allColumns:   allColumns,
	}
}

func (j *NestedLoopJoin) Open() error {
	if err := j.left.Open(); err != nil {
		return err
	}
	if err := j.right.Open(); err != nil {
		j.left.Close()
		return err
	}

	row, err := j.left.Next()
	if err == io.EOF {
		j.done = true
		return nil
	}
	if err != nil {
		return err
	}
	j.leftRow = row
	return nil
}

func (j *NestedLoopJoin) Next() (Row, error) {
	for !j.done {
		rightRow, err := j.right.Next()
		if err == io.EOF {
			nextLeft, err := j.left.Next()
			if err == io.EOF {
				j.done = true
				return nil, io.EOF
			}
			if err != nil {
				return nil, err
			}
			j.leftRow = nextLeft
			if err := j.right.Close(); err != nil {
				return nil, err
			}
			if err := j.right.Open(); err != nil {
				return nil, err
			}
			continue
		}
		if err != nil {
			return nil, err
		}

		combined := make(Row, 0, len(j.leftRow)+len(rightRow))
		combined = append(combined, j.leftRow...)
		combined = append(combined, rightRow...)

		if j.condition == nil {
			return combined, nil
		}
		result, err := evalExpr(j.condition, combined, j.allColumns)
		if err != nil {
			return nil, err
		}
		if matches, ok := result.(bool); ok && matches {
			return combined, nil
		}
	}
	return nil, io.EOF
}

func (j *NestedLoopJoin) Close() error {
	leftErr := j.left.Close()
	rightErr := j.right.Close()
	if leftErr != nil {
		return leftErr
	}
	return rightErr
}
