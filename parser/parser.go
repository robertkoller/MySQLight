package parser

import (
	"errors"
	"fmt"
	"strconv"
)

type Parser struct {
	lexer   *Lexer
	current Token // the token we just consumed
	peek    Token // one token of lookahead
}

// NewParser creates a Lexer for the input string and pre-fills the current and peek token
// slots by calling NextToken twice, so the parser always has one token of lookahead available.
func NewParser(input string) (*Parser, error) {
	lexer := NewLexer(input)
	current := lexer.NextToken()

	if current.Type == TokenEOF {
		return &Parser{}, errors.New("EOF error from first read")
	}

	if current.Type == TokenErr {
		return &Parser{}, errors.New("Error reading next token from first read")
	}

	peek := lexer.NextToken()

	if peek.Type == TokenErr {
		return &Parser{}, errors.New("Error reading next token from first peek read")
	}
	return &Parser{
		lexer:   lexer,
		current: current,
		peek:    peek,
	}, nil
}

// Parse dispatches to the correct parse function based on the first token of the statement.
// It handles SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, BEGIN, COMMIT, ROLLBACK, and ANALYZE.
func (p *Parser) Parse() (Statement, error) {
	var stmnt Statement
	var err error

	switch p.current.Type {
	case TokenSelect:
		stmnt, err = p.parseSelect()
	case TokenInsert:
		stmnt, err = p.parseInsert()
	case TokenUpdate:
		stmnt, err = p.parseUpdate()
	case TokenDelete:
		stmnt, err = p.parseDelete()
	case TokenCreate:
		stmnt, err = p.parseCreate()
	case TokenDrop:
		stmnt, err = p.parseDrop()
	case TokenBegin:
		p.advance()
		return &BeginStmt{}, nil
	case TokenCommit:
		p.advance()
		return &CommitStmt{}, nil
	case TokenRollback:
		p.advance()
		return &RollbackStmt{}, nil
	case TokenAnalyze:
		stmnt, err = p.parseAnalyze()
	default:
		return nil, errors.New("Unreadable token type")
	}

	return stmnt, err
}

// parseSelect parses a full SELECT statement including the column list, FROM clause,
// optional JOIN clauses, WHERE expression, GROUP BY, HAVING, ORDER BY, and LIMIT.
func (p *Parser) parseSelect() (*SelectStmt, error) {
	var stmt SelectStmt

	_, err := p.expect(TokenSelect)
	if err != nil {
		return &stmt, err
	}

	if p.current.Type == TokenStar {
		stmt.Columns = append(stmt.Columns, &StarExpr{})
		p.advance()
	} else {
		for {
			expression, err := p.parseExpr()
			if err != nil {
				return &SelectStmt{}, err
			}
			stmt.Columns = append(stmt.Columns, expression)
			if p.current.Type != TokenComma {
				break
			}
			p.advance()
		}
	}

	_, err = p.expect(TokenFrom)
	if err != nil {
		return &stmt, err
	}

	tableToken, err := p.expect(TokenIdent)
	if err != nil {
		return &stmt, err
	}
	stmt.From = tableToken.Literal

	// JOIN
	for p.current.Type == TokenJoin || p.current.Type == TokenInner || p.current.Type == TokenLeft {
		if p.current.Type == TokenInner || p.current.Type == TokenLeft {
			p.advance()
		}
		_, err = p.expect(TokenJoin)
		if err != nil {
			return &stmt, err
		}
		joinTableToken, err := p.expect(TokenIdent)
		if err != nil {
			return &stmt, err
		}
		join := JoinClause{Table: joinTableToken.Literal}
		if p.current.Type == TokenIdent {
			join.Alias = p.current.Literal
			p.advance()
		}
		_, err = p.expect(TokenOn)
		if err != nil {
			return &stmt, err
		}
		join.On, err = p.parseExpr()
		if err != nil {
			return &stmt, err
		}
		stmt.Joins = append(stmt.Joins, join)
	}

	// WHERE
	if p.current.Type == TokenWhere {
		p.advance()
		stmt.Where, err = p.parseExpr()
		if err != nil {
			return &stmt, err
		}
	}

	// GROUP BY
	if p.current.Type == TokenGroup {
		p.advance()
		_, err = p.expect(TokenBy)
		if err != nil {
			return &stmt, err
		}
		for {
			expression, err := p.parseExpr()
			if err != nil {
				return &stmt, err
			}
			stmt.GroupBy = append(stmt.GroupBy, expression)
			if p.current.Type != TokenComma {
				break
			}
			p.advance()
		}
	}

	// HAVING
	if p.current.Type == TokenHaving {
		p.advance()
		stmt.Having, err = p.parseExpr()
		if err != nil {
			return &stmt, err
		}
	}

	// ORDER BY
	if p.current.Type == TokenOrder {
		p.advance()
		_, err = p.expect(TokenBy)
		if err != nil {
			return &stmt, err
		}
		for {
			expression, err := p.parseExpr()
			if err != nil {
				return &stmt, err
			}
			clause := OrderClause{Expr: expression}
			if p.current.Type == TokenDesc {
				clause.Desc = true
				p.advance()
			}
			stmt.OrderBy = append(stmt.OrderBy, clause)
			if p.current.Type != TokenComma {
				break
			}
			p.advance()
		}
	}

	// LIMIT
	if p.current.Type == TokenLimit {
		p.advance()
		limitToken, err := p.expect(TokenInteger)
		if err != nil {
			return &stmt, err
		}
		limitValue, err := strconv.Atoi(limitToken.Literal)
		if err != nil {
			return &stmt, fmt.Errorf("invalid LIMIT value: %s", limitToken.Literal)
		}
		stmt.Limit = &limitValue
	}

	return &stmt, nil
}

