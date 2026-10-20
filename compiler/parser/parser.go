// Package parser construit l'AST Chili par descente récursive.
//
// En cas d'erreur, le parser note l'erreur, se resynchronise sur la fin de
// l'instruction courante et continue, pour signaler plusieurs erreurs d'un coup.
package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/getchili/chili/compiler/ast"
	"github.com/getchili/chili/compiler/diag"
	"github.com/getchili/chili/compiler/lexer"
	"github.com/getchili/chili/compiler/natives"
	"github.com/getchili/chili/compiler/token"
)

const maxErrors = 10

type (
	Error     = diag.Error
	ErrorList = diag.ErrorList
)

// bailout est levé (panic) pour abandonner l'instruction en cours.
type bailout struct{}

type Parser struct {
	lx   *lexer.Lexer
	tok  token.Token
	errs ErrorList
}

// Parse analyse un fichier source complet. err est une ErrorList si non nil.
func Parse(src []byte) (prog *ast.Program, err error) {
	p := &Parser{lx: lexer.New(src)}
	p.next()
	prog = p.parseProgram()
	if len(p.errs) > 0 {
		return prog, p.errs
	}
	return prog, nil
}

// ParseExpr analyse une expression seule (utile pour les tests).
func ParseExpr(src string) (ast.Expr, error) {
	p := &Parser{lx: lexer.New([]byte(src))}
	p.next()
	var e ast.Expr
	func() {
		defer p.recoverBailout()
		e = p.parseExpr()
		if p.tok.Kind == token.SEMICOLON && p.tok.Lit == "\n" {
			p.next()
		}
		if p.tok.Kind != token.EOF {
			p.errorf(p.tok.Pos, "fin d'expression attendue, trouvé %s", describe(p.tok))
		}
	}()
	if len(p.errs) > 0 {
		return e, p.errs
	}
	return e, nil
}

// ---------------------------------------------------------------------------
// Utilitaires

func (p *Parser) next() {
	p.tok = p.lx.Next()
	// Les erreurs lexicales sont signalées ici et le token est ignoré.
	for p.tok.Kind == token.ILLEGAL {
		p.addError(p.tok.Pos, p.tok.Lit)
		p.tok = p.lx.Next()
	}
}

func (p *Parser) addError(pos token.Pos, msg string) {
	if len(p.errs) >= maxErrors {
		return
	}
	// Une seule erreur par ligne : les suivantes sont presque toujours en cascade.
	if n := len(p.errs); n > 0 && p.errs[n-1].Pos.Line == pos.Line {
		return
	}
	p.errs = append(p.errs, Error{Pos: pos, Msg: msg})
}

// errorf enregistre une erreur et abandonne l'instruction courante.
func (p *Parser) errorf(pos token.Pos, format string, args ...any) {
	p.addError(pos, fmt.Sprintf(format, args...))
	panic(bailout{})
}

func (p *Parser) recoverBailout() {
	if r := recover(); r != nil {
		if _, ok := r.(bailout); !ok {
			panic(r)
		}
	}
}

func (p *Parser) expect(k token.Kind) token.Token {
	t := p.tok
	if t.Kind != k {
		p.errorf(t.Pos, "%s attendu, trouvé %s", quote(k), describe(t))
	}
	p.next()
	return t
}

func (p *Parser) ident() *ast.Ident {
	t := p.expect(token.IDENT)
	return &ast.Ident{NamePos: t.Pos, Name: t.Lit}
}

// sync saute les tokens jusqu'à la fin de l'instruction : un ';' (consommé)
// ou un '}' / EOF (non consommés). Les blocs imbriqués sont sautés en entier.
func (p *Parser) sync() {
	depth := 0
	for {
		switch p.tok.Kind {
		case token.EOF:
			return
		case token.LBRACE:
			depth++
		case token.RBRACE:
			if depth == 0 {
				return
			}
			depth--
		case token.SEMICOLON:
			if depth == 0 {
				p.next()
				return
			}
		}
		p.next()
	}
}

func describe(t token.Token) string {
	switch t.Kind {
	case token.EOF:
		return "fin de fichier"
	case token.SEMICOLON:
		if t.Lit == "\n" {
			return "fin de ligne"
		}
	case token.IDENT:
		return "identifiant '" + t.Lit + "'"
	case token.INT:
		return "nombre " + t.Lit
	}
	return quote(t.Kind)
}

func quote(k token.Kind) string {
	switch k {
	case token.IDENT:
		return "identifiant"
	case token.INT:
		return "nombre"
	}
	return "'" + k.String() + "'"
}

// ---------------------------------------------------------------------------
// Programme

