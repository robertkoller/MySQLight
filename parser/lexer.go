package parser

import "strings"

type TokenType int

const (
	// Literals
	TokenEOF TokenType = iota
	TokenIdent
	TokenInteger
	TokenFloat
	TokenString

	// Keywords
	TokenSelect
	TokenFrom
	TokenWhere
	TokenInsert
	TokenInto
	TokenValues
	TokenUpdate
	TokenSet
	TokenDelete
	TokenCreate
	TokenTable
	TokenDrop
	TokenIndex
	TokenOn
	TokenPrimary
	TokenKey
	TokenNot
	TokenNull
	TokenDefault
	TokenBegin
	TokenCommit
	TokenRollback
	TokenAnd
	TokenOr
	TokenLike
	TokenOrder
	TokenBy
	TokenLimit
	TokenJoin
	TokenInner
	TokenLeft
	TokenGroup
	TokenHaving
	TokenCount
	TokenSum
	TokenMin
	TokenMax
	TokenAvg
	TokenReferences
	TokenForeign
	TokenUnique
	TokenAnalyze
	TokenDesc

	// Punctuation
	TokenLParen
	TokenRParen
	TokenComma
	TokenSemicolon
	TokenDot
	TokenStar
	TokenEq
	TokenLt
	TokenGt
	TokenLtEq
	TokenGtEq
	TokenNotEq
	TokenPlus
	TokenMinus
	TokenSlash
	TokenPercent
	TokenErr
)

type Token struct {
	Type    TokenType
	Literal string // raw text from the input
	Line    int
}

type Lexer struct {
	input    []rune // the full SQL string as runes (handles unicode safely)
	position int    // current read position
	line     int    // current line number (for error messages)
}

// NewLexer converts the input SQL string to a slice of runes for safe Unicode handling
// and initialises the lexer at position zero on line one.
func NewLexer(input string) *Lexer {
	return &Lexer{
		input:    []rune(input),
		position: 0,
		line:     1,
	}
}

// NextToken advances through the input and returns the next token. It first skips whitespace,
// incrementing the line counter on newlines. It then inspects the current rune: digits produce
// integer or float literals, letters and underscores produce identifiers that are checked
// against the keyword table, single quotes delimit string literals, and punctuation characters
// produce operator or delimiter tokens. At the end of input, TokenEOF is returned.
func (l *Lexer) NextToken() Token {
	// 1. skip whitespace, counting newlines
	for l.position < len(l.input) {
		character := l.input[l.position]
		if character == '\n' {
			l.line++
			l.position++
		} else if character == ' ' || character == '\t' || character == '\r' {
			l.position++
		} else {
			break
		}
	}

	if l.position >= len(l.input) {
		return Token{
			Type: TokenEOF,
			Line: l.line,
		}
	}

	character := l.input[l.position]

	switch {
	case isDigit(character):
		return l.readNumber()
	case isLetter(character) || character == '_':
		return l.readIdent()
	case character == '\'':
		return l.readString()
	default:
		return l.readOperator()
	}
}

// readOperator handles punctuation and operator tokens. Single-character tokens are
// returned directly. For !, <, and >, it peeks one character ahead to check for a
// two-character operator (!=, <=, >=). Unknown characters return TokenErr.
func (l *Lexer) readOperator() Token {
	start := l.position
	character := l.input[l.position]

	var types TokenType

	switch character {
	case '(':
		types = TokenLParen
	case ')':
		types = TokenRParen
	case ',':
		types = TokenComma
	case ';':
		types = TokenSemicolon
	case '+':
		types = TokenPlus
	case '-':
		types = TokenMinus
	case '*':
		types = TokenStar
	case '.':
		types = TokenDot
	case '/':
		types = TokenSlash
	case '=':
		types = TokenEq
	case '!':
		if l.position+1 >= len(l.input) || l.input[l.position+1] != '=' {
			types = TokenErr
		} else {
			l.position++
			types = TokenNotEq
		}
	case '%':
		types = TokenPercent
	case '>':
		if l.position+1 >= len(l.input) || l.input[l.position+1] != '=' {
			types = TokenGt
		} else {
			l.position++
			types = TokenGtEq
		}
	case '<':
		if l.position+1 >= len(l.input) || l.input[l.position+1] != '=' {
			types = TokenLt
		} else {
			l.position++
			types = TokenLtEq
		}
	default:
		types = TokenErr
	}

	l.position++
	return Token{
		Type: types, Literal: string(l.input[start:l.position]), Line: l.line,
	}
}

