// Package codegen traduit un programme vérifié par sema en bytecode
// (cf. docs/bytecode.md).
package codegen

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/getchili/chili/compiler/ast"
	"github.com/getchili/chili/compiler/bytecode"
	"github.com/getchili/chili/compiler/sema"
	"github.com/getchili/chili/compiler/token"
)

// Error est une erreur de génération (fonction trop grosse, saut trop long…).
type Error struct {
	Pos token.Pos
	Msg string
}

func (e *Error) Error() string { return fmt.Sprintf("%s: %s", e.Pos, e.Msg) }

type loop struct {
	start  int   // début de la condition (cible de continue)
	breaks []int // positions des opérandes de saut à corriger
}

type gen struct {
	info *sema.Info
	code []byte

	// État de la fonction en cours.
	depth, maxDepth int
	loops           []*loop
	fnPos           token.Pos
}

// bailout transporte une *Error à travers la génération.
type bailout struct{ err *Error }

// Generate produit le programme bytecode. info doit venir d'un sema.Check
// sans erreur.
func Generate(info *sema.Info) (prog *bytecode.Program, err error) {
	g := &gen{info: info}
	defer func() {
		if r := recover(); r != nil {
			b, ok := r.(bailout)
			if !ok {
				panic(r)
			}
			prog, err = nil, b.err
		}
	}()

	prog = &bytecode.Program{NumGlobals: uint16(len(info.Globals))}
	for _, fi := range info.Funcs {
		prog.Funcs = append(prog.Funcs, g.genFunc(fi))
	}
	prog.Entry = uint16(len(prog.Funcs))
	prog.Funcs = append(prog.Funcs, g.genInit())
	prog.Code = g.code
	return prog, nil
}

func (g *gen) fail(pos token.Pos, format string, args ...any) {
	panic(bailout{&Error{Pos: pos, Msg: fmt.Sprintf(format, args...)}})
}

// ---------------------------------------------------------------------------
// Émission

func (g *gen) adjust(delta int) {
	g.depth += delta
	if g.depth > g.maxDepth {
		g.maxDepth = g.depth
	}
}

func (g *gen) op(op bytecode.Op, delta int) {
	g.code = append(g.code, byte(op))
	g.adjust(delta)
}

func (g *gen) u8(v int)  { g.code = append(g.code, byte(v)) }
func (g *gen) u16(v int) { g.code = binary.LittleEndian.AppendUint16(g.code, uint16(v)) }

func (g *gen) pushConst(v int32) {
	if v >= math.MinInt8 && v <= math.MaxInt8 {
		g.op(bytecode.PUSH8, +1)
		g.u8(int(int8(v)))
		return
	}
	g.op(bytecode.PUSH, +1)
	g.code = binary.LittleEndian.AppendUint32(g.code, uint32(v))
}

// jump émet un saut avec un opérande à corriger plus tard ; renvoie la
// position de l'opérande.
func (g *gen) jump(op bytecode.Op) int {
	delta := 0
	if op != bytecode.JMP {
		delta = -1
	}
	g.op(op, delta)
	at := len(g.code)
	g.u16(0)
	return at
}

// patch fait pointer le saut dont l'opérande est à `at` vers target.
func (g *gen) patch(at, target int) {
	rel := target - (at + 2)
	if rel < math.MinInt16 || rel > math.MaxInt16 {
		g.fail(g.fnPos, "fonction trop grande : saut de %d octets (max %d)", rel, math.MaxInt16)
	}
	binary.LittleEndian.PutUint16(g.code[at:], uint16(int16(rel)))
}

// jumpTo émet un saut vers une adresse déjà connue (en arrière).
func (g *gen) jumpTo(op bytecode.Op, target int) {
	g.patch(g.jump(op), target)
}

// ---------------------------------------------------------------------------
// Fonctions

func (g *gen) beginFunc(pos token.Pos) int {
	g.depth, g.maxDepth, g.loops, g.fnPos = 0, 0, nil, pos
	return len(g.code)
}

