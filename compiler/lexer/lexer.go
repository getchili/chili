// Package lexer découpe un source Chili en tokens.
//
// Comme en Go, un ';' est inséré automatiquement en fin de ligne quand le
// dernier token peut terminer une instruction (voir docs/spec.md §2).
package lexer

import (
	"fmt"
	"unicode/utf8"

	"github.com/getchili/chili/compiler/token"
)

type Lexer struct {
	src  []byte
	off  int // position du prochain octet à lire
	line int
	col  int

	insertSemi bool // vrai si un retour à la ligne doit produire un ';'
}

func New(src []byte) *Lexer {
	// Marque d'ordre UTF-8 (BOM) : ajoutée par certains éditeurs Windows, on l'ignore.
	if len(src) >= 3 && src[0] == 0xEF && src[1] == 0xBB && src[2] == 0xBF {
		src = src[3:]
	}
	return &Lexer{src: src, line: 1, col: 1}
}

// Tokenize lit tout le source. Pratique pour les tests et le débogage.
func Tokenize(src []byte) []token.Token {
	lx := New(src)
	var toks []token.Token
	for {
		t := lx.Next()
		toks = append(toks, t)
		if t.Kind == token.EOF {
			return toks
		}
	}
}

func (lx *Lexer) peek(n int) byte {
	if lx.off+n < len(lx.src) {
		return lx.src[lx.off+n]
	}
	return 0
}

func (lx *Lexer) advance() byte {
	c := lx.src[lx.off]
	lx.off++
	if c == '\n' {
		lx.line++
		lx.col = 1
	} else {
		lx.col++
	}
	return c
}

func (lx *Lexer) pos() token.Pos { return token.Pos{Line: lx.line, Col: lx.col} }

// Next renvoie le token suivant. Après EOF, renvoie EOF indéfiniment.
func (lx *Lexer) Next() token.Token {
	if semi, ok := lx.skipSpace(); ok {
		return semi
	}
	if lx.off >= len(lx.src) {
		// Fin de fichier : on ferme la dernière instruction si besoin.
		if lx.insertSemi {
			lx.insertSemi = false
			return token.Token{Kind: token.SEMICOLON, Lit: "\n", Pos: lx.pos()}
		}
		return token.Token{Kind: token.EOF, Pos: lx.pos()}
	}

	p := lx.pos()
	c := lx.src[lx.off]

	switch {
	case isLetter(c):
		start := lx.off
		for lx.off < len(lx.src) && (isLetter(lx.src[lx.off]) || isDigit(lx.src[lx.off])) {
			lx.advance()
		}
		lit := string(lx.src[start:lx.off])
		kind := token.LookupIdent(lit)
		switch kind {
		case token.IDENT, token.TRUE, token.FALSE, token.RETURN, token.BREAK, token.CONTINUE,
			token.THIS, token.SUPER:
			lx.insertSemi = true
		default:
			lx.insertSemi = false
		}
		return token.Token{Kind: kind, Lit: lit, Pos: p}

	case isDigit(c):
		return lx.number(p)

	case c == '"':
		return lx.str(p)
	}

	lx.advance()
	kind := token.ILLEGAL
	switch c {
	case '+':
		kind = lx.pick('=', token.ADD_ASSIGN, token.PLUS)
	case '-':
		kind = lx.pick('=', token.SUB_ASSIGN, token.MINUS)
	case '*':
		kind = lx.pick('=', token.MUL_ASSIGN, token.STAR)
	case '/':
		kind = lx.pick('=', token.DIV_ASSIGN, token.SLASH)
	case '%':
		kind = lx.pick('=', token.MOD_ASSIGN, token.PERCENT)
	case '&':
		kind = lx.pick('&', token.ANDAND, token.AMP)
	case '|':
		kind = lx.pick('|', token.OROR, token.PIPE)
	case '^':
		kind = token.CARET
	case '~':
		kind = token.TILDE
	case '!':
		kind = lx.pick('=', token.NE, token.BANG)
	case '=':
		kind = lx.pick('=', token.EQ, token.ASSIGN)
	case '<':
		if lx.peek(0) == '<' {
			lx.advance()
			kind = token.SHL
		} else {
			kind = lx.pick('=', token.LE, token.LT)
		}
	case '>':
		if lx.peek(0) == '>' {
			lx.advance()
			kind = token.SHR
		} else {
			kind = lx.pick('=', token.GE, token.GT)
		}
	case '.':
		kind = lx.pick('.', token.DOTDOT, token.DOT)
	case '(':
		kind = token.LPAREN
	case ')':
		kind = token.RPAREN
	case '{':
		kind = token.LBRACE
	case '}':
		kind = token.RBRACE
	case '[':
		kind = token.LBRACKET
	case ']':
		kind = token.RBRACKET
	case ',':
		kind = token.COMMA
	case ';':
		kind = token.SEMICOLON
	}

	lx.insertSemi = kind == token.RPAREN || kind == token.RBRACE || kind == token.RBRACKET
	if kind == token.ILLEGAL {
		return token.Token{Kind: token.ILLEGAL, Lit: "caractère inattendu " + quoteByte(c), Pos: p}
	}
	return token.Token{Kind: kind, Lit: string(lx.src[lx.off-len(kind.String()) : lx.off]), Pos: p}
}