// parseInsert parses INSERT INTO table [(cols)] VALUES (vals), ...
func (p *Parser) parseInsert() (*InsertStmt, error) {
	var stmt InsertStmt

	_, err := p.expect(TokenInsert)
	if err != nil {
		return &stmt, err
	}
	_, err = p.expect(TokenInto)
	if err != nil {
		return &stmt, err
	}
	tableToken, err := p.expect(TokenIdent)
	if err != nil {
		return &stmt, err
	}
	stmt.Table = tableToken.Literal

	if p.current.Type == TokenLParen {
		p.advance()
		for {
			colToken, err := p.expect(TokenIdent)
			if err != nil {
				return &stmt, err
			}
			stmt.Columns = append(stmt.Columns, colToken.Literal)
			if p.current.Type != TokenComma {
				break
			}
			p.advance()
		}
		_, err = p.expect(TokenRParen)
		if err != nil {
			return &stmt, err
		}
	}

	_, err = p.expect(TokenValues)
	if err != nil {
		return &stmt, err
	}

	// one or more value rows
	for {
		_, err = p.expect(TokenLParen)
		if err != nil {
			return &stmt, err
		}
		var row []Expr
		for {
			expression, err := p.parseExpr()
			if err != nil {
				return &stmt, err
			}
			row = append(row, expression)
			if p.current.Type != TokenComma {
				break
			}
			p.advance()
		}
		_, err = p.expect(TokenRParen)
		if err != nil {
			return &stmt, err
		}
		stmt.Values = append(stmt.Values, row)
		if p.current.Type != TokenComma {
			break
		}
		p.advance()
	}

	return &stmt, nil
}

// parseUpdate parses UPDATE table SET col=expr, ... [WHERE expr]
func (p *Parser) parseUpdate() (*UpdateStmt, error) {
	var stmt UpdateStmt

	_, err := p.expect(TokenUpdate)
	if err != nil {
		return &stmt, err
	}
	tableToken, err := p.expect(TokenIdent)
	if err != nil {
		return &stmt, err
	}
	stmt.Table = tableToken.Literal

	_, err = p.expect(TokenSet)
	if err != nil {
		return &stmt, err
	}

	for {
		colToken, err := p.expect(TokenIdent)
		if err != nil {
			return &stmt, err
		}
		_, err = p.expect(TokenEq)
		if err != nil {
			return &stmt, err
		}
		value, err := p.parseExpr()
		if err != nil {
			return &stmt, err
		}
		stmt.Set = append(stmt.Set, Assignment{Column: colToken.Literal, Value: value})
		if p.current.Type != TokenComma {
			break
		}
		p.advance()
	}

	if p.current.Type == TokenWhere {
		p.advance()
		stmt.Where, err = p.parseExpr()
		if err != nil {
			return &stmt, err
		}
	}

	return &stmt, nil
}

// parseDelete parses DELETE FROM table [WHERE expr]
func (p *Parser) parseDelete() (*DeleteStmt, error) {
	var stmt DeleteStmt

	_, err := p.expect(TokenDelete)
	if err != nil {
		return &stmt, err
	}
	_, err = p.expect(TokenFrom)
	if err != nil {
		return &stmt, err
	}
	tableToken, err := p.expect(TokenIdent)
	if err != nil {
		return &stmt, err
	}
	stmt.Table = tableToken.Literal

	if p.current.Type == TokenWhere {
		p.advance()
		stmt.Where, err = p.parseExpr()
		if err != nil {
			return &stmt, err
		}
	}

	return &stmt, nil
}

