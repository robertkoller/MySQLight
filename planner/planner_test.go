package planner

import (
	"os"
	"testing"

	"github.com/robertkoller/MySQLight/catalog"
	"github.com/robertkoller/MySQLight/parser"
	"github.com/robertkoller/MySQLight/storage"
)

// newTestCatalog creates a temporary on-disk catalog pre-populated with a "users" table
// (id INT PK, name TEXT, age INT) and an index on age. The returned cleanup func flushes
// and removes the temp file.
func newTestCatalog(t *testing.T) (*catalog.Catalog, func()) {
	t.Helper()

	file, err := os.CreateTemp("", "planner_test_*.db")
	if err != nil {
		t.Fatal(err)
	}
	file.Close()

	pager, err := storage.Open(file.Name())
	if err != nil {
		os.Remove(file.Name())
		t.Fatal(err)
	}

	pool := storage.NewBufferPool(pager, 64)

	tree, err := storage.NewBTree(pool, 0)
	if err != nil {
		pager.Close()
		os.Remove(file.Name())
		t.Fatal(err)
	}

	cat, err := catalog.Open(tree)
	if err != nil {
		pager.Close()
		os.Remove(file.Name())
		t.Fatal(err)
	}

	usersTable := &catalog.TableDef{
		Name: "users",
		Columns: []catalog.ColumnDef{
			{Name: "id", Type: catalog.TypeInt, PrimaryKey: true, NotNull: true},
			{Name: "name", Type: catalog.TypeText, NotNull: true},
			{Name: "age", Type: catalog.TypeInt},
		},
	}
	if createErr := cat.CreateTable(usersTable); createErr != nil {
		pager.Close()
		os.Remove(file.Name())
		t.Fatal(createErr)
	}

	ageIndex := &catalog.IndexDef{
		Name:       "idx_age",
		TableName:  "users",
		ColumnName: "age",
	}
	if indexErr := cat.CreateIndex(ageIndex); indexErr != nil {
		pager.Close()
		os.Remove(file.Name())
		t.Fatal(indexErr)
	}

	return cat, func() {
		pool.FlushAll()
		pager.Close()
		os.Remove(file.Name())
	}
}

// mustParse parses a SQL string and returns the first statement, failing the test on error.
func mustParse(t *testing.T, sql string) parser.Statement {
	t.Helper()
	p, err := parser.NewParser(sql)
	if err != nil {
		t.Fatalf("NewParser(%q): %v", sql, err)
	}
	stmt, err := p.Parse()
	if err != nil {
		t.Fatalf("Parse(%q): %v", sql, err)
	}
	return stmt
}

func TestPlanSimpleSelect(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	planner := NewPlanner(cat)
	stmt := mustParse(t, "SELECT id, name FROM users WHERE age > 25")

	root, err := planner.Plan(stmt)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	// Expected tree: Project → Filter → Scan
	project, ok := root.(*LogicalProject)
	if !ok {
		t.Fatalf("root: got %T, want *LogicalProject", root)
	}
	if len(project.Columns) != 2 {
		t.Errorf("project columns: got %d, want 2", len(project.Columns))
	}

	filter, ok := project.Child.(*LogicalFilter)
	if !ok {
		t.Fatalf("project.Child: got %T, want *LogicalFilter", project.Child)
	}
	if filter.Predicate == nil {
		t.Error("filter.Predicate should be non-nil")
	}

	scan, ok := filter.Child.(*LogicalScan)
	if !ok {
		t.Fatalf("filter.Child: got %T, want *LogicalScan", filter.Child)
	}
	if scan.Table != "users" {
		t.Errorf("scan.Table: got %q, want %q", scan.Table, "users")
	}
	if len(scan.Columns) != 3 {
		t.Errorf("scan.Columns: got %d, want 3 (full schema)", len(scan.Columns))
	}
}