// readString reads a single-quoted string literal, returning the inner text as TokenString.
// The opening quote is consumed before start is recorded. If the input ends before a
// closing quote is found, TokenErr is returned with whatever was read.
func (l *Lexer) readString() Token {
	inputLen := len(l.input)
	l.position++ // we are calling this method when the character is ' so we skip
	start := l.position

	for {
		if l.position >= inputLen { // this is here because if the string doesnt end with a ' we error back
			output := string(l.input[start:inputLen])
			return Token{Type: TokenErr, Literal: output, Line: l.line}
		}

		character := l.input[l.position]

		if character == '\'' {
			var output string
			if !(l.position == start) {
				output = string(l.input[start:l.position])
			}

			l.position++
			return Token{
				Type: TokenString, Literal: output, Line: l.line,
			}
		}
		l.position++
	}
}

// readIdent reads a word token (letters, digits, underscores). The result is lowercased
// and looked up in the keywords map; if found, the keyword token type is returned with
// the original-case literal. Otherwise TokenIdent is returned.
func (l *Lexer) readIdent() Token {
	start := l.position
	inputLen := len(l.input)

	for l.position < inputLen && (isLetter(l.input[l.position]) || isDigit(l.input[l.position]) || l.input[l.position] == '_') {
		l.position++
	}

	output := string(l.input[start:l.position])
	word := strings.ToLower(output)
	keyWord, exists := keywords[word]
	if exists {
		return Token{
			Type: keyWord, Literal: output, Line: l.line,
		}
	} else {
		return Token{Type: TokenIdent, Literal: output, Line: l.line}
	}
}

// readNumber reads an integer or float literal. A single dot switches to float mode;
// a second dot returns TokenErr. Any non-digit, non-dot character terminates the number
// without being consumed, leaving it for the next NextToken call.
func (l *Lexer) readNumber() Token {
	var float bool
	start := l.position
	inputLen := len(l.input)

	for {
		if l.position >= inputLen {
			var types TokenType
			if float {
				types = TokenFloat
			} else {
				types = TokenInteger
			}
			output := string(l.input[start:l.position])
			return Token{Type: types,
				Literal: output,
				Line:    l.line,
			}
		}
		character := l.input[l.position]

		if character == '.' {
			if float {
				l.position++
				output := string(l.input[start:l.position])
				return Token{Type: TokenErr,
					Literal: output,
					Line:    l.line,
				}
			} else {
				l.position++
				float = true
				continue
			}
		}

		if !isDigit(character) {
			var types TokenType
			if float {
				types = TokenFloat
			} else {
				types = TokenInteger
			}

			output := string(l.input[start:l.position])
			return Token{Type: types,
				Literal: output,
				Line:    l.line,
			}

		}

		l.position++
	}
}

// checks if a rune is a digit
func isDigit(character rune) bool {
	if character >= '0' && character <= '9' {
		return true
	}
	return false

}

// checks if a rune is a letter
func isLetter(character rune) bool {
	if (character >= 'a' && character <= 'z') || (character >= 'A' && character <= 'Z') {
		return true
	}
	return false
}

// this is the sql keywords matched with their respective token
var keywords = map[string]TokenType{
	"select":     TokenSelect,
	"from":       TokenFrom,
	"where":      TokenWhere,
	"insert":     TokenInsert,
	"into":       TokenInto,
	"values":     TokenValues,
	"update":     TokenUpdate,
	"set":        TokenSet,
	"delete":     TokenDelete,
	"create":     TokenCreate,
	"table":      TokenTable,
	"drop":       TokenDrop,
	"index":      TokenIndex,
	"on":         TokenOn,
	"primary":    TokenPrimary,
	"key":        TokenKey,
	"not":        TokenNot,
	"null":       TokenNull,
	"default":    TokenDefault,
	"begin":      TokenBegin,
	"commit":     TokenCommit,
	"rollback":   TokenRollback,
	"and":        TokenAnd,
	"or":         TokenOr,
	"like":       TokenLike,
	"order":      TokenOrder,
	"by":         TokenBy,
	"limit":      TokenLimit,
	"join":       TokenJoin,
	"inner":      TokenInner,
	"left":       TokenLeft,
	"group":      TokenGroup,
	"having":     TokenHaving,
	"count":      TokenCount,
	"sum":        TokenSum,
	"min":        TokenMin,
	"max":        TokenMax,
	"avg":        TokenAvg,
	"references": TokenReferences,
	"foreign":    TokenForeign,
	"unique":     TokenUnique,
	"analyze":    TokenAnalyze,
	"desc":       TokenDesc,
}
