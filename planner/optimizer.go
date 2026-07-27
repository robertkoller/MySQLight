package planner

import (
	"strconv"

	"github.com/robertkoller/MySQLight/catalog"
	"github.com/robertkoller/MySQLight/parser"
)

// Optimizer applies rule-based rewrites to a logical plan tree, then selects
// physical operators (e.g. choosing IndexScan over TableScan when profitable).
type Optimizer struct {
	catalog *catalog.Catalog
}

// NewOptimizer initialises the optimizer with a reference to the catalog, which is needed
// for index metadata during the index selection pass.
func NewOptimizer(cat *catalog.Catalog) *Optimizer {
	return &Optimizer{catalog: cat}
}

// Optimize applies four rule-based passes in order:
//  1. predicatePushdown — move filters closer to scans
//  2. constantFolding   — evaluate constant expressions at plan time
//  3. columnPruning     — drop columns not referenced downstream
//  4. indexSelection    — replace TableScan+Filter with IndexScan where an index exists
func (o *Optimizer) Optimize(root LogicalNode) (LogicalNode, error) {
	root = predicatePushdown(root)
	root = constantFolding(root)
	root = columnPruning(root)
	root = o.indexSelection(root)
	return root, nil
}

// predicatePushdown moves LogicalFilter nodes as close to their LogicalScan as possible.
// When a filter sits directly above a join and its predicate references only columns from
// one side of the join, the filter is pushed below the join onto that side.
func predicatePushdown(node LogicalNode) LogicalNode {
	switch typed := node.(type) {
	case *LogicalFilter:
		typed.Child = predicatePushdown(typed.Child)
		join, ok := typed.Child.(*LogicalJoin)
		if !ok {
			return typed
		}
		predicateTableRefs := collectColumnRefTables(typed.Predicate)
		if len(predicateTableRefs) == 0 {
			// Unqualified column refs — can't determine which side without schema resolution.
			return typed
		}
		leftTables := tablesInSubtree(join.Left)
		rightTables := tablesInSubtree(join.Right)

		allFromLeft := true
		for table := range predicateTableRefs {
			if !leftTables[table] {
				allFromLeft = false
				break
			}
		}
		allFromRight := true
		for table := range predicateTableRefs {
			if !rightTables[table] {
				allFromRight = false
				break
			}
		}

		if allFromLeft {
			join.Left = &LogicalFilter{Child: join.Left, Predicate: typed.Predicate}
			return join
		}
		if allFromRight {
			join.Right = &LogicalFilter{Child: join.Right, Predicate: typed.Predicate}
			return join
		}
		return typed
	case *LogicalJoin:
		typed.Left = predicatePushdown(typed.Left)
		typed.Right = predicatePushdown(typed.Right)
		return typed
	case *LogicalProject:
		typed.Child = predicatePushdown(typed.Child)
		return typed
	case *LogicalAggregate:
		typed.Child = predicatePushdown(typed.Child)
		return typed
	case *LogicalSort:
		typed.Child = predicatePushdown(typed.Child)
		return typed
	case *LogicalLimit:
		typed.Child = predicatePushdown(typed.Child)
		return typed
	default:
		return node
	}
}

// tablesInSubtree returns the set of table names reachable from the given subtree root.
func tablesInSubtree(node LogicalNode) map[string]bool {
	result := make(map[string]bool)
	switch typed := node.(type) {
	case *LogicalScan:
		result[typed.Table] = true
	case *LogicalIndexScan:
		result[typed.Table] = true
	default:
		for _, child := range node.Children() {
			for table := range tablesInSubtree(child) {
				result[table] = true
			}
		}
	}
	return result
}

// collectColumnRefTables returns the set of table names (from qualified ColumnRef nodes)
// present in the expression tree. Unqualified refs produce no entry.
func collectColumnRefTables(expr parser.Expr) map[string]bool {
	result := make(map[string]bool)
	collectColumnRefTablesInto(expr, result)
	return result
}