func TestPlanSelectStar(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	planner := NewPlanner(cat)
	root, err := planner.Plan(mustParse(t, "SELECT * FROM users"))
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	project, ok := root.(*LogicalProject)
	if !ok {
		t.Fatalf("root: got %T, want *LogicalProject", root)
	}
	if len(project.Columns) != 1 {
		t.Errorf("project columns: got %d, want 1 (the StarExpr)", len(project.Columns))
	}
	if _, isStar := project.Columns[0].(*parser.StarExpr); !isStar {
		t.Errorf("project.Columns[0]: got %T, want *parser.StarExpr", project.Columns[0])
	}
}

func TestPlanSelectWithGroupBy(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	planner := NewPlanner(cat)
	root, err := planner.Plan(mustParse(t, "SELECT age, COUNT(*) FROM users GROUP BY age"))
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	agg, ok := root.(*LogicalAggregate)
	if !ok {
		t.Fatalf("root: got %T, want *LogicalAggregate", root)
	}
	if len(agg.GroupBy) != 1 {
		t.Errorf("GroupBy: got %d expressions, want 1", len(agg.GroupBy))
	}
	if _, isScan := agg.Child.(*LogicalScan); !isScan {
		t.Errorf("agg.Child: got %T, want *LogicalScan", agg.Child)
	}
}

func TestPlanSelectWithLimit(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	planner := NewPlanner(cat)
	root, err := planner.Plan(mustParse(t, "SELECT id FROM users LIMIT 10"))
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	limit, ok := root.(*LogicalLimit)
	if !ok {
		t.Fatalf("root: got %T, want *LogicalLimit", root)
	}
	if limit.N != 10 {
		t.Errorf("limit.N: got %d, want 10", limit.N)
	}
}

func TestPlanTableNotFound(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	planner := NewPlanner(cat)
	_, err := planner.Plan(mustParse(t, "SELECT id FROM nonexistent"))
	if err == nil {
		t.Fatal("expected error for unknown table, got nil")
	}
}

func TestPlanUnsupportedStatement(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	planner := NewPlanner(cat)
	_, err := planner.Plan(&parser.InsertStmt{Table: "users"})
	if err == nil {
		t.Fatal("expected error for non-SELECT statement, got nil")
	}
}

func TestPredicatePushdown(t *testing.T) {
	// Build:  Filter(users.age > 25)  →  Join(users.id = orders.user_id)  →  [Scan(users), Scan(orders)]
	// After push-down: Join  →  [Filter(users.age > 25) → Scan(users),  Scan(orders)]
	tree := &LogicalFilter{
		Predicate: &parser.BinaryExpr{
			Left:  &parser.ColumnRef{Table: "users", Column: "age"},
			Op:    ">",
			Right: &parser.Literal{Kind: "integer", Value: "25"},
		},
		Child: &LogicalJoin{
			Left:  &LogicalScan{Table: "users"},
			Right: &LogicalScan{Table: "orders"},
			Condition: &parser.BinaryExpr{
				Left:  &parser.ColumnRef{Table: "users", Column: "id"},
				Op:    "=",
				Right: &parser.ColumnRef{Table: "orders", Column: "user_id"},
			},
		},
	}

	result := predicatePushdown(tree)

	join, ok := result.(*LogicalJoin)
	if !ok {
		t.Fatalf("after push-down root: got %T, want *LogicalJoin", result)
	}

	leftFilter, ok := join.Left.(*LogicalFilter)
	if !ok {
		t.Fatalf("join.Left after push-down: got %T, want *LogicalFilter", join.Left)
	}
	if _, isScan := leftFilter.Child.(*LogicalScan); !isScan {
		t.Errorf("leftFilter.Child: got %T, want *LogicalScan", leftFilter.Child)
	}

	if _, isScan := join.Right.(*LogicalScan); !isScan {
		t.Errorf("join.Right: got %T, want *LogicalScan (filter should not have moved here)", join.Right)
	}
}

func TestPredicatePushdownCrossTablePredicateStaysAboveJoin(t *testing.T) {
	// A predicate that references both tables cannot be pushed to either side.
	tree := &LogicalFilter{
		Predicate: &parser.BinaryExpr{
			Left:  &parser.ColumnRef{Table: "users", Column: "id"},
			Op:    "=",
			Right: &parser.ColumnRef{Table: "orders", Column: "user_id"},
		},
		Child: &LogicalJoin{
			Left:  &LogicalScan{Table: "users"},
			Right: &LogicalScan{Table: "orders"},
		},
	}

	result := predicatePushdown(tree)

	if _, ok := result.(*LogicalFilter); !ok {
		t.Fatalf("cross-table predicate should remain as a Filter above the Join, got %T", result)
	}
}

