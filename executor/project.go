package executor

import (
	"fmt"
	"io"
	"sort"

	"github.com/robertkoller/MySQLight/catalog"
	"github.com/robertkoller/MySQLight/parser"
)

// Project wraps a child operator and evaluates the SELECT expression list, emitting
// only the columns produced by those expressions.
type Project struct {
	input   Operator
	exprs   []parser.Expr
	columns []catalog.ColumnDef
}

func NewProject(input Operator, exprs []parser.Expr, columns []catalog.ColumnDef) *Project {
	return &Project{input: input, exprs: exprs, columns: columns}
}

func (p *Project) Open() error {
	return p.input.Open()
}

func (p *Project) Next() (Row, error) {
	row, err := p.input.Next()
	if err != nil {
		return nil, err
	}
	output := make(Row, len(p.exprs))
	for index, expr := range p.exprs {
		value, err := evalExpr(expr, row, p.columns)
		if err != nil {
			return nil, err
		}
		output[index] = toValue(value)
	}
	return output, nil
}

func (p *Project) Close() error {
	return p.input.Close()
}

// toValue converts the interface{} result of evalExpr back into a catalog.Value.
func toValue(value interface{}) catalog.Value {
	if value == nil {
		return catalog.Value{IsNull: true}
	}
	switch typed := value.(type) {
	case int64:
		return catalog.Value{IntVal: typed}
	case float64:
		return catalog.Value{FloatVal: typed}
	case string:
		return catalog.Value{TextVal: typed}
	case []byte:
		return catalog.Value{BlobVal: typed}
	}
	return catalog.Value{IsNull: true}
}

// Sort buffers all child rows during Open and re-emits them in ORDER BY key order.
type Sort struct {
	input   Operator
	orderBy []parser.OrderClause
	columns []catalog.ColumnDef
	rows    []Row
	cursor  int
}

func NewSort(input Operator, orderBy []parser.OrderClause, columns []catalog.ColumnDef) *Sort {
	return &Sort{input: input, orderBy: orderBy, columns: columns}
}

func (s *Sort) Open() error {
	if err := s.input.Open(); err != nil {
		return err
	}
	for {
		row, err := s.input.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			s.input.Close()
			return err
		}
		s.rows = append(s.rows, row)
	}
	sort.SliceStable(s.rows, func(i, j int) bool {
		for _, clause := range s.orderBy {
			left, _ := evalExpr(clause.Expr, s.rows[i], s.columns)
			right, _ := evalExpr(clause.Expr, s.rows[j], s.columns)
			comparison := compareValues(left, right)
			if comparison == 0 {
				continue
			}
			if clause.Desc {
				return comparison > 0
			}
			return comparison < 0
		}
		return false
	})
	return nil
}

func (s *Sort) Next() (Row, error) {
	if s.cursor >= len(s.rows) {
		return nil, io.EOF
	}
	row := s.rows[s.cursor]
	s.cursor++
	return row, nil
}

func (s *Sort) Close() error {
	return s.input.Close()
}

// compareValues returns -1, 0, or 1 for two interface{} values from evalExpr.
// NULL sorts before all non-NULL values.
func compareValues(left, right interface{}) int {
	if left == nil && right == nil {
		return 0
	}
	if left == nil {
		return -1
	}
	if right == nil {
		return 1
	}
	leftFloat, leftIsNum := toFloat64(left)
	rightFloat, rightIsNum := toFloat64(right)
	if leftIsNum && rightIsNum {
		if leftFloat < rightFloat {
			return -1
		}
		if leftFloat > rightFloat {
			return 1
		}
		return 0
	}
	leftStr := fmt.Sprintf("%v", left)
	rightStr := fmt.Sprintf("%v", right)
	if leftStr < rightStr {
		return -1
	}
	if leftStr > rightStr {
		return 1
	}
	return 0
}

// Limit passes through the first n rows then returns io.EOF.
type Limit struct {
	input Operator
	limit int
	count int
}

func NewLimit(input Operator, limit int) *Limit {
	return &Limit{input: input, limit: limit}
}

func (l *Limit) Open() error {
	return l.input.Open()
}

func (l *Limit) Next() (Row, error) {
	if l.count >= l.limit {
		return nil, io.EOF
	}
	row, err := l.input.Next()
	if err != nil {
		return nil, err
	}
	l.count++
	return row, nil
}

func (l *Limit) Close() error {
	return l.input.Close()
}