func collectColumnRefTablesInto(expr parser.Expr, result map[string]bool) {
	switch typed := expr.(type) {
	case *parser.ColumnRef:
		if typed.Table != "" {
			result[typed.Table] = true
		}
	case *parser.BinaryExpr:
		collectColumnRefTablesInto(typed.Left, result)
		collectColumnRefTablesInto(typed.Right, result)
	case *parser.UnaryExpr:
		collectColumnRefTablesInto(typed.Operand, result)
	case *parser.FuncCall:
		for _, arg := range typed.Args {
			collectColumnRefTablesInto(arg, result)
		}
	}
}

// constantFolding evaluates constant subexpressions at plan time.
// If both operands of a BinaryExpr are Literal nodes the expression is replaced
// with a single Literal result (e.g. "2 + 3" → Literal{"5"}).
func constantFolding(node LogicalNode) LogicalNode {
	switch typed := node.(type) {
	case *LogicalFilter:
		typed.Child = constantFolding(typed.Child)
		typed.Predicate = foldExpr(typed.Predicate)
		return typed
	case *LogicalProject:
		typed.Child = constantFolding(typed.Child)
		for index, col := range typed.Columns {
			typed.Columns[index] = foldExpr(col)
		}
		return typed
	case *LogicalJoin:
		typed.Left = constantFolding(typed.Left)
		typed.Right = constantFolding(typed.Right)
		if typed.Condition != nil {
			typed.Condition = foldExpr(typed.Condition)
		}
		return typed
	case *LogicalAggregate:
		typed.Child = constantFolding(typed.Child)
		for index, expr := range typed.GroupBy {
			typed.GroupBy[index] = foldExpr(expr)
		}
		return typed
	case *LogicalSort:
		typed.Child = constantFolding(typed.Child)
		for index, orderClause := range typed.OrderBy {
			typed.OrderBy[index].Expr = foldExpr(orderClause.Expr)
		}
		return typed
	case *LogicalLimit:
		typed.Child = constantFolding(typed.Child)
		return typed
	default:
		return node
	}
}

// foldExpr recursively evaluates constant subexpressions within a single Expr tree.
func foldExpr(expr parser.Expr) parser.Expr {
	binary, ok := expr.(*parser.BinaryExpr)
	if !ok {
		return expr
	}
	binary.Left = foldExpr(binary.Left)
	binary.Right = foldExpr(binary.Right)

	leftLiteral, leftOk := binary.Left.(*parser.Literal)
	rightLiteral, rightOk := binary.Right.(*parser.Literal)
	if !leftOk || !rightOk {
		return binary
	}
	return evalConstantBinary(leftLiteral, binary.Op, rightLiteral)
}

func evalConstantBinary(left *parser.Literal, op string, right *parser.Literal) parser.Expr {
	if left.Kind == "null" || right.Kind == "null" {
		return &parser.Literal{Kind: "null", Value: "null"}
	}

	if op == "AND" || op == "OR" {
		if left.Kind == "integer" && right.Kind == "integer" {
			leftBool := left.Value != "0"
			rightBool := right.Value != "0"
			if op == "AND" {
				return boolLiteral(leftBool && rightBool)
			}
			return boolLiteral(leftBool || rightBool)
		}
		return &parser.BinaryExpr{Left: left, Op: op, Right: right}
	}

	if (left.Kind == "integer" || left.Kind == "float") && (right.Kind == "integer" || right.Kind == "float") {
		leftVal, err1 := strconv.ParseFloat(left.Value, 64)
		rightVal, err2 := strconv.ParseFloat(right.Value, 64)
		if err1 != nil || err2 != nil {
			return &parser.BinaryExpr{Left: left, Op: op, Right: right}
		}
		bothInt := left.Kind == "integer" && right.Kind == "integer"
		switch op {
		case "+":
			return numericLiteral(leftVal+rightVal, bothInt)
		case "-":
			return numericLiteral(leftVal-rightVal, bothInt)
		case "*":
			return numericLiteral(leftVal*rightVal, bothInt)
		case "/":
			if rightVal == 0 {
				return &parser.BinaryExpr{Left: left, Op: op, Right: right}
			}
			return numericLiteral(leftVal/rightVal, bothInt)
		case "%":
			if rightVal == 0 {
				return &parser.BinaryExpr{Left: left, Op: op, Right: right}
			}
			return numericLiteral(float64(int64(leftVal)%int64(rightVal)), bothInt)
		case "=":
			return boolLiteral(leftVal == rightVal)
		case "!=":
			return boolLiteral(leftVal != rightVal)
		case "<":
			return boolLiteral(leftVal < rightVal)
		case ">":
			return boolLiteral(leftVal > rightVal)
		case "<=":
			return boolLiteral(leftVal <= rightVal)
		case ">=":
			return boolLiteral(leftVal >= rightVal)
		}
	}

	if left.Kind == "string" && right.Kind == "string" {
		switch op {
		case "=":
			return boolLiteral(left.Value == right.Value)
		case "!=":
			return boolLiteral(left.Value != right.Value)
		}
	}

	return &parser.BinaryExpr{Left: left, Op: op, Right: right}
}

