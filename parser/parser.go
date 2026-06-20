package parser

type Parser struct {
	// TODO: lexer   *Lexer
	// TODO: current Token  — the token we just consumed
	// TODO: peek    Token  — one token of lookahead
}

func NewParser(input string) *Parser {
	// TODO: create a Lexer, pre-fill current and peek by calling lexer.NextToken() twice
	panic("not implemented")
}

func (p *Parser) Parse() (Statement, error) {
	// TODO: dispatch on p.current.Type to the correct parse function
	//   TokenSelect   → parseSelect
	//   TokenInsert   → parseInsert
	//   TokenUpdate   → parseUpdate
	//   TokenDelete   → parseDelete
	//   TokenCreate   → parseCreate (table or index depending on next token)
	//   TokenDrop     → parseDrop
	//   TokenBegin    → return &BeginStmt{}
	//   TokenCommit   → return &CommitStmt{}
	//   TokenRollback → return &RollbackStmt{}
	//   TokenAnalyze  → parseAnalyze
	panic("not implemented")
}

func (p *Parser) parseSelect() (*SelectStmt, error) {
	// TODO: expect TokenSelect
	// TODO: parse column list (* or comma-separated expressions)
	// TODO: expect TokenFrom, read table name
	// TODO: parse optional JOIN clauses
	// TODO: parse optional WHERE expr
	// TODO: parse optional GROUP BY exprs
	// TODO: parse optional HAVING expr
	// TODO: parse optional ORDER BY clauses
	// TODO: parse optional LIMIT integer
	panic("not implemented")
}

func (p *Parser) parseInsert() (*InsertStmt, error) {
	// TODO: expect INSERT INTO, read table name
	// TODO: parse optional column list in parens
	// TODO: expect VALUES, parse one or more value rows
	panic("not implemented")
}

func (p *Parser) parseUpdate() (*UpdateStmt, error) {
	// TODO: expect UPDATE, read table name
	// TODO: expect SET, parse comma-separated col=expr assignments
	// TODO: parse optional WHERE expr
	panic("not implemented")
}

func (p *Parser) parseDelete() (*DeleteStmt, error) {
	// TODO: expect DELETE FROM, read table name
	// TODO: parse optional WHERE expr
	panic("not implemented")
}

func (p *Parser) parseCreate() (Statement, error) {
	// TODO: expect CREATE
	// TODO: if next token is TABLE → parseCreateTable
	// TODO: if next token is INDEX or UNIQUE INDEX → parseCreateIndex
	panic("not implemented")
}

func (p *Parser) parseCreateTable() (*CreateTableStmt, error) {
	// TODO: expect TABLE, read table name
	// TODO: expect '(', parse comma-separated column defs and FOREIGN KEY constraints
	// TODO: expect ')'
	panic("not implemented")
}

func (p *Parser) parseCreateIndex() (*CreateIndexStmt, error) {
	// TODO: handle optional UNIQUE keyword
	// TODO: expect INDEX, read index name
	// TODO: expect ON, read table name
	// TODO: expect '(', read column name, expect ')'
	panic("not implemented")
}

func (p *Parser) parseDrop() (Statement, error) {
	// TODO: expect DROP
	// TODO: if TABLE → DropTableStmt; if INDEX → DropIndexStmt
	panic("not implemented")
}

// --- Expression parsing (precedence climbing via recursive descent) ---
// Grammar (low to high precedence):
//   expr        → or
//   or          → and (OR and)*
//   and         → not (AND not)*
//   not         → NOT comparison | comparison
//   comparison  → addSub ((= | != | < | > | <= | >= | LIKE) addSub)?
//   addSub      → mulDiv ((+ | -) mulDiv)*
//   mulDiv      → unary ((* | / | %) unary)*
//   unary       → - primary | primary
//   primary     → INTEGER | FLOAT | STRING | NULL | ident | funcCall | ( expr ) | *

func (p *Parser) parseExpr() (Expr, error) {
	// TODO: call parseOr
	panic("not implemented")
}

func (p *Parser) parseOr() (Expr, error) {
	panic("not implemented")
}

func (p *Parser) parseAnd() (Expr, error) {
	panic("not implemented")
}

func (p *Parser) parseNot() (Expr, error) {
	panic("not implemented")
}

func (p *Parser) parseComparison() (Expr, error) {
	panic("not implemented")
}

func (p *Parser) parseAddSub() (Expr, error) {
	panic("not implemented")
}

func (p *Parser) parseMulDiv() (Expr, error) {
	panic("not implemented")
}

func (p *Parser) parseUnary() (Expr, error) {
	panic("not implemented")
}

func (p *Parser) parsePrimary() (Expr, error) {
	// TODO: INTEGER literal → &Literal{Kind:"integer", Value: token.Literal}
	// TODO: FLOAT literal   → &Literal{Kind:"float"}
	// TODO: STRING literal  → &Literal{Kind:"string"}
	// TODO: NULL keyword    → &Literal{Kind:"null"}
	// TODO: *               → &StarExpr{}
	// TODO: ident followed by '(' → funcCall
	// TODO: ident optionally followed by '.' ident → &ColumnRef{}
	// TODO: '(' expr ')'   → grouped expression
	panic("not implemented")
}

// --- Helpers ---

func (p *Parser) advance() Token {
	// TODO: current = peek; peek = lexer.NextToken(); return old current
	panic("not implemented")
}

func (p *Parser) expect(t TokenType) (Token, error) {
	// TODO: if current.Type != t return a parse error with line info
	// TODO: otherwise advance and return the consumed token
	panic("not implemented")
}