func TestConstantFoldingArithmetic(t *testing.T) {
	// Filter predicate: 2 + 3 > 4  →  5 > 4  →  1 (true)
	tree := &LogicalFilter{
		Predicate: &parser.BinaryExpr{
			Left: &parser.BinaryExpr{
				Left:  &parser.Literal{Kind: "integer", Value: "2"},
				Op:    "+",
				Right: &parser.Literal{Kind: "integer", Value: "3"},
			},
			Op:    ">",
			Right: &parser.Literal{Kind: "integer", Value: "4"},
		},
		Child: &LogicalScan{Table: "users"},
	}

	result := constantFolding(tree)

	filter, ok := result.(*LogicalFilter)
	if !ok {
		t.Fatalf("result: got %T, want *LogicalFilter", result)
	}
	literal, ok := filter.Predicate.(*parser.Literal)
	if !ok {
		t.Fatalf("predicate after folding: got %T, want *parser.Literal", filter.Predicate)
	}
	if literal.Kind != "integer" || literal.Value != "1" {
		t.Errorf("folded predicate: got {%s %s}, want {integer 1}", literal.Kind, literal.Value)
	}
}

func TestConstantFoldingFalse(t *testing.T) {
	// 3 > 10 → 0 (false)
	tree := &LogicalFilter{
		Predicate: &parser.BinaryExpr{
			Left:  &parser.Literal{Kind: "integer", Value: "3"},
			Op:    ">",
			Right: &parser.Literal{Kind: "integer", Value: "10"},
		},
		Child: &LogicalScan{Table: "users"},
	}

	result := constantFolding(tree)
	filter := result.(*LogicalFilter)
	literal, ok := filter.Predicate.(*parser.Literal)
	if !ok {
		t.Fatalf("predicate after folding: got %T, want *parser.Literal", filter.Predicate)
	}
	if literal.Value != "0" {
		t.Errorf("folded false predicate: got %q, want %q", literal.Value, "0")
	}
}

func TestConstantFoldingAndOr(t *testing.T) {
	// 1 AND 0 → 0
	tree := &LogicalFilter{
		Predicate: &parser.BinaryExpr{
			Left:  &parser.Literal{Kind: "integer", Value: "1"},
			Op:    "AND",
			Right: &parser.Literal{Kind: "integer", Value: "0"},
		},
		Child: &LogicalScan{Table: "users"},
	}
	result := constantFolding(tree)
	filter := result.(*LogicalFilter)
	literal, ok := filter.Predicate.(*parser.Literal)
	if !ok {
		t.Fatalf("predicate: got %T, want *parser.Literal", filter.Predicate)
	}
	if literal.Value != "0" {
		t.Errorf("1 AND 0: got %q, want %q", literal.Value, "0")
	}
}

func TestConstantFoldingPreservesColumnRef(t *testing.T) {
	// age > 25 — not constant; must be left unchanged.
	predicate := &parser.BinaryExpr{
		Left:  &parser.ColumnRef{Column: "age"},
		Op:    ">",
		Right: &parser.Literal{Kind: "integer", Value: "25"},
	}
	tree := &LogicalFilter{
		Predicate: predicate,
		Child:     &LogicalScan{Table: "users"},
	}

	result := constantFolding(tree)
	filter := result.(*LogicalFilter)
	if _, ok := filter.Predicate.(*parser.BinaryExpr); !ok {
		t.Errorf("non-constant predicate should remain BinaryExpr, got %T", filter.Predicate)
	}
}