func boolLiteral(value bool) *parser.Literal {
	if value {
		return &parser.Literal{Kind: "integer", Value: "1"}
	}
	return &parser.Literal{Kind: "integer", Value: "0"}
}

func numericLiteral(value float64, isInteger bool) *parser.Literal {
	if isInteger {
		return &parser.Literal{Kind: "integer", Value: strconv.FormatInt(int64(value), 10)}
	}
	return &parser.Literal{Kind: "float", Value: strconv.FormatFloat(value, 'f', -1, 64)}
}

// columnPruning removes columns from LogicalScan nodes that are not referenced by any
// operator above the scan. The primary key column is always retained for row identity.
// If a StarExpr appears anywhere in the tree all columns are kept.
func columnPruning(node LogicalNode) LogicalNode {
	referenced := make(map[string]bool)
	gatherReferencedColumns(node, referenced)
	if referenced["*"] {
		return node
	}
	return pruneScans(node, referenced)
}

func gatherReferencedColumns(node LogicalNode, result map[string]bool) {
	switch typed := node.(type) {
	case *LogicalFilter:
		collectColumnNamesInto(typed.Predicate, result)
		gatherReferencedColumns(typed.Child, result)
	case *LogicalProject:
		for _, col := range typed.Columns {
			collectColumnNamesInto(col, result)
		}
		gatherReferencedColumns(typed.Child, result)
	case *LogicalJoin:
		if typed.Condition != nil {
			collectColumnNamesInto(typed.Condition, result)
		}
		gatherReferencedColumns(typed.Left, result)
		gatherReferencedColumns(typed.Right, result)
	case *LogicalAggregate:
		for _, expr := range typed.GroupBy {
			collectColumnNamesInto(expr, result)
		}
		for _, agg := range typed.Aggs {
			collectColumnNamesInto(agg, result)
		}
		gatherReferencedColumns(typed.Child, result)
	case *LogicalSort:
		for _, orderClause := range typed.OrderBy {
			collectColumnNamesInto(orderClause.Expr, result)
		}
		gatherReferencedColumns(typed.Child, result)
	case *LogicalLimit:
		gatherReferencedColumns(typed.Child, result)
	}
}

func collectColumnNamesInto(expr parser.Expr, result map[string]bool) {
	switch typed := expr.(type) {
	case *parser.ColumnRef:
		result[typed.Column] = true
	case *parser.BinaryExpr:
		collectColumnNamesInto(typed.Left, result)
		collectColumnNamesInto(typed.Right, result)
	case *parser.UnaryExpr:
		collectColumnNamesInto(typed.Operand, result)
	case *parser.FuncCall:
		for _, arg := range typed.Args {
			collectColumnNamesInto(arg, result)
		}
	case *parser.StarExpr:
		result["*"] = true
	}
}

