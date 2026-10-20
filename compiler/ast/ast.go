// Package ast définit l'arbre syntaxique de Chili.
//
// Chaque nœud sait s'afficher en S-expression (méthode String), ce qui sert
// aux tests du parser et à la commande `chili parse`.
package ast

import (
	"fmt"
	"strings"

	"github.com/getchili/chili/compiler/token"
)

type Node interface {
	Pos() token.Pos
	String() string
}

type Expr interface {
	Node
	exprNode()
}

type Stmt interface {
	Node
	stmtNode()
}

// Decl est une déclaration de premier niveau : *FnDecl ou *LetStmt.
type Decl interface {
	Node
	declNode()
}

// ---------------------------------------------------------------------------
// Programme et déclarations

type Program struct {
	Decls []Decl
}

func (p *Program) String() string {
	parts := make([]string, len(p.Decls))
	for i, d := range p.Decls {
		parts[i] = d.String()
	}
	return strings.Join(parts, "\n")
}

type FnDecl struct {
	FnPos  token.Pos
	Name   *Ident
	Params []*Ident
	Body   *Block
}

func (d *FnDecl) Pos() token.Pos { return d.FnPos }
func (d *FnDecl) declNode()      {}
func (d *FnDecl) String() string {
	params := make([]string, len(d.Params))
	for i, p := range d.Params {
		params[i] = p.Name
	}
	return fmt.Sprintf("(fn %s (%s) %s)", d.Name.Name, strings.Join(params, " "), d.Body)
}

// ---------------------------------------------------------------------------
// Instructions

type Block struct {
	LBrace token.Pos
	Stmts  []Stmt
}

func (s *Block) Pos() token.Pos { return s.LBrace }
func (s *Block) stmtNode()      {}
func (s *Block) String() string {
	var b strings.Builder
	b.WriteString("(block")
	for _, st := range s.Stmts {
		b.WriteByte(' ')
		b.WriteString(st.String())
	}
	b.WriteByte(')')
	return b.String()
}

type LetStmt struct {
	LetPos token.Pos
	Name   *Ident
	Value  Expr
}

func (s *LetStmt) Pos() token.Pos { return s.LetPos }
func (s *LetStmt) stmtNode()      {}
func (s *LetStmt) declNode()      {}
func (s *LetStmt) String() string { return fmt.Sprintf("(let %s %s)", s.Name.Name, s.Value) }

// AssignStmt couvre '=' et les affectations composées ('+=', ...).
type AssignStmt struct {
	Target *Ident
	Op     token.Kind // ASSIGN, ADD_ASSIGN, ...
	Value  Expr
}

func (s *AssignStmt) Pos() token.Pos { return s.Target.Pos() }
func (s *AssignStmt) stmtNode()      {}
func (s *AssignStmt) String() string {
	return fmt.Sprintf("(%s %s %s)", s.Op, s.Target.Name, s.Value)
}

type ExprStmt struct {
	X Expr
}

func (s *ExprStmt) Pos() token.Pos { return s.X.Pos() }
func (s *ExprStmt) stmtNode()      {}
func (s *ExprStmt) String() string { return s.X.String() }

type IfStmt struct {
	IfPos token.Pos
	Cond  Expr
	Then  *Block
	Else  Stmt // nil, *Block ou *IfStmt
}

func (s *IfStmt) Pos() token.Pos { return s.IfPos }
func (s *IfStmt) stmtNode()      {}
func (s *IfStmt) String() string {
	if s.Else == nil {
		return fmt.Sprintf("(if %s %s)", s.Cond, s.Then)
	}
	return fmt.Sprintf("(if %s %s %s)", s.Cond, s.Then, s.Else)
}

type WhileStmt struct {
	WhilePos token.Pos
	Cond     Expr
	Body     *Block
}

func (s *WhileStmt) Pos() token.Pos { return s.WhilePos }
func (s *WhileStmt) stmtNode()      {}
func (s *WhileStmt) String() string { return fmt.Sprintf("(while %s %s)", s.Cond, s.Body) }

type ReturnStmt struct {
	ReturnPos token.Pos
	Value     Expr // nil si `return` seul
}

func (s *ReturnStmt) Pos() token.Pos { return s.ReturnPos }
func (s *ReturnStmt) stmtNode()      {}
func (s *ReturnStmt) String() string {
	if s.Value == nil {
		return "(return)"
	}
	return fmt.Sprintf("(return %s)", s.Value)
}

// BranchStmt représente `break` ou `continue`.
type BranchStmt struct {
	TokPos token.Pos
	Tok    token.Kind // BREAK ou CONTINUE
}

func (s *BranchStmt) Pos() token.Pos { return s.TokPos }
func (s *BranchStmt) stmtNode()      {}
func (s *BranchStmt) String() string { return "(" + s.Tok.String() + ")" }

// ---------------------------------------------------------------------------
// Expressions

type Ident struct {
	NamePos token.Pos
	Name    string
}

func (e *Ident) Pos() token.Pos { return e.NamePos }
func (e *Ident) exprNode()      {}
func (e *Ident) String() string { return e.Name }

// QualifiedName désigne une native préfixée, ex. gpio.write.
type QualifiedName struct {
	Module *Ident
	Name   *Ident
}

func (e *QualifiedName) Pos() token.Pos { return e.Module.Pos() }
func (e *QualifiedName) exprNode()      {}
func (e *QualifiedName) String() string { return e.Module.Name + "." + e.Name.Name }

type IntLit struct {
	LitPos token.Pos
	Raw    string
	Value  int32
}

func (e *IntLit) Pos() token.Pos { return e.LitPos }
func (e *IntLit) exprNode()      {}
func (e *IntLit) String() string { return fmt.Sprint(e.Value) }

type BoolLit struct {
	LitPos token.Pos
	Value  bool
}

func (e *BoolLit) Pos() token.Pos { return e.LitPos }
func (e *BoolLit) exprNode()      {}
func (e *BoolLit) String() string { return fmt.Sprint(e.Value) }

type UnaryExpr struct {
	OpPos token.Pos
	Op    token.Kind // MINUS, BANG, TILDE
	X     Expr
}

func (e *UnaryExpr) Pos() token.Pos { return e.OpPos }
func (e *UnaryExpr) exprNode()      {}
func (e *UnaryExpr) String() string { return fmt.Sprintf("(%s %s)", e.Op, e.X) }

type BinaryExpr struct {
	X     Expr
	OpPos token.Pos
	Op    token.Kind
	Y     Expr
}

func (e *BinaryExpr) Pos() token.Pos { return e.X.Pos() }
func (e *BinaryExpr) exprNode()      {}
func (e *BinaryExpr) String() string { return fmt.Sprintf("(%s %s %s)", e.Op, e.X, e.Y) }

type CallExpr struct {
	Fn     Expr // *Ident ou *QualifiedName
	LParen token.Pos
	Args   []Expr
}

func (e *CallExpr) Pos() token.Pos { return e.Fn.Pos() }
func (e *CallExpr) exprNode()      {}
func (e *CallExpr) String() string {
	var b strings.Builder
	b.WriteString("(call ")
	b.WriteString(e.Fn.String())
	for _, a := range e.Args {
		b.WriteByte(' ')
		b.WriteString(a.String())
	}
	b.WriteByte(')')
	return b.String()
}
