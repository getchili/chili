package lexer

import (
	"strings"
	"testing"

	"github.com/getchili/chili/compiler/token"
)

// kinds résume une suite de tokens sous forme de chaîne lisible.
func kinds(src string) string {
	var parts []string
	for _, t := range Tokenize([]byte(src)) {
		parts = append(parts, t.String())
	}
	return strings.Join(parts, " ")
}

func TestTokens(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{"", "EOF"},
		{"let x = 42", "let IDENT(x) = INT(42) ; EOF"},
		{"fn main() {}", "fn IDENT(main) ( ) { } ; EOF"},
		{"a+=b-=c*=d/=e%=f", "IDENT(a) += IDENT(b) -= IDENT(c) *= IDENT(d) /= IDENT(e) %= IDENT(f) ; EOF"},
		{"a && b || !c", "IDENT(a) && IDENT(b) || ! IDENT(c) ; EOF"},
		{"a & b | c ^ ~d", "IDENT(a) & IDENT(b) | IDENT(c) ^ ~ IDENT(d) ; EOF"},
		{"a << 2 >> 1", "IDENT(a) << INT(2) >> INT(1) ; EOF"},
		{"a == b != c < d <= e > f >= g", "IDENT(a) == IDENT(b) != IDENT(c) < IDENT(d) <= IDENT(e) > IDENT(f) >= IDENT(g) ; EOF"},
		{"gpio.write(2, 1)", "IDENT(gpio) . IDENT(write) ( INT(2) , INT(1) ) ; EOF"},
		{"0x2A 0b1010 1_000", "INT(0x2A) INT(0b1010) INT(1_000) ; EOF"},
		{"if else while return break continue true false",
			"if else while return break continue true false ; EOF"},
		{"_privé", "IDENT(_priv) ILLEGAL(caractère inattendu 0xc3) ILLEGAL(caractère inattendu 0xa9) EOF"},
	}
	for _, tt := range tests {
		if got := kinds(tt.src); got != tt.want {
			t.Errorf("%q\n got: %s\nwant: %s", tt.src, got, tt.want)
		}
	}
}

func TestAutoSemicolon(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		// Pas de ';' après un opérateur ou un '{' en fin de ligne.
		{"x = 1 +\n2\n", "IDENT(x) = INT(1) + INT(2) ; EOF"},
		{"if x {\n}\n", "if IDENT(x) { } ; EOF"},
		// ';' après ident, littéral, ')', '}', return, break, continue, true, false.
		{"return\nbreak\ncontinue\ntrue\nf()\n", "return ; break ; continue ; true ; IDENT(f) ( ) ; EOF"},
		// Lignes vides et commentaires n'ajoutent rien.
		{"a\n\n// commentaire\n\nb // fin\n", "IDENT(a) ; IDENT(b) ; EOF"},
		// ';' explicite : pas de doublon en fin de ligne.
		{"a; b;\n", "IDENT(a) ; IDENT(b) ; EOF"},
		{"a\r\nb", "IDENT(a) ; IDENT(b) ; EOF"},
	}
	for _, tt := range tests {
		if got := kinds(tt.src); got != tt.want {
			t.Errorf("%q\n got: %s\nwant: %s", tt.src, got, tt.want)
		}
	}
}

func TestBadNumbers(t *testing.T) {
	for _, src := range []string{"12ab", "0x", "0b102", "0xZZ", "0b_"} {
		toks := Tokenize([]byte(src))
		if toks[0].Kind != token.ILLEGAL {
			t.Errorf("%q : ILLEGAL attendu, obtenu %s", src, toks[0])
		}
	}
}

func TestPositions(t *testing.T) {
	toks := Tokenize([]byte("let x\n  = 5"))
	pos := func(l, c int) token.Pos { return token.Pos{Line: l, Col: c} }
	want := []token.Pos{pos(1, 1), pos(1, 5), pos(1, 6), pos(2, 3), pos(2, 5)}
	for i, p := range want {
		if toks[i].Pos != p {
			t.Errorf("token %d (%s) : position %s, attendu %s", i, toks[i], toks[i].Pos, p)
		}
	}
}
