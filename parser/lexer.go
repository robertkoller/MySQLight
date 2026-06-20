package parser

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
)

type Token struct {
	Type    TokenType
	Literal string // raw text from the input
	Line    int
}

type Lexer struct {
	// TODO: input []rune  — the full SQL string as runes (handles unicode safely)
	// TODO: pos   int     — current read position
	// TODO: line  int     — current line number (for error messages)
}

// NewLexer converts the input SQL string to a slice of runes for safe Unicode handling
// and initialises the lexer at position zero on line one.
func NewLexer(input string) *Lexer {
	// TODO: convert input to []rune, initialise pos=0, line=1
	panic("not implemented")
}

// NextToken advances through the input and returns the next token. It first skips whitespace,
// incrementing the line counter on newlines. It then inspects the current rune: digits produce
// integer or float literals, letters and underscores produce identifiers that are checked
// against the keyword table, single quotes delimit string literals, and punctuation characters
// produce operator or delimiter tokens. At the end of input, TokenEOF is returned.
func (l *Lexer) NextToken() Token {
	// TODO: skip whitespace (and increment l.line on '\n')
	// TODO: peek at the current rune and branch:
	//   digit        → readNumber (integer or float)
	//   letter or _  → readIdent, then check keyword table to set correct TokenType
	//   '            → readString (single-quoted)
	//   punctuation  → emit the matching Token directly
	//   0            → emit TokenEOF
	panic("not implemented")
}

// keywords maps lowercase keyword strings to their TokenType.
// TODO: populate this map for every keyword constant above.
var keywords = map[string]TokenType{
	"select": TokenSelect,
	// TODO: fill in the rest
}
