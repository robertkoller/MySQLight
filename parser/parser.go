package parser

type Parser struct {
	// TODO: lexer   *Lexer
	// TODO: current Token  — the token we just consumed
	// TODO: peek    Token  — one token of lookahead
}

// NewParser creates a Lexer for the input string and pre-fills the current and peek token
// slots by calling NextToken twice, so the parser always has one token of lookahead available.
func NewParser(input string) *Parser {
	// TODO: create a Lexer, pre-fill current and peek by calling lexer.NextToken() twice
	panic("not implemented")
}

// Parse dispatches to the correct parse function based on the first token of the statement.
// It handles SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, BEGIN, COMMIT, ROLLBACK, and ANALYZE.
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

// parseSelect parses a full SELECT statement including the column list, FROM clause,
// optional JOIN clauses, WHERE expression, GROUP BY, HAVING, ORDER BY, and LIMIT.
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

// parseInsert parses INSERT INTO ... VALUES ..., reading the table name, an optional
// column list in parentheses, and one or more value rows.
func (p *Parser) parseInsert() (*InsertStmt, error) {
	// TODO: expect INSERT INTO, read table name
	// TODO: parse optional column list in parens
	// TODO: expect VALUES, parse one or more value rows
	panic("not implemented")
}

// parseUpdate parses UPDATE ... SET ... WHERE ..., reading the table name, the
// comma-separated list of column assignments, and the optional WHERE expression.
func (p *Parser) parseUpdate() (*UpdateStmt, error) {
	// TODO: expect UPDATE, read table name
	// TODO: expect SET, parse comma-separated col=expr assignments
	// TODO: parse optional WHERE expr
	panic("not implemented")
}

// parseDelete parses DELETE FROM ... WHERE ..., reading the table name and the optional
// WHERE expression.
func (p *Parser) parseDelete() (*DeleteStmt, error) {
	// TODO: expect DELETE FROM, read table name
	// TODO: parse optional WHERE expr
	panic("not implemented")
}

// parseCreate reads the CREATE keyword and then dispatches to parseCreateTable or
// parseCreateIndex based on whether the next token is TABLE, INDEX, or UNIQUE.
func (p *Parser) parseCreate() (Statement, error) {
	// TODO: expect CREATE
	// TODO: if next token is TABLE → parseCreateTable
	// TODO: if next token is INDEX or UNIQUE INDEX → parseCreateIndex
	panic("not implemented")
}

// parseCreateTable reads the table name and a parenthesised list of column definitions
// and FOREIGN KEY constraints, returning a CreateTableStmt.
func (p *Parser) parseCreateTable() (*CreateTableStmt, error) {
	// TODO: expect TABLE, read table name
	// TODO: expect '(', parse comma-separated column defs and FOREIGN KEY constraints
	// TODO: expect ')'
	panic("not implemented")
}

// parseCreateIndex reads an optional UNIQUE keyword, then the index name, the target
// table name after ON, and the indexed column name in parentheses.
func (p *Parser) parseCreateIndex() (*CreateIndexStmt, error) {
	// TODO: handle optional UNIQUE keyword
	// TODO: expect INDEX, read index name
	// TODO: expect ON, read table name
	// TODO: expect '(', read column name, expect ')'
	panic("not implemented")
}

// parseDrop reads the DROP keyword and dispatches to produce either a DropTableStmt or
// a DropIndexStmt depending on whether the next token is TABLE or INDEX.
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

// parseExpr is the entry point for expression parsing. It delegates to parseOr, which is
// the lowest-precedence level, and the chain of functions below it implements precedence
// climbing: each level handles one tier of operators and calls the next-higher-precedence
// function for its operands.
func (p *Parser) parseExpr() (Expr, error) {
	// TODO: call parseOr
	panic("not implemented")
}

// parseOr parses one or more AND expressions joined by OR operators.
func (p *Parser) parseOr() (Expr, error) {
	panic("not implemented")
}

// parseAnd parses one or more NOT expressions joined by AND operators.
func (p *Parser) parseAnd() (Expr, error) {
	panic("not implemented")
}

// parseNot handles an optional leading NOT keyword, then delegates to parseComparison.
func (p *Parser) parseNot() (Expr, error) {
	panic("not implemented")
}

// parseComparison parses two additive expressions connected by =, !=, <, >, <=, >=, or LIKE.
func (p *Parser) parseComparison() (Expr, error) {
	panic("not implemented")
}

// parseAddSub parses one or more multiplicative expressions joined by + or - operators.
func (p *Parser) parseAddSub() (Expr, error) {
	panic("not implemented")
}

// parseMulDiv parses one or more unary expressions joined by *, /, or % operators.
func (p *Parser) parseMulDiv() (Expr, error) {
	panic("not implemented")
}

// parseUnary handles an optional leading minus sign and then delegates to parsePrimary.
func (p *Parser) parseUnary() (Expr, error) {
	panic("not implemented")
}

// parsePrimary handles the highest-precedence expression forms: integer, float, and string
// literals; the NULL keyword; the star wildcard; aggregate function calls; column references
// (optionally qualified with a table name); and parenthesised sub-expressions.
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

// advance shifts the current token to the previous peek token, reads the next token from
// the lexer into peek, and returns the token that was just consumed.
func (p *Parser) advance() Token {
	// TODO: current = peek; peek = lexer.NextToken(); return old current
	panic("not implemented")
}

// expect asserts that the current token matches the expected type, advances past it, and
// returns the consumed token. If the type does not match, it returns a parse error that
// includes the current line number to aid debugging.
func (p *Parser) expect(t TokenType) (Token, error) {
	// TODO: if current.Type != t return a parse error with line info
	// TODO: otherwise advance and return the consumed token
	panic("not implemented")
}