// parseCreate reads the CREATE keyword and dispatches to parseCreateTable or parseCreateIndex.
func (p *Parser) parseCreate() (Statement, error) {
	_, err := p.expect(TokenCreate)
	if err != nil {
		return nil, err
	}

	switch p.current.Type {
	case TokenTable:
		return p.parseCreateTable()
	case TokenUnique, TokenIndex:
		return p.parseCreateIndex()
	default:
		return nil, fmt.Errorf("expected TABLE or INDEX after CREATE, line: %d", p.lexer.line)
	}
}

// parseCreateTable parses CREATE TABLE name (col defs and foreign key constraints)
func (p *Parser) parseCreateTable() (*CreateTableStmt, error) {
	var stmt CreateTableStmt

	_, err := p.expect(TokenTable)
	if err != nil {
		return &stmt, err
	}
	nameToken, err := p.expect(TokenIdent)
	if err != nil {
		return &stmt, err
	}
	stmt.Table = nameToken.Literal

	_, err = p.expect(TokenLParen)
	if err != nil {
		return &stmt, err
	}

	for {
		if p.current.Type == TokenForeign {
			p.advance()
			_, err = p.expect(TokenKey)
			if err != nil {
				return &stmt, err
			}
			_, err = p.expect(TokenLParen)
			if err != nil {
				return &stmt, err
			}
			colToken, err := p.expect(TokenIdent)
			if err != nil {
				return &stmt, err
			}
			_, err = p.expect(TokenRParen)
			if err != nil {
				return &stmt, err
			}
			_, err = p.expect(TokenReferences)
			if err != nil {
				return &stmt, err
			}
			refTableToken, err := p.expect(TokenIdent)
			if err != nil {
				return &stmt, err
			}
			_, err = p.expect(TokenLParen)
			if err != nil {
				return &stmt, err
			}
			refColToken, err := p.expect(TokenIdent)
			if err != nil {
				return &stmt, err
			}
			_, err = p.expect(TokenRParen)
			if err != nil {
				return &stmt, err
			}
			fk := ForeignKeyAST{
				Column:    colToken.Literal,
				RefTable:  refTableToken.Literal,
				RefColumn: refColToken.Literal,
			}
			for p.current.Type == TokenOn {
				p.advance()
				if p.current.Type == TokenDelete {
					p.advance()
					fk.OnDelete = p.parseReferentialAction()
				} else if p.current.Type == TokenUpdate {
					p.advance()
					fk.OnUpdate = p.parseReferentialAction()
				} else {
					return &stmt, fmt.Errorf("expected DELETE or UPDATE after ON, line: %d", p.lexer.line)
				}
			}
			stmt.ForeignKeys = append(stmt.ForeignKeys, fk)
		} else {
			colNameToken, err := p.expect(TokenIdent)
			if err != nil {
				return &stmt, err
			}
			colTypeToken, err := p.expect(TokenIdent)
			if err != nil {
				return &stmt, err
			}
			col := ColumnDefAST{
				Name:     colNameToken.Literal,
				TypeName: colTypeToken.Literal,
			}
			if p.current.Type == TokenPrimary {
				p.advance()
				_, err = p.expect(TokenKey)
				if err != nil {
					return &stmt, err
				}
				col.PrimaryKey = true
			}
			if p.current.Type == TokenNot {
				p.advance()
				_, err = p.expect(TokenNull)
				if err != nil {
					return &stmt, err
				}
				col.NotNull = true
			}
			if p.current.Type == TokenDefault {
				p.advance()
				col.Default, err = p.parseExpr()
				if err != nil {
					return &stmt, err
				}
			}
			stmt.Columns = append(stmt.Columns, col)
		}

		if p.current.Type != TokenComma {
			break
		}
		p.advance()
	}

	_, err = p.expect(TokenRParen)
	if err != nil {
		return &stmt, err
	}

	return &stmt, nil
}

// parseReferentialAction reads a foreign key action: CASCADE, RESTRICT, or SET NULL.
func (p *Parser) parseReferentialAction() string {
	if p.current.Type == TokenSet {
		p.advance()
		if p.current.Type == TokenNull {
			p.advance()
		}
		return "SET NULL"
	}
	return p.advance().Literal
}