func (p *Parser) parseProgram() *ast.Program {
	prog := &ast.Program{}
	for p.tok.Kind != token.EOF {
		if p.tok.Kind == token.SEMICOLON {
			p.next()
			continue
		}
		if d, ok := p.parseTopDecl(); ok {
			prog.Decls = append(prog.Decls, d)
		} else {
			p.sync()
			// Un '}' orphelin au niveau global : on le saute pour avancer.
			if p.tok.Kind == token.RBRACE {
				p.next()
			}
		}
	}
	return prog
}

func (p *Parser) parseTopDecl() (d ast.Decl, ok bool) {
	defer p.recoverBailout()
	switch p.tok.Kind {
	case token.FUNCTION:
		d = p.parseFnDecl()
	case token.LET:
		d = p.parseLet()
	case token.CLASS, token.ABSTRACT:
		d = p.parseClass()
	case token.IDENT:
		if p.tok.Lit == "fn" {
			p.errorf(p.tok.Pos, "'fn' n'existe plus : écris 'function'")
		}
		fallthrough
	default:
		p.errorf(p.tok.Pos, "'class', 'function' ou 'let' attendu au niveau global, trouvé %s", describe(p.tok))
	}
	p.endStmt()
	return d, true
}

// endStmt exige la fin d'une instruction : ';', '}' (non consommé) ou EOF.
func (p *Parser) endStmt() {
	switch p.tok.Kind {
	case token.SEMICOLON:
		p.next()
	case token.RBRACE, token.EOF:
	default:
		p.errorf(p.tok.Pos, "fin d'instruction attendue, trouvé %s", describe(p.tok))
	}
}

func (p *Parser) parseFnDecl() *ast.FnDecl {
	fnTok := p.expect(token.FUNCTION)
	d := &ast.FnDecl{FnPos: fnTok.Pos, Name: p.ident()}
	d.Params = p.parseParams()
	d.Body = p.parseBlock()
	return d
}

// parseParams lit « ( a, b, … ) ».
func (p *Parser) parseParams() []*ast.Ident {
	var params []*ast.Ident
	p.expect(token.LPAREN)
	if p.tok.Kind != token.RPAREN {
		params = append(params, p.ident())
		for p.tok.Kind == token.COMMA {
			p.next()
			params = append(params, p.ident())
		}
	}
	p.expect(token.RPAREN)
	return params
}

// parseClass lit une classe (cf. docs/classes.md §4).
func (p *Parser) parseClass() *ast.ClassDecl {
	d := &ast.ClassDecl{ClassPos: p.tok.Pos}
	if p.tok.Kind == token.ABSTRACT {
		d.Abstract = true
		p.next()
	}
	p.expect(token.CLASS)
	d.Name = p.ident()
	if p.tok.Kind == token.EXTENDS {
		p.next()
		d.Parent = p.ident()
	}
	p.expect(token.LBRACE)
	for p.tok.Kind != token.RBRACE && p.tok.Kind != token.EOF {
		if p.tok.Kind == token.SEMICOLON {
			p.next()
			continue
		}
		if !p.parseMemberRecover(d) {
			p.sync()
		}
	}
	p.expect(token.RBRACE)
	return d
}

func (p *Parser) parseMemberRecover(d *ast.ClassDecl) (ok bool) {
	defer p.recoverBailout()
	p.parseMember(d)
	p.endStmt()
	return true
}

func (p *Parser) parseMember(d *ast.ClassDecl) {
	switch p.tok.Kind {
	case token.LET:
		t := p.tok
		p.next()
		f := &ast.FieldDecl{LetPos: t.Pos, Name: p.ident()}
		if p.tok.Kind == token.ASSIGN {
			p.next()
			f.Value = p.parseExpr()
		}
		d.Fields = append(d.Fields, f)

	case token.CONSTRUCTOR:
		t := p.tok
		p.next()
		m := &ast.MethodDecl{FnPos: t.Pos, Name: &ast.Ident{NamePos: t.Pos, Name: "constructor"}}
		m.Params = p.parseParams()
		m.Body = p.parseBlock()
		if d.Ctor != nil {
			p.errorf(t.Pos, "la classe %s a déjà un constructeur (ligne %d)", d.Name.Name, d.Ctor.Pos().Line)
		}
		d.Ctor = m

	case token.ABSTRACT, token.FUNCTION:
		m := &ast.MethodDecl{FnPos: p.tok.Pos}
		if p.tok.Kind == token.ABSTRACT {
			m.Abstract = true
			p.next()
		}
		p.expect(token.FUNCTION)
		m.Name = p.ident()
		m.Params = p.parseParams()
		if m.Abstract {
			if p.tok.Kind == token.LBRACE {
				p.errorf(p.tok.Pos, "une méthode abstraite n'a pas de corps")
			}
		} else {
			m.Body = p.parseBlock()
		}
		d.Methods = append(d.Methods, m)

	default:
		p.errorf(p.tok.Pos, "dans une classe : 'let', 'constructor' ou 'function' attendu, trouvé %s", describe(p.tok))
	}
}

