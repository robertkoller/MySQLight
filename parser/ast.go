package parser

// Statement is the top-level AST node for any SQL statement.
type Statement interface {
	stmtNode()
}

type SelectStmt struct {
	Columns []Expr
	From    string
	Joins   []JoinClause
	Where   Expr
	GroupBy []Expr
	Having  Expr
	OrderBy []OrderClause
	Limit   *int
}

type JoinClause struct {
	Table string
	Alias string
	On    Expr
}

type OrderClause struct {
	Expr Expr
	Desc bool
}

type InsertStmt struct {
	Table   string
	Columns []string
	Values  [][]Expr
}

type UpdateStmt struct {
	Table  string
	Set    []Assignment
	Where  Expr
}

type Assignment struct {
	Column string
	Value  Expr
}

type DeleteStmt struct {
	Table string
	Where Expr
}

type CreateTableStmt struct {
	Table       string
	Columns     []ColumnDefAST
	ForeignKeys []ForeignKeyAST
}

type ColumnDefAST struct {
	Name       string
	TypeName   string // "INTEGER", "FLOAT", "TEXT", "BLOB"
	PrimaryKey bool
	NotNull    bool
	Default    Expr
}

type ForeignKeyAST struct {
	Column    string
	RefTable  string
	RefColumn string
	OnDelete  string // "RESTRICT", "CASCADE", "SET NULL"
	OnUpdate  string
}

type CreateIndexStmt struct {
	Index  string
	Table  string
	Column string
	Unique bool
}

type DropTableStmt struct {
	Table string
}

type DropIndexStmt struct {
	Index string
}

type BeginStmt struct{}
type CommitStmt struct{}
type RollbackStmt struct{}

type AnalyzeStmt struct {
	Table string
}

// Expr is the AST node for any expression.
type Expr interface {
	exprNode()
}

type BinaryExpr struct {
	Left  Expr
	Op    string // "=", "!=", "<", ">", "<=", ">=", "+", "-", "*", "/", "%", "AND", "OR", "LIKE"
	Right Expr
}

type UnaryExpr struct {
	Op      string // "-", "NOT"
	Operand Expr
}

type ColumnRef struct {
	Table  string // empty if unqualified
	Column string
}

type Literal struct {
	Kind  string // "integer", "float", "string", "null"
	Value string // raw lexed text
}

type FuncCall struct {
	Name string // "COUNT", "SUM", "MIN", "MAX", "AVG"
	Args []Expr
	Star bool // COUNT(*)
}

type StarExpr struct{} // the bare * in SELECT *

func (s *SelectStmt) stmtNode()      {}
func (s *InsertStmt) stmtNode()      {}
func (s *UpdateStmt) stmtNode()      {}
func (s *DeleteStmt) stmtNode()      {}
func (s *CreateTableStmt) stmtNode() {}
func (s *CreateIndexStmt) stmtNode() {}
func (s *DropTableStmt) stmtNode()   {}
func (s *DropIndexStmt) stmtNode()   {}
func (s *BeginStmt) stmtNode()       {}
func (s *CommitStmt) stmtNode()      {}
func (s *RollbackStmt) stmtNode()    {}
func (s *AnalyzeStmt) stmtNode()     {}

func (e *BinaryExpr) exprNode() {}
func (e *UnaryExpr) exprNode()  {}
func (e *ColumnRef) exprNode()  {}
func (e *Literal) exprNode()    {}
func (e *FuncCall) exprNode()   {}
func (e *StarExpr) exprNode()   {}
