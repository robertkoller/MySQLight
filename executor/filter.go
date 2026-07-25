package executor

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/robertkoller/MySQLight/catalog"
	"github.com/robertkoller/MySQLight/parser"
)

// Filter wraps a child operator and skips rows where the predicate evaluates to false.
type Filter struct {
	input     Operator
	predicate parser.Expr
	columns   []catalog.ColumnDef
}

func NewFilter(input Operator, predicate parser.Expr, columns []catalog.ColumnDef) *Filter {
	return &Filter{input: input, predicate: predicate, columns: columns}
}

func (f *Filter) Open() error {
	return f.input.Open()
}

func (f *Filter) Next() (Row, error) {
	for {
		row, err := f.input.Next()
		if err != nil {
			return nil, err
		}
		result, err := evalExpr(f.predicate, row, f.columns)
		if err != nil {
			return nil, err
		}
		if matches, ok := result.(bool); ok && matches {
			return row, nil
		}
	}
}

func (f *Filter) Close() error {
	return f.input.Close()
}

// evalExpr evaluates an AST expression against one row. Return types match the
// value: bool for comparisons and logical ops, int64/float64 for arithmetic,
// string for text, []byte for blob, nil for NULL. FuncCall is invalid here.
func evalExpr(expr parser.Expr, row Row, columns []catalog.ColumnDef) (interface{}, error) {
	switch node := expr.(type) {
	case *parser.Literal:
		switch node.Kind {
		case "integer":
			value, err := strconv.ParseInt(node.Value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid integer literal %q: %w", node.Value, err)
			}
			return value, nil
		case "float":
			value, err := strconv.ParseFloat(node.Value, 64)
			if err != nil {
				return nil, fmt.Errorf("invalid float literal %q: %w", node.Value, err)
			}
			return value, nil
		case "string":
			return node.Value, nil
		case "null":
			return nil, nil
		default:
			return nil, fmt.Errorf("unknown literal kind %q", node.Kind)
		}

	case *parser.ColumnRef:
		for index, col := range columns {
			if col.Name == node.Column {
				if index >= len(row) {
					return nil, fmt.Errorf("column index %d out of range for row length %d", index, len(row))
				}
				if row[index].IsNull {
					return nil, nil
				}
				switch col.Type {
				case catalog.TypeInt:
					return row[index].IntVal, nil
				case catalog.TypeFloat:
					return row[index].FloatVal, nil
				case catalog.TypeText:
					return row[index].TextVal, nil
				case catalog.TypeBlob:
					return row[index].BlobVal, nil
				}
			}
		}
		return nil, fmt.Errorf("column %q not found in schema", node.Column)

	case *parser.BinaryExpr:
		left, err := evalExpr(node.Left, row, columns)
		if err != nil {
			return nil, err
		}
		right, err := evalExpr(node.Right, row, columns)
		if err != nil {
			return nil, err
		}
		return applyBinaryOp(node.Op, left, right)

	case *parser.UnaryExpr:
		operand, err := evalExpr(node.Operand, row, columns)
		if err != nil {
			return nil, err
		}
		switch node.Op {
		case "NOT":
			boolVal, ok := operand.(bool)
			if !ok {
				return nil, fmt.Errorf("NOT requires a boolean operand")
			}
			return !boolVal, nil
		case "-":
			switch typed := operand.(type) {
			case int64:
				return -typed, nil
			case float64:
				return -typed, nil
			default:
				return nil, fmt.Errorf("unary minus requires a numeric operand")
			}
		default:
			return nil, fmt.Errorf("unknown unary operator %q", node.Op)
		}

	case *parser.FuncCall:
		return nil, fmt.Errorf("aggregate %q is not valid outside an Aggregate operator", node.Name)

	case *parser.StarExpr:
		return nil, fmt.Errorf("star expression is not valid in this context")

	default:
		return nil, fmt.Errorf("unknown expression type %T", expr)
	}
}

