package executor

import (
	"fmt"
	"io"

	"github.com/robertkoller/MySQLight/catalog"
	"github.com/robertkoller/MySQLight/parser"
)

// Aggregate consumes all child rows, groups them by the GROUP BY key, and
// accumulates per-group aggregate state. Results are materialised in Open so
// that Next can return them one at a time.
type Aggregate struct {
	input    Operator
	groupBy  []parser.Expr
	aggExprs []parser.Expr // the full SELECT list (mix of aggregates and group-by cols)
	columns  []catalog.ColumnDef
	results  []Row
	cursor   int
}

// groupData holds per-group accumulation state. Each SELECT expression gets its
// own aggState slot so SELECT COUNT(*), SUM(salary) don't share counters.
type groupData struct {
	states   []aggState
	firstRow Row // used to evaluate non-aggregate SELECT expressions
}

type aggState struct {
	count    int64
	sum      float64
	minVal   interface{}
	maxVal   interface{}
	hasValue bool
}

func NewAggregate(input Operator, groupBy []parser.Expr, aggExprs []parser.Expr, columns []catalog.ColumnDef) *Aggregate {
	return &Aggregate{
		input:    input,
		groupBy:  groupBy,
		aggExprs: aggExprs,
		columns:  columns,
	}
}

func (a *Aggregate) Open() error {
	if err := a.input.Open(); err != nil {
		return err
	}

	var keyOrder []string
	groups := make(map[string]*groupData)

	for {
		row, err := a.input.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		key, err := a.groupKey(row)
		if err != nil {
			return err
		}

		group, exists := groups[key]
		if !exists {
			group = &groupData{
				states:   make([]aggState, len(a.aggExprs)),
				firstRow: row,
			}
			groups[key] = group
			keyOrder = append(keyOrder, key)
		}

		for index, expr := range a.aggExprs {
			call, ok := expr.(*parser.FuncCall)
			if !ok {
				continue
			}
			if err := updateAggState(&group.states[index], call, row, a.columns); err != nil {
				return err
			}
		}
	}

	// Materialise one output row per group in insertion order.
	for _, key := range keyOrder {
		group := groups[key]
		outputRow := make(Row, len(a.aggExprs))
		for index, expr := range a.aggExprs {
			call, ok := expr.(*parser.FuncCall)
			if !ok {
				// Non-aggregate expression: evaluate against the group's first row.
				val, err := evalExpr(expr, group.firstRow, a.columns)
				if err != nil {
					return err
				}
				outputRow[index] = toValue(val)
				continue
			}
			outputRow[index] = finalizeAgg(call.Name, &group.states[index])
		}
		a.results = append(a.results, outputRow)
	}

	return nil
}

func (a *Aggregate) Next() (Row, error) {
	if a.cursor >= len(a.results) {
		return nil, io.EOF
	}
	row := a.results[a.cursor]
	a.cursor++
	return row, nil
}

func (a *Aggregate) Close() error {
	return a.input.Close()
}

// groupKey serialises GROUP BY expression values into a map key. With no GROUP BY,
// all rows belong to the single "__all__" group.
func (a *Aggregate) groupKey(row Row) (string, error) {
	if len(a.groupBy) == 0 {
		return "__all__", nil
	}
	key := ""
	for _, expr := range a.groupBy {
		val, err := evalExpr(expr, row, a.columns)
		if err != nil {
			return "", err
		}
		key += fmt.Sprintf("%v\x00", val)
	}
	return key, nil
}

// updateAggState feeds one row into a single aggregate accumulator slot.
func updateAggState(state *aggState, call *parser.FuncCall, row Row, columns []catalog.ColumnDef) error {
	// COUNT(*) counts all rows regardless of nulls.
	if call.Star {
		state.count++
		return nil
	}

	var value interface{}
	if len(call.Args) > 0 {
		var err error
		value, err = evalExpr(call.Args[0], row, columns)
		if err != nil {
			return err
		}
	}

	switch call.Name {
	case "COUNT":
		if value != nil {
			state.count++
		}
	case "SUM":
		if value != nil {
			if number, ok := toFloat64(value); ok {
				state.sum += number
				state.count++
			}
		}
	case "AVG":
		if value != nil {
			if number, ok := toFloat64(value); ok {
				state.sum += number
				state.count++
			}
		}
	case "MIN":
		if value != nil {
			if !state.hasValue || compareValues(value, state.minVal) < 0 {
				state.minVal = value
				state.hasValue = true
			}
		}
	case "MAX":
		if value != nil {
			if !state.hasValue || compareValues(value, state.maxVal) > 0 {
				state.maxVal = value
				state.hasValue = true
			}
		}
	}
	return nil
}

// finalizeAgg converts accumulated state into the output catalog.Value.
func finalizeAgg(name string, state *aggState) catalog.Value {
	switch name {
	case "COUNT":
		return catalog.Value{IntVal: state.count}
	case "SUM":
		if state.count == 0 {
			return catalog.Value{IsNull: true}
		}
		return catalog.Value{FloatVal: state.sum}
	case "AVG":
		if state.count == 0 {
			return catalog.Value{IsNull: true}
		}
		return catalog.Value{FloatVal: state.sum / float64(state.count)}
	case "MIN":
		if !state.hasValue {
			return catalog.Value{IsNull: true}
		}
		return toValue(state.minVal)
	case "MAX":
		if !state.hasValue {
			return catalog.Value{IsNull: true}
		}
		return toValue(state.maxVal)
	}
	return catalog.Value{IsNull: true}
}