func (g *gen) endFunc(name string, start, params, locals int) bytecode.Func {
	if g.maxDepth > math.MaxUint16 {
		g.fail(g.fnPos, "%s : expression trop profonde", name)
	}
	return bytecode.Func{
		Name:      name,
		Offset:    uint32(start),
		NumParams: uint8(params),
		NumLocals: uint8(locals),
		MaxStack:  uint16(g.maxDepth),
	}
}

func (g *gen) genFunc(fi *sema.FuncInfo) bytecode.Func {
	d := fi.Decl
	start := g.beginFunc(d.Pos())
	g.stmts(d.Body.Stmts)
	// Retour implicite (inatteignable si la fonction finit par un return).
	g.pushConst(0)
	g.op(bytecode.RET, -1)
	return g.endFunc(d.Name.Name, start, len(d.Params), fi.NumLocals)
}

// genInit produit <init> : globales, appel de main, HALT.
func (g *gen) genInit() bytecode.Func {
	start := g.beginFunc(token.Pos{Line: 1, Col: 1})
	for i, ls := range g.info.Globals {
		g.expr(ls.Value)
		g.op(bytecode.GSTORE, -1)
		g.u16(i)
	}
	g.op(bytecode.CALL, +1)
	g.u16(g.info.Main.Index)
	g.op(bytecode.POP, -1)
	g.op(bytecode.HALT, 0)
	return g.endFunc("<init>", start, 0, 0)
}

// ---------------------------------------------------------------------------
// Instructions

func (g *gen) stmts(list []ast.Stmt) {
	for _, s := range list {
		g.stmt(s)
	}
}

func (g *gen) stmt(s ast.Stmt) {
	switch s := s.(type) {
	case *ast.LetStmt:
		g.expr(s.Value)
		g.store(s.Name)

	case *ast.AssignStmt:
		if s.Op == token.ASSIGN {
			g.expr(s.Value)
		} else {
			g.load(s.Target)
			g.expr(s.Value)
			g.op(compoundOps[s.Op], -1)
		}
		g.store(s.Target)

	case *ast.ExprStmt:
		g.expr(s.X)
		g.op(bytecode.POP, -1)

	case *ast.Block:
		g.stmts(s.Stmts)

	case *ast.IfStmt:
		g.expr(s.Cond)
		toElse := g.jump(bytecode.JZ)
		g.stmts(s.Then.Stmts)
		if s.Else == nil {
			g.patch(toElse, len(g.code))
			return
		}
		toEnd := g.jump(bytecode.JMP)
		g.patch(toElse, len(g.code))
		g.stmt(s.Else)
		g.patch(toEnd, len(g.code))

	case *ast.WhileStmt:
		l := &loop{start: len(g.code)}
		g.loops = append(g.loops, l)
		g.expr(s.Cond)
		exit := g.jump(bytecode.JZ)
		g.stmts(s.Body.Stmts)
		g.jumpTo(bytecode.JMP, l.start)
		end := len(g.code)
		g.patch(exit, end)
		for _, b := range l.breaks {
			g.patch(b, end)
		}
		g.loops = g.loops[:len(g.loops)-1]

	case *ast.ReturnStmt:
		if s.Value != nil {
			g.expr(s.Value)
		} else {
			g.pushConst(0)
		}
		g.op(bytecode.RET, -1)

	case *ast.BranchStmt:
		l := g.loops[len(g.loops)-1]
		if s.Tok == token.BREAK {
			l.breaks = append(l.breaks, g.jump(bytecode.JMP))
		} else {
			g.jumpTo(bytecode.JMP, l.start)
		}

	default:
		panic(fmt.Sprintf("codegen: instruction inconnue %T", s))
	}
}

var compoundOps = map[token.Kind]bytecode.Op{
	token.ADD_ASSIGN: bytecode.ADD,
	token.SUB_ASSIGN: bytecode.SUB,
	token.MUL_ASSIGN: bytecode.MUL,
	token.DIV_ASSIGN: bytecode.DIV,
	token.MOD_ASSIGN: bytecode.MOD,
}