// ---------------------------------------------------------------------------
// Instructions

func (p *Parser) parseBlock() *ast.Block {
	lb := p.expect(token.LBRACE)
	b := &ast.Block{LBrace: lb.Pos}
	for p.tok.Kind != token.RBRACE && p.tok.Kind != token.EOF {
		if p.tok.Kind == token.SEMICOLON {
			p.next()
			continue
		}
		if s, ok := p.parseStmtRecover(); ok {
			b.Stmts = append(b.Stmts, s)
		} else {
			p.sync()
		}
	}
	p.expect(token.RBRACE)
	return b
}

func (p *Parser) parseStmtRecover() (s ast.Stmt, ok bool) {
	defer p.recoverBailout()
	s = p.parseStmt()
	p.endStmt()
	return s, true
}

func (p *Parser) parseStmt() ast.Stmt {
	switch p.tok.Kind {
	case token.LET:
		return p.parseLet()
	case token.IF:
		return p.parseIf()
	case token.WHILE:
		t := p.tok
		p.next()
		cond := p.parseExpr()
		return &ast.WhileStmt{WhilePos: t.Pos, Cond: cond, Body: p.parseBlock()}
	case token.RETURN:
		t := p.tok
		p.next()
		s := &ast.ReturnStmt{ReturnPos: t.Pos}
		if p.tok.Kind != token.SEMICOLON && p.tok.Kind != token.RBRACE && p.tok.Kind != token.EOF {
			s.Value = p.parseExpr()
		}
		return s
	case token.BREAK, token.CONTINUE:
		t := p.tok
		p.next()
		return &ast.BranchStmt{TokPos: t.Pos, Tok: t.Kind}
	case token.LBRACE:
		return p.parseBlock()
	case token.FUNCTION, token.CLASS, token.ABSTRACT:
		p.errorf(p.tok.Pos, "les fonctions et les classes se déclarent au niveau global")
	}
	return p.parseSimpleStmt()
}

func (p *Parser) parseLet() *ast.LetStmt {
	t := p.expect(token.LET)
	name := p.ident()
	p.expect(token.ASSIGN)
	return &ast.LetStmt{LetPos: t.Pos, Name: name, Value: p.parseExpr()}
}

func (p *Parser) parseIf() *ast.IfStmt {
	t := p.expect(token.IF)
	s := &ast.IfStmt{IfPos: t.Pos, Cond: p.parseExpr(), Then: p.parseBlock()}
	if p.tok.Kind == token.ELSE {
		p.next()
		if p.tok.Kind == token.IF {
			s.Else = p.parseIf()
		} else {
			s.Else = p.parseBlock()
		}
	}
	return s
}

func isAssignOp(k token.Kind) bool {
	switch k {
	case token.ASSIGN, token.ADD_ASSIGN, token.SUB_ASSIGN,
		token.MUL_ASSIGN, token.DIV_ASSIGN, token.MOD_ASSIGN:
		return true
	}
	return false
}

func (p *Parser) parseSimpleStmt() ast.Stmt {
	x := p.parseExpr()
	if isAssignOp(p.tok.Kind) {
		op := p.tok
		switch x.(type) {
		case *ast.Ident, *ast.FieldExpr:
		default:
			p.errorf(op.Pos, "on ne peut affecter qu'une variable ou un champ (this.champ)")
		}
		p.next()
		return &ast.AssignStmt{Target: x, Op: op.Kind, Value: p.parseExpr()}
	}
	switch x.(type) {
	case *ast.CallExpr, *ast.MethodCall, *ast.SuperCall, *ast.SuperMethodCall, *ast.NewExpr:
	default:
		p.errorf(x.Pos(), "expression inutilisée : seul un appel (fonction, méthode, new) peut servir d'instruction")
	}
	return &ast.ExprStmt{X: x}
}

// ---------------------------------------------------------------------------
// Expressions

// Précédences des opérateurs binaires (cf. docs/spec.md §4).
var binaryPrec = map[token.Kind]int{
	token.OROR:    1,
	token.ANDAND:  2,
	token.EQ:      3,
	token.NE:      3,
	token.LT:      4,
	token.LE:      4,
	token.GT:      4,
	token.GE:      4,
	token.PIPE:    5,
	token.CARET:   6,
	token.AMP:     7,
	token.SHL:     8,
	token.SHR:     8,
	token.PLUS:    9,
	token.MINUS:   9,
	token.STAR:    10,
	token.SLASH:   10,
	token.PERCENT: 10,
}

func (p *Parser) parseExpr() ast.Expr { return p.parseBinary(1) }