func TestColumnPruning(t *testing.T) {
	// SELECT name FROM users WHERE age > 25
	// Referenced columns: name (project), age (filter). PK (id) always kept.
	// After pruning: scan should have [id, name, age] — all three are referenced or PK.
	tree := &LogicalProject{
		Columns: []parser.Expr{&parser.ColumnRef{Column: "name"}},
		Child: &LogicalFilter{
			Predicate: &parser.BinaryExpr{
				Left:  &parser.ColumnRef{Column: "age"},
				Op:    ">",
				Right: &parser.Literal{Kind: "integer", Value: "25"},
			},
			Child: &LogicalScan{
				Table: "users",
				Columns: []catalog.ColumnDef{
					{Name: "id", Type: catalog.TypeInt, PrimaryKey: true},
					{Name: "name", Type: catalog.TypeText},
					{Name: "age", Type: catalog.TypeInt},
					{Name: "email", Type: catalog.TypeText},
				},
			},
		},
	}

	result := columnPruning(tree)

	project := result.(*LogicalProject)
	filter := project.Child.(*LogicalFilter)
	scan := filter.Child.(*LogicalScan)

	// id (PK), name, age should be kept; email should be pruned.
	if len(scan.Columns) != 3 {
		t.Errorf("pruned scan columns: got %d, want 3 (id, name, age)", len(scan.Columns))
	}
	for _, col := range scan.Columns {
		if col.Name == "email" {
			t.Error("email should have been pruned but was kept")
		}
	}
}

func TestColumnPruningKeepsStarExpr(t *testing.T) {
	// SELECT * — StarExpr means all columns must be kept.
	tree := &LogicalProject{
		Columns: []parser.Expr{&parser.StarExpr{}},
		Child: &LogicalScan{
			Table: "users",
			Columns: []catalog.ColumnDef{
				{Name: "id", Type: catalog.TypeInt, PrimaryKey: true},
				{Name: "name", Type: catalog.TypeText},
				{Name: "age", Type: catalog.TypeInt},
			},
		},
	}

	result := columnPruning(tree)

	project := result.(*LogicalProject)
	scan := project.Child.(*LogicalScan)
	if len(scan.Columns) != 3 {
		t.Errorf("SELECT * should keep all columns; got %d, want 3", len(scan.Columns))
	}
}

func TestExtractIndexPredicateEquality(t *testing.T) {
	expr := &parser.BinaryExpr{
		Left:  &parser.ColumnRef{Column: "age"},
		Op:    "=",
		Right: &parser.Literal{Kind: "integer", Value: "30"},
	}
	columnName, exact, low, high := extractIndexPredicate(expr)
	if columnName != "age" {
		t.Errorf("columnName: got %q, want %q", columnName, "age")
	}
	if exact == nil {
		t.Error("exact: should be non-nil for equality")
	}
	if low != nil || high != nil {
		t.Error("low and high should be nil for equality")
	}
}

func TestExtractIndexPredicateRange(t *testing.T) {
	expr := &parser.BinaryExpr{
		Left:  &parser.ColumnRef{Column: "age"},
		Op:    ">",
		Right: &parser.Literal{Kind: "integer", Value: "18"},
	}
	columnName, exact, low, high := extractIndexPredicate(expr)
	if columnName != "age" {
		t.Errorf("columnName: got %q, want %q", columnName, "age")
	}
	if exact != nil {
		t.Error("exact: should be nil for range predicate")
	}
	if low == nil {
		t.Error("low: should be non-nil for > predicate")
	}
	if high != nil {
		t.Error("high: should be nil for > predicate")
	}
}

func TestExtractIndexPredicateFlipped(t *testing.T) {
	// 30 = age — same as age = 30 but operands are flipped.
	expr := &parser.BinaryExpr{
		Left:  &parser.Literal{Kind: "integer", Value: "30"},
		Op:    "=",
		Right: &parser.ColumnRef{Column: "age"},
	}
	columnName, exact, _, _ := extractIndexPredicate(expr)
	if columnName != "age" {
		t.Errorf("flipped equality: columnName got %q, want %q", columnName, "age")
	}
	if exact == nil {
		t.Error("flipped equality: exact should be non-nil")
	}
}

func TestExtractIndexPredicateNonEligible(t *testing.T) {
	// col1 = col2 — two column refs, not index-eligible.
	expr := &parser.BinaryExpr{
		Left:  &parser.ColumnRef{Column: "id"},
		Op:    "=",
		Right: &parser.ColumnRef{Column: "user_id"},
	}
	columnName, _, _, _ := extractIndexPredicate(expr)
	if columnName != "" {
		t.Errorf("two-column predicate should not be index-eligible, got columnName %q", columnName)
	}
}