// parseCreateIndex parses CREATE [UNIQUE] INDEX name ON table (column)
func (p *Parser) parseCreateIndex() (*CreateIndexStmt, error) {
	var stmt CreateIndexStmt

	if p.current.Type == TokenUnique {
		stmt.Unique = true
		p.advance()
	}
	_, err := p.expect(TokenIndex)
	if err != nil {
		return &stmt, err
	}
	indexToken, err := p.expect(TokenIdent)
	if err != nil {
		return &stmt, err
	}
	stmt.Index = indexToken.Literal

	_, err = p.expect(TokenOn)
	if err != nil {
		return &stmt, err
	}
	tableToken, err := p.expect(TokenIdent)
	if err != nil {
		return &stmt, err
	}
	stmt.Table = tableToken.Literal

	_, err = p.expect(TokenLParen)
	if err != nil {
		return &stmt, err
	}
	colToken, err := p.expect(TokenIdent)
	if err != nil {
		return &stmt, err
	}
	stmt.Column = colToken.Literal

	_, err = p.expect(TokenRParen)
	if err != nil {
		return &stmt, err
	}

	return &stmt, nil
}

// parseDrop parses DROP TABLE name or DROP INDEX name.
func (p *Parser) parseDrop() (Statement, error) {
	_, err := p.expect(TokenDrop)
	if err != nil {
		return nil, err
	}

	switch p.current.Type {
	case TokenTable:
		p.advance()
		nameToken, err := p.expect(TokenIdent)
		if err != nil {
			return nil, err
		}
		return &DropTableStmt{Table: nameToken.Literal}, nil
	case TokenIndex:
		p.advance()
		nameToken, err := p.expect(TokenIdent)
		if err != nil {
			return nil, err
		}
		return &DropIndexStmt{Index: nameToken.Literal}, nil
	default:
		return nil, fmt.Errorf("expected TABLE or INDEX after DROP, line: %d", p.lexer.line)
	}
}

// parseAnalyze parses ANALYZE [table_name].
func (p *Parser) parseAnalyze() (*AnalyzeStmt, error) {
	var stmt AnalyzeStmt

	_, err := p.expect(TokenAnalyze)
	if err != nil {
		return &stmt, err
	}
	if p.current.Type == TokenIdent {
		stmt.Table = p.current.Literal
		p.advance()
	}

	return &stmt, nil
}

// parseExpr is the entry point for expression parsing.
func (p *Parser) parseExpr() (Expr, error) {
	return p.parseOr()
}

// parseOr parses one or more AND expressions joined by OR operators.
func (p *Parser) parseOr() (Expr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.current.Type == TokenOr {
		p.advance()
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Op: "OR", Right: right}
	}
	return left, nil
}

// parseAnd parses one or more NOT expressions joined by AND operators.
func (p *Parser) parseAnd() (Expr, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.current.Type == TokenAnd {
		p.advance()
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Op: "AND", Right: right}
	}
	return left, nil
}

// parseNot handles an optional leading NOT keyword, then delegates to parseComparison.
func (p *Parser) parseNot() (Expr, error) {
	if p.current.Type == TokenNot {
		p.advance()
		operand, err := p.parseComparison()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{Op: "NOT", Operand: operand}, nil
	}
	return p.parseComparison()
}

// parseComparison parses two additive expressions connected by a comparison operator.
func (p *Parser) parseComparison() (Expr, error) {
	left, err := p.parseAddSub()
	if err != nil {
		return nil, err
	}

	var op string
	switch p.current.Type {
	case TokenEq:
		op = "="
	case TokenNotEq:
		op = "!="
	case TokenLt:
		op = "<"
	case TokenGt:
		op = ">"
	case TokenLtEq:
		op = "<="
	case TokenGtEq:
		op = ">="
	case TokenLike:
		op = "LIKE"
	default:
		return left, nil
	}

	p.advance()
	right, err := p.parseAddSub()
	if err != nil {
		return nil, err
	}
	return &BinaryExpr{Left: left, Op: op, Right: right}, nil
}

// parseAddSub parses one or more multiplicative expressions joined by + or -.
func (p *Parser) parseAddSub() (Expr, error) {
	left, err := p.parseMulDiv()
	if err != nil {
		return nil, err
	}
	for p.current.Type == TokenPlus || p.current.Type == TokenMinus {
		op := "+"
		if p.current.Type == TokenMinus {
			op = "-"
		}
		p.advance()
		right, err := p.parseMulDiv()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Op: op, Right: right}
	}
	return left, nil
}