func applyBinaryOp(op string, left, right interface{}) (interface{}, error) {
	// Any binary op with NULL yields NULL (SQL three-valued logic).
	if left == nil || right == nil {
		return nil, nil
	}

	switch op {
	case "AND":
		leftBool, ok1 := left.(bool)
		rightBool, ok2 := right.(bool)
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("AND requires boolean operands")
		}
		return leftBool && rightBool, nil

	case "OR":
		leftBool, ok1 := left.(bool)
		rightBool, ok2 := right.(bool)
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("OR requires boolean operands")
		}
		return leftBool || rightBool, nil

	case "LIKE":
		leftStr, ok1 := left.(string)
		rightStr, ok2 := right.(string)
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("LIKE requires string operands")
		}
		return likeMatch(leftStr, rightStr), nil
	}

	leftFloat, leftIsNum := toFloat64(left)
	rightFloat, rightIsNum := toFloat64(right)

	switch op {
	case "=":
		if leftIsNum && rightIsNum {
			return leftFloat == rightFloat, nil
		}
		return fmt.Sprintf("%v", left) == fmt.Sprintf("%v", right), nil
	case "!=":
		if leftIsNum && rightIsNum {
			return leftFloat != rightFloat, nil
		}
		return fmt.Sprintf("%v", left) != fmt.Sprintf("%v", right), nil
	case "<":
		if leftIsNum && rightIsNum {
			return leftFloat < rightFloat, nil
		}
		return fmt.Sprintf("%v", left) < fmt.Sprintf("%v", right), nil
	case ">":
		if leftIsNum && rightIsNum {
			return leftFloat > rightFloat, nil
		}
		return fmt.Sprintf("%v", left) > fmt.Sprintf("%v", right), nil
	case "<=":
		if leftIsNum && rightIsNum {
			return leftFloat <= rightFloat, nil
		}
		return fmt.Sprintf("%v", left) <= fmt.Sprintf("%v", right), nil
	case ">=":
		if leftIsNum && rightIsNum {
			return leftFloat >= rightFloat, nil
		}
		return fmt.Sprintf("%v", left) >= fmt.Sprintf("%v", right), nil
	case "+", "-", "*", "/":
		if op == "/" && rightFloat == 0 {
			return nil, fmt.Errorf("division by zero")
		}
		return applyArith(op, left, right, leftFloat, rightFloat, leftIsNum, rightIsNum)
	case "%":
		leftInt, ok1 := left.(int64)
		rightInt, ok2 := right.(int64)
		if !ok1 || !ok2 {
			return nil, fmt.Errorf("%% requires integer operands")
		}
		if rightInt == 0 {
			return nil, fmt.Errorf("modulo by zero")
		}
		return leftInt % rightInt, nil
	}

	return nil, fmt.Errorf("unknown binary operator %q", op)
}

func applyArith(op string, left, right interface{}, leftFloat, rightFloat float64, leftIsNum, rightIsNum bool) (interface{}, error) {
	if !leftIsNum || !rightIsNum {
		return nil, fmt.Errorf("operator %q requires numeric operands", op)
	}
	// Preserve int64 when both sides are integers.
	leftInt, leftIsInt := left.(int64)
	rightInt, rightIsInt := right.(int64)
	if leftIsInt && rightIsInt {
		switch op {
		case "+":
			return leftInt + rightInt, nil
		case "-":
			return leftInt - rightInt, nil
		case "*":
			return leftInt * rightInt, nil
		case "/":
			return leftInt / rightInt, nil
		}
	}
	switch op {
	case "+":
		return leftFloat + rightFloat, nil
	case "-":
		return leftFloat - rightFloat, nil
	case "*":
		return leftFloat * rightFloat, nil
	case "/":
		return leftFloat / rightFloat, nil
	}
	return nil, fmt.Errorf("unknown arithmetic operator %q", op)
}

// toFloat64 coerces an int64 or float64 to float64 for comparison. Returns false if
// the value is neither.
func toFloat64(value interface{}) (float64, bool) {
	switch typed := value.(type) {
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	}
	return 0, false
}

// likeMatch matches str against a SQL LIKE pattern where % matches any sequence of characters.
func likeMatch(str, pattern string) bool {
	parts := strings.Split(pattern, "%")
	if len(parts) == 1 {
		return str == pattern
	}
	if !strings.HasPrefix(str, parts[0]) {
		return false
	}
	remaining := str[len(parts[0]):]
	for _, part := range parts[1 : len(parts)-1] {
		index := strings.Index(remaining, part)
		if index < 0 {
			return false
		}
		remaining = remaining[index+len(part):]
	}
	return strings.HasSuffix(remaining, parts[len(parts)-1])
}