func (g *gen) load(id *ast.Ident) {
	sym := g.info.Refs[id]
	if sym.Kind == sema.Local {
		g.op(bytecode.LOAD, +1)
		g.u8(sym.Index)
	} else {
		g.op(bytecode.GLOAD, +1)
		g.u16(sym.Index)
	}
}

func (g *gen) store(id *ast.Ident) {
	sym := g.info.Refs[id]
	if sym.Kind == sema.Local {
		g.op(bytecode.STORE, -1)
		g.u8(sym.Index)
	} else {
		g.op(bytecode.GSTORE, -1)
		g.u16(sym.Index)
	}
}

// ---------------------------------------------------------------------------
// Expressions

var binaryOps = map[token.Kind]bytecode.Op{
	token.PLUS:    bytecode.ADD,
	token.MINUS:   bytecode.SUB,
	token.STAR:    bytecode.MUL,
	token.SLASH:   bytecode.DIV,
	token.PERCENT: bytecode.MOD,
	token.AMP:     bytecode.AND,
	token.PIPE:    bytecode.OR,
	token.CARET:   bytecode.XOR,
	token.SHL:     bytecode.SHL,
	token.SHR:     bytecode.SHR,
	token.EQ:      bytecode.EQ,
	token.NE:      bytecode.NE,
	token.LT:      bytecode.LT,
	token.LE:      bytecode.LE,
	token.GT:      bytecode.GT,
	token.GE:      bytecode.GE,
}

var unaryOps = map[token.Kind]bytecode.Op{
	token.MINUS: bytecode.NEG,
	token.BANG:  bytecode.NOT,
	token.TILDE: bytecode.BNOT,
}

func (g *gen) expr(e ast.Expr) {
	switch e := e.(type) {
	case *ast.IntLit:
		g.pushConst(e.Value)

	case *ast.BoolLit:
		if e.Value {
			g.pushConst(1)
		} else {
			g.pushConst(0)
		}

	case *ast.Ident:
		g.load(e)

	case *ast.UnaryExpr:
		// Littéral négatif : directement une constante.
		if lit, ok := e.X.(*ast.IntLit); ok && e.Op == token.MINUS {
			g.pushConst(-lit.Value)
			return
		}
		g.expr(e.X)
		g.op(unaryOps[e.Op], 0)

	case *ast.BinaryExpr:
		switch e.Op {
		case token.ANDAND:
			g.shortCircuit(e, bytecode.JZ, 0)
		case token.OROR:
			g.shortCircuit(e, bytecode.JNZ, 1)
		default:
			g.expr(e.X)
			g.expr(e.Y)
			g.op(binaryOps[e.Op], -1)
		}

	case *ast.CallExpr:
		for _, a := range e.Args {
			g.expr(a)
		}
		sym := g.info.Calls[e]
		n := len(e.Args)
		if sym.Kind == sema.Native {
			g.op(bytecode.CALL_NATIVE, 1-n)
			g.u16(sym.Index)
			g.u8(n)
		} else {
			g.op(bytecode.CALL, 1-n)
			g.u16(sym.Index)
		}

	default:
		panic(fmt.Sprintf("codegen: expression inconnue %T", e))
	}
}

// shortCircuit compile && (jmp = JZ, short = 0) et || (jmp = JNZ, short = 1) :
//
//	X ; jmp L ; Y ; jmp L ; PUSH !short ; JMP end ; L: PUSH short ; end:
//
// Le résultat vaut toujours 0 ou 1.
func (g *gen) shortCircuit(e *ast.BinaryExpr, jmp bytecode.Op, short int32) {
	g.expr(e.X)
	j1 := g.jump(jmp)
	g.expr(e.Y)
	j2 := g.jump(jmp)
	g.pushConst(1 - short)
	toEnd := g.jump(bytecode.JMP)
	g.adjust(-1) // l'autre branche empile sa propre valeur
	g.patch(j1, len(g.code))
	g.patch(j2, len(g.code))
	g.pushConst(short)
	g.patch(toEnd, len(g.code))
}