// parseMulDiv parses one or more unary expressions joined by *, /, or %.
func (p *Parser) parseMulDiv() (Expr, error) {
	left, err := p.parseUnary()
	if err != nil {
		return nil, err
	}
	for p.current.Type == TokenStar || p.current.Type == TokenSlash || p.current.Type == TokenPercent {
		var op string
		switch p.current.Type {
		case TokenStar:
			op = "*"
		case TokenSlash:
			op = "/"
		case TokenPercent:
			op = "%"
		}
		p.advance()
		right, err := p.parseUnary()
		if err != nil {
			return nil, err
		}
		left = &BinaryExpr{Left: left, Op: op, Right: right}
	}
	return left, nil
}

// parseUnary handles an optional leading minus sign and then delegates to parsePrimary.
func (p *Parser) parseUnary() (Expr, error) {
	if p.current.Type == TokenMinus {
		p.advance()
		operand, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return &UnaryExpr{Op: "-", Operand: operand}, nil
	}
	return p.parsePrimary()
}

// parsePrimary handles literals, NULL, star, function calls, column refs, and grouped expressions.
func (p *Parser) parsePrimary() (Expr, error) {
	switch p.current.Type {
	case TokenInteger:
		token := p.advance()
		return &Literal{Kind: "integer", Value: token.Literal}, nil
	case TokenFloat:
		token := p.advance()
		return &Literal{Kind: "float", Value: token.Literal}, nil
	case TokenString:
		token := p.advance()
		return &Literal{Kind: "string", Value: token.Literal}, nil
	case TokenNull:
		p.advance()
		return &Literal{Kind: "null", Value: "NULL"}, nil
	case TokenStar:
		p.advance()
		return &StarExpr{}, nil
	case TokenLParen:
		p.advance()
		expression, err := p.parseExpr()
		if err != nil {
			return nil, err
		}
		_, err = p.expect(TokenRParen)
		if err != nil {
			return nil, err
		}
		return expression, nil
	case TokenCount, TokenSum, TokenMin, TokenMax, TokenAvg:
		nameToken := p.advance()
		_, err := p.expect(TokenLParen)
		if err != nil {
			return nil, err
		}
		call := &FuncCall{Name: nameToken.Literal}
		if p.current.Type == TokenStar {
			call.Star = true
			p.advance()
		} else if p.current.Type != TokenRParen {
			for {
				arg, err := p.parseExpr()
				if err != nil {
					return nil, err
				}
				call.Args = append(call.Args, arg)
				if p.current.Type != TokenComma {
					break
				}
				p.advance()
			}
		}
		_, err = p.expect(TokenRParen)
		if err != nil {
			return nil, err
		}
		return call, nil
	case TokenIdent:
		nameToken := p.advance()
		if p.current.Type == TokenLParen {
			// function call: name(args) or name(*)
			p.advance()
			call := &FuncCall{Name: nameToken.Literal}
			if p.current.Type == TokenStar {
				call.Star = true
				p.advance()
			} else if p.current.Type != TokenRParen {
				for {
					arg, err := p.parseExpr()
					if err != nil {
						return nil, err
					}
					call.Args = append(call.Args, arg)
					if p.current.Type != TokenComma {
						break
					}
					p.advance()
				}
			}
			_, err := p.expect(TokenRParen)
			if err != nil {
				return nil, err
			}
			return call, nil
		}
		// column ref: name or table.name
		ref := &ColumnRef{Column: nameToken.Literal}
		if p.current.Type == TokenDot {
			p.advance()
			colToken, err := p.expect(TokenIdent)
			if err != nil {
				return nil, err
			}
			ref.Table = nameToken.Literal
			ref.Column = colToken.Literal
		}
		return ref, nil
	default:
		return nil, fmt.Errorf("unexpected token '%s' in expression, line: %d", p.current.Literal, p.lexer.line)
	}
}

// advance shifts the current token to the previous peek token, reads the next token from
// the lexer into peek, and returns the token that was just consumed.
func (p *Parser) advance() Token {
	old := p.current
	p.current = p.peek
	p.peek = p.lexer.NextToken()

	return old
}

// expect asserts that the current token matches the expected type, advances past it, and
// returns the consumed token. If the type does not match, it returns a parse error that
// includes the current line number to aid debugging.
func (p *Parser) expect(t TokenType) (Token, error) {
	if p.current.Type != t {
		return Token{}, fmt.Errorf("Expected token type not found, line: %v", p.lexer.line)
	}

	return p.advance(), nil
}