// skipSpace saute espaces et commentaires. Si un retour à la ligne doit
// produire un ';' implicite, il le renvoie avec ok = true.
func (lx *Lexer) skipSpace() (semi token.Token, ok bool) {
	for lx.off < len(lx.src) {
		c := lx.src[lx.off]
		switch {
		case c == '\n':
			if lx.insertSemi {
				p := lx.pos()
				lx.advance()
				lx.insertSemi = false
				return token.Token{Kind: token.SEMICOLON, Lit: "\n", Pos: p}, true
			}
			lx.advance()
		case c == ' ' || c == '\t' || c == '\r':
			lx.advance()
		case c == '/' && lx.peek(1) == '/':
			for lx.off < len(lx.src) && lx.src[lx.off] != '\n' {
				lx.advance()
			}
		default:
			return token.Token{}, false
		}
	}
	return token.Token{}, false
}

// pick consomme next s'il suit et renvoie yes, sinon renvoie no.
func (lx *Lexer) pick(next byte, yes, no token.Kind) token.Kind {
	if lx.peek(0) == next {
		lx.advance()
		return yes
	}
	return no
}

// number lit un entier décimal, hexadécimal (0x) ou binaire (0b).
// La conversion en valeur est faite par le parser.
func (lx *Lexer) number(p token.Pos) token.Token {
	start := lx.off
	valid := isDigit
	if lx.src[lx.off] == '0' && (lx.peek(1) == 'x' || lx.peek(1) == 'X') {
		lx.advance()
		lx.advance()
		valid = isHex
	} else if lx.src[lx.off] == '0' && (lx.peek(1) == 'b' || lx.peek(1) == 'B') {
		lx.advance()
		lx.advance()
		valid = func(c byte) bool { return c == '0' || c == '1' }
	}
	digits := 0
	for lx.off < len(lx.src) {
		c := lx.src[lx.off]
		if valid(c) {
			digits++
		} else if c != '_' {
			break
		}
		lx.advance()
	}
	// Un nombre collé à une lettre ou un chiffre hors base (12ab, 0b102) est invalide.
	bad := digits == 0
	for lx.off < len(lx.src) && (isLetter(lx.src[lx.off]) || isDigit(lx.src[lx.off])) {
		lx.advance()
		bad = true
	}
	lit := string(lx.src[start:lx.off])
	lx.insertSemi = true
	if bad {
		return token.Token{Kind: token.ILLEGAL, Lit: "nombre invalide " + lit, Pos: p}
	}
	return token.Token{Kind: token.INT, Lit: lit, Pos: p}
}