// parseBinary : montée de précédence, tous les opérateurs binaires sont
// associatifs à gauche.
func (p *Parser) parseBinary(minPrec int) ast.Expr {
	x := p.parseUnary()
	for {
		prec, ok := binaryPrec[p.tok.Kind]
		if !ok || prec < minPrec {
			return x
		}
		op := p.tok
		p.next()
		y := p.parseBinary(prec + 1)
		x = &ast.BinaryExpr{X: x, OpPos: op.Pos, Op: op.Kind, Y: y}
	}
}

func (p *Parser) parseUnary() ast.Expr {
	switch p.tok.Kind {
	case token.MINUS, token.BANG, token.TILDE:
		op := p.tok
		p.next()
		return &ast.UnaryExpr{OpPos: op.Pos, Op: op.Kind, X: p.parseUnary()}
	}
	return p.parsePostfix()
}

func (p *Parser) parsePostfix() ast.Expr {
	x := p.parsePrimary()
	called := false
	for {
		switch p.tok.Kind {
		case token.LPAREN:
			switch x.(type) {
			case *ast.Ident, *ast.QualifiedName:
			default:
				p.errorf(p.tok.Pos, "seule une fonction nommée peut être appelée")
			}
			if called {
				p.errorf(p.tok.Pos, "seule une fonction nommée peut être appelée")
			}
			lp := p.tok
			x = &ast.CallExpr{Fn: x, LParen: lp.Pos, Args: p.parseArgs()}
			called = true

		case token.DOT:
			p.next()
			name := p.ident()
			if p.tok.Kind == token.LPAREN {
				x = &ast.MethodCall{Recv: x, Name: name, Args: p.parseArgs()}
				called = true
				continue
			}
			th, ok := x.(*ast.ThisExpr)
			if !ok {
				p.errorf(name.Pos(), "les champs sont privés : écris this.%s dans la classe, ou passe par une méthode", name.Name)
			}
			x = &ast.FieldExpr{ThisPos: th.ThisPos, Name: name}

		default:
			return x
		}
	}
}

// parseArgs lit « ( expr, expr, … ) ».
func (p *Parser) parseArgs() []ast.Expr {
	var args []ast.Expr
	p.expect(token.LPAREN)
	if p.tok.Kind != token.RPAREN {
		args = append(args, p.parseExpr())
		for p.tok.Kind == token.COMMA {
			p.next()
			args = append(args, p.parseExpr())
		}
	}
	p.expect(token.RPAREN)
	return args
}

func (p *Parser) parsePrimary() ast.Expr {
	t := p.tok
	switch t.Kind {
	case token.INT:
		p.next()
		v, err := parseInt(t.Lit)
		if err != nil {
			p.errorf(t.Pos, "%v", err)
		}
		return &ast.IntLit{LitPos: t.Pos, Raw: t.Lit, Value: v}
	case token.TRUE, token.FALSE:
		p.next()
		return &ast.BoolLit{LitPos: t.Pos, Value: t.Kind == token.TRUE}
	case token.IDENT:
		id := p.ident()
		if p.tok.Kind == token.DOT && natives.IsModule(id.Name) {
			p.next()
			return &ast.QualifiedName{Module: id, Name: p.ident()}
		}
		return id
	case token.THIS:
		p.next()
		return &ast.ThisExpr{ThisPos: t.Pos}
	case token.NEW:
		p.next()
		return &ast.NewExpr{NewPos: t.Pos, Class: p.ident(), Args: p.parseArgs()}
	case token.SUPER:
		p.next()
		if p.tok.Kind == token.DOT {
			p.next()
			name := p.ident()
			return &ast.SuperMethodCall{SuperPos: t.Pos, Name: name, Args: p.parseArgs()}
		}
		return &ast.SuperCall{SuperPos: t.Pos, Args: p.parseArgs()}
	case token.LPAREN:
		p.next()
		x := p.parseExpr()
		p.expect(token.RPAREN)
		return x
	}
	p.errorf(t.Pos, "expression attendue, trouvé %s", describe(t))
	return nil
}

// parseInt convertit un littéral (décimal, 0x, 0b, avec '_') en int32.
// Les valeurs jusqu'à 0xFFFFFFFF sont acceptées et repliées modulo 2³²,
// pour pouvoir écrire des masques comme 0xFFFF0000.
func parseInt(lit string) (int32, error) {
	s := strings.ReplaceAll(lit, "_", "")
	base := 10
	if len(s) > 2 && s[0] == '0' {
		switch s[1] {
		case 'x', 'X':
			base, s = 16, s[2:]
		case 'b', 'B':
			base, s = 2, s[2:]
		}
	}
	v, err := strconv.ParseUint(s, base, 32)
	if err != nil {
		return 0, fmt.Errorf("entier hors limites (32 bits) : %s", lit)
	}
	return int32(uint32(v)), nil
}
