// Package token définit les tokens produits par le lexer Chili.
package token

import "fmt"

type Kind int

const (
	ILLEGAL Kind = iota
	EOF

	IDENT
	INT
	STRING

	// Mots-clés
	FUNCTION
	CLASS
	ABSTRACT
	EXTENDS
	CONSTRUCTOR
	THIS
	SUPER
	NEW
	LET
	IF
	ELSE
	WHILE
	FOR
	IN
	RETURN
	BREAK
	CONTINUE
	TRUE
	FALSE

	// Opérateurs
	PLUS    // +
	MINUS   // -
	STAR    // *
	SLASH   // /
	PERCENT // %
	AMP     // &
	PIPE    // |
	CARET   // ^
	TILDE   // ~
	SHL     // <<
	SHR     // >>
	BANG    // !
	ANDAND  // &&
	OROR    // ||
	EQ      // ==
	NE      // !=
	LT      // <
	LE      // <=
	GT      // >
	GE      // >=

	ASSIGN     // =
	ADD_ASSIGN // +=
	SUB_ASSIGN // -=
	MUL_ASSIGN // *=
	DIV_ASSIGN // /=
	MOD_ASSIGN // %=

	DOT    // .
	DOTDOT // ..

	// Délimiteurs
	LPAREN    // (
	RPAREN    // )
	LBRACE    // {
	RBRACE    // }
	LBRACKET  // [
	RBRACKET  // ]
	COMMA     // ,
	SEMICOLON // ; (explicite ou inséré automatiquement)
)

var names = [...]string{
	ILLEGAL: "ILLEGAL",
	EOF:     "EOF",
	IDENT:   "IDENT",
	INT:     "INT",
	STRING:  "STRING",

	FUNCTION:    "function",
	CLASS:       "class",
	ABSTRACT:    "abstract",
	EXTENDS:     "extends",
	CONSTRUCTOR: "constructor",
	THIS:        "this",
	SUPER:       "super",
	NEW:         "new",
	LET:         "let",
	IF:          "if",
	ELSE:        "else",
	WHILE:       "while",
	FOR:         "for",
	IN:          "in",
	RETURN:      "return",
	BREAK:       "break",
	CONTINUE:    "continue",
	TRUE:        "true",
	FALSE:       "false",

	PLUS:    "+",
	MINUS:   "-",
	STAR:    "*",
	SLASH:   "/",
	PERCENT: "%",
	AMP:     "&",
	PIPE:    "|",
	CARET:   "^",
	TILDE:   "~",
	SHL:     "<<",
	SHR:     ">>",
	BANG:    "!",
	ANDAND:  "&&",
	OROR:    "||",
	EQ:      "==",
	NE:      "!=",
	LT:      "<",
	LE:      "<=",
	GT:      ">",
	GE:      ">=",

	ASSIGN:     "=",
	ADD_ASSIGN: "+=",
	SUB_ASSIGN: "-=",
	MUL_ASSIGN: "*=",
	DIV_ASSIGN: "/=",
	MOD_ASSIGN: "%=",

	DOT:    ".",
	DOTDOT: "..",

	LPAREN:    "(",
	RPAREN:    ")",
	LBRACE:    "{",
	RBRACE:    "}",
	LBRACKET:  "[",
	RBRACKET:  "]",
	COMMA:     ",",
	SEMICOLON: ";",
}

func (k Kind) String() string {
	if int(k) < len(names) && names[k] != "" {
		return names[k]
	}
	return fmt.Sprintf("Kind(%d)", int(k))
}

var keywords = map[string]Kind{
	"function":    FUNCTION,
	"class":       CLASS,
	"abstract":    ABSTRACT,
	"extends":     EXTENDS,
	"constructor": CONSTRUCTOR,
	"this":        THIS,
	"super":       SUPER,
	"new":         NEW,
	"let":         LET,
	"if":          IF,
	"else":        ELSE,
	"while":       WHILE,
	"for":         FOR,
	"in":          IN,
	"return":      RETURN,
	"break":       BREAK,
	"continue":    CONTINUE,
	"true":        TRUE,
	"false":       FALSE,
}

// LookupIdent renvoie le mot-clé correspondant à ident, ou IDENT.
func LookupIdent(ident string) Kind {
	if k, ok := keywords[ident]; ok {
		return k
	}
	return IDENT
}

// Pos est une position dans le source (1-indexée).
type Pos struct {
	Line int
	Col  int
}

func (p Pos) String() string { return fmt.Sprintf("%d:%d", p.Line, p.Col) }

type Token struct {
	Kind Kind
	Lit  string // texte source (identifiants, entiers, "\n" pour un ';' implicite)
	Pos  Pos
}

func (t Token) String() string {
	switch t.Kind {
	case IDENT, INT, ILLEGAL:
		return fmt.Sprintf("%s(%s)", t.Kind, t.Lit)
	case STRING:
		return fmt.Sprintf("STRING(%q)", t.Lit)
	}
	return t.Kind.String()
}