// MaxString est la longueur maximale d'une chaîne constante (u16 dans le bytecode).
const MaxString = 65535

// str lit une chaîne "…" et décode ses échappements (cf. docs/strings.md §3).
// La valeur décodée est placée dans Lit.
func (lx *Lexer) str(p token.Pos) token.Token {
	lx.advance() // "
	lx.insertSemi = true
	var out []byte
	var bad string // première erreur rencontrée
	for {
		if lx.off >= len(lx.src) || lx.src[lx.off] == '\n' {
			return token.Token{Kind: token.ILLEGAL, Lit: "chaîne non terminée : il manque le \" final", Pos: p}
		}
		c := lx.advance()
		if c == '"' {
			break
		}
		if c != '\\' {
			out = append(out, c)
			continue
		}
		escPos := lx.col - 1
		if lx.off >= len(lx.src) || lx.src[lx.off] == '\n' {
			continue // signalé comme chaîne non terminée au tour suivant
		}
		e := lx.advance()
		switch e {
		case 'n':
			out = append(out, '\n')
		case 't':
			out = append(out, '\t')
		case 'r':
			out = append(out, '\r')
		case '0':
			out = append(out, 0)
		case '"', '\\':
			out = append(out, e)
		case 'x':
			if isHex(lx.peek(0)) && isHex(lx.peek(1)) {
				out = append(out, hexVal(lx.advance())<<4|hexVal(lx.advance()))
			} else if bad == "" {
				bad = fmt.Sprintf("échappement \\x invalide (colonne %d) : il faut 2 chiffres hexadécimaux, ex. \\x41", escPos)
			}
		case 'u':
			r, ok := lx.unicodeEscape()
			if ok {
				out = utf8.AppendRune(out, r)
			} else if bad == "" {
				bad = fmt.Sprintf("échappement \\u invalide (colonne %d) : écris \\u{e9} avec un code Unicode valide", escPos)
			}
		default:
			if bad == "" {
				bad = fmt.Sprintf("échappement inconnu \\%c (colonne %d) : utilise \\n \\t \\r \\0 \\\" \\\\ \\xHH ou \\u{…}", e, escPos)
			}
		}
	}
	if bad != "" {
		return token.Token{Kind: token.ILLEGAL, Lit: bad, Pos: p}
	}
	if len(out) > MaxString {
		return token.Token{Kind: token.ILLEGAL, Lit: fmt.Sprintf("chaîne trop longue (%d octets, max %d)", len(out), MaxString), Pos: p}
	}
	return token.Token{Kind: token.STRING, Lit: string(out), Pos: p}
}

// unicodeEscape lit « {H…} » après \u.
func (lx *Lexer) unicodeEscape() (rune, bool) {
	if lx.peek(0) != '{' {
		return 0, false
	}
	lx.advance()
	v, n := 0, 0
	for lx.off < len(lx.src) && isHex(lx.src[lx.off]) {
		v = v*16 + int(hexVal(lx.advance()))
		n++
		if n > 6 {
			return 0, false
		}
	}
	if n == 0 || lx.peek(0) != '}' {
		return 0, false
	}
	lx.advance()
	if v > 0x10FFFF || (v >= 0xD800 && v <= 0xDFFF) {
		return 0, false
	}
	return rune(v), true
}

func hexVal(c byte) byte {
	switch {
	case c >= 'a':
		return c - 'a' + 10
	case c >= 'A':
		return c - 'A' + 10
	}
	return c - '0'
}

func isLetter(c byte) bool { return c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') }
func isDigit(c byte) bool  { return '0' <= c && c <= '9' }
func isHex(c byte) bool {
	return isDigit(c) || ('a' <= c && c <= 'f') || ('A' <= c && c <= 'F')
}

func quoteByte(c byte) string {
	if c >= 0x20 && c < 0x7f {
		return "'" + string(c) + "'"
	}
	const hex = "0123456789abcdef"
	return "0x" + string(hex[c>>4]) + string(hex[c&0xf])
}