func pruneScans(node LogicalNode, referenced map[string]bool) LogicalNode {
	switch typed := node.(type) {
	case *LogicalScan:
		var kept []catalog.ColumnDef
		for _, col := range typed.Columns {
			if col.PrimaryKey || referenced[col.Name] {
				kept = append(kept, col)
			}
		}
		if len(kept) == 0 && len(typed.Columns) > 0 {
			kept = []catalog.ColumnDef{typed.Columns[0]}
		}
		typed.Columns = kept
		return typed
	case *LogicalFilter:
		typed.Child = pruneScans(typed.Child, referenced)
		return typed
	case *LogicalProject:
		typed.Child = pruneScans(typed.Child, referenced)
		return typed
	case *LogicalJoin:
		typed.Left = pruneScans(typed.Left, referenced)
		typed.Right = pruneScans(typed.Right, referenced)
		return typed
	case *LogicalAggregate:
		typed.Child = pruneScans(typed.Child, referenced)
		return typed
	case *LogicalSort:
		typed.Child = pruneScans(typed.Child, referenced)
		return typed
	case *LogicalLimit:
		typed.Child = pruneScans(typed.Child, referenced)
		return typed
	default:
		return node
	}
}

// indexSelection finds LogicalFilter nodes sitting directly above a LogicalScan where the
// predicate is a simple single-column comparison. If the catalog has an index on that
// column the pair is replaced with a LogicalIndexScan.
func (o *Optimizer) indexSelection(node LogicalNode) LogicalNode {
	switch typed := node.(type) {
	case *LogicalFilter:
		typed.Child = o.indexSelection(typed.Child)
		scan, ok := typed.Child.(*LogicalScan)
		if !ok {
			return typed
		}
		columnName, exact, low, high := extractIndexPredicate(typed.Predicate)
		if columnName == "" {
			return typed
		}
		indexes, err := o.catalog.IndexesForTable(scan.Table)
		if err != nil {
			return typed
		}
		for _, indexDef := range indexes {
			if indexDef.ColumnName == columnName {
				return &LogicalIndexScan{
					Table:   scan.Table,
					Index:   indexDef.Name,
					Column:  columnName,
					Columns: scan.Columns,
					Exact:   exact,
					Low:     low,
					High:    high,
				}
			}
		}
		return typed
	case *LogicalJoin:
		typed.Left = o.indexSelection(typed.Left)
		typed.Right = o.indexSelection(typed.Right)
		return typed
	case *LogicalProject:
		typed.Child = o.indexSelection(typed.Child)
		return typed
	case *LogicalAggregate:
		typed.Child = o.indexSelection(typed.Child)
		return typed
	case *LogicalSort:
		typed.Child = o.indexSelection(typed.Child)
		return typed
	case *LogicalLimit:
		typed.Child = o.indexSelection(typed.Child)
		return typed
	default:
		return node
	}
}

// extractIndexPredicate checks whether an expression is a simple single-column comparison
// eligible for index lookup. Returns the column name and bound values; returns an empty
// column name if the predicate is not index-eligible.
//
// Supported forms: col = value, col > value, col >= value, col < value, col <= value.
// Both col op value and value op col are handled (the operator is flipped for the latter).
func extractIndexPredicate(expr parser.Expr) (columnName string, exact, low, high parser.Expr) {
	binary, ok := expr.(*parser.BinaryExpr)
	if !ok {
		return "", nil, nil, nil
	}

	var column *parser.ColumnRef
	var literal *parser.Literal
	flipped := false

	if colRef, leftOk := binary.Left.(*parser.ColumnRef); leftOk {
		if lit, rightOk := binary.Right.(*parser.Literal); rightOk {
			column = colRef
			literal = lit
		}
	} else if colRef, rightOk := binary.Right.(*parser.ColumnRef); rightOk {
		if lit, leftOk := binary.Left.(*parser.Literal); leftOk {
			column = colRef
			literal = lit
			flipped = true
		}
	}

	if column == nil {
		return "", nil, nil, nil
	}

	op := binary.Op
	if flipped {
		switch op {
		case "<":
			op = ">"
		case ">":
			op = "<"
		case "<=":
			op = ">="
		case ">=":
			op = "<="
		}
	}

	switch op {
	case "=":
		return column.Column, literal, nil, nil
	case ">", ">=":
		return column.Column, nil, literal, nil
	case "<", "<=":
		return column.Column, nil, nil, literal
	}

	return "", nil, nil, nil
}