func TestIndexSelection(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	optimizer := NewOptimizer(cat)

	// Build Filter(age = 30) → Scan(users) manually.
	tree := &LogicalFilter{
		Predicate: &parser.BinaryExpr{
			Left:  &parser.ColumnRef{Column: "age"},
			Op:    "=",
			Right: &parser.Literal{Kind: "integer", Value: "30"},
		},
		Child: &LogicalScan{
			Table: "users",
			Columns: []catalog.ColumnDef{
				{Name: "id", Type: catalog.TypeInt, PrimaryKey: true},
				{Name: "name", Type: catalog.TypeText},
				{Name: "age", Type: catalog.TypeInt},
			},
		},
	}

	optimized, err := optimizer.Optimize(tree)
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}

	indexScan, ok := optimized.(*LogicalIndexScan)
	if !ok {
		t.Fatalf("after optimization: got %T, want *LogicalIndexScan", optimized)
	}
	if indexScan.Index != "idx_age" {
		t.Errorf("Index: got %q, want %q", indexScan.Index, "idx_age")
	}
	if indexScan.Column != "age" {
		t.Errorf("Column: got %q, want %q", indexScan.Column, "age")
	}
	if indexScan.Exact == nil {
		t.Error("Exact: should be non-nil for equality predicate")
	}
	if indexScan.Low != nil || indexScan.High != nil {
		t.Error("Low and High should be nil for equality predicate")
	}
}

func TestIndexSelectionNoIndex(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	optimizer := NewOptimizer(cat)

	// name has no index — filter should remain.
	tree := &LogicalFilter{
		Predicate: &parser.BinaryExpr{
			Left:  &parser.ColumnRef{Column: "name"},
			Op:    "=",
			Right: &parser.Literal{Kind: "string", Value: "alice"},
		},
		Child: &LogicalScan{
			Table: "users",
			Columns: []catalog.ColumnDef{
				{Name: "id", Type: catalog.TypeInt, PrimaryKey: true},
				{Name: "name", Type: catalog.TypeText},
				{Name: "age", Type: catalog.TypeInt},
			},
		},
	}

	optimized, err := optimizer.Optimize(tree)
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}

	// Should remain a Filter → Scan (no index on name).
	// After column pruning: scan keeps id (PK) and name (referenced).
	filter, ok := optimized.(*LogicalFilter)
	if !ok {
		t.Fatalf("without index: got %T, want *LogicalFilter", optimized)
	}
	if _, isScan := filter.Child.(*LogicalScan); !isScan {
		t.Errorf("filter.Child: got %T, want *LogicalScan", filter.Child)
	}
}

func TestFullPipelineWithOptimizer(t *testing.T) {
	cat, cleanup := newTestCatalog(t)
	defer cleanup()

	planner := NewPlanner(cat)
	optimizer := NewOptimizer(cat)

	stmt := mustParse(t, "SELECT id, name FROM users WHERE age = 30")

	root, err := planner.Plan(stmt)
	if err != nil {
		t.Fatalf("Plan: %v", err)
	}

	optimized, err := optimizer.Optimize(root)
	if err != nil {
		t.Fatalf("Optimize: %v", err)
	}

	// Expected: Project → IndexScan (age index found, filter collapsed into scan)
	project, ok := optimized.(*LogicalProject)
	if !ok {
		t.Fatalf("optimized root: got %T, want *LogicalProject", optimized)
	}
	indexScan, ok := project.Child.(*LogicalIndexScan)
	if !ok {
		t.Fatalf("project.Child: got %T, want *LogicalIndexScan", project.Child)
	}
	if indexScan.Table != "users" {
		t.Errorf("IndexScan.Table: got %q, want %q", indexScan.Table, "users")
	}
	if indexScan.Index != "idx_age" {
		t.Errorf("IndexScan.Index: got %q, want %q", indexScan.Index, "idx_age")
	}
}
