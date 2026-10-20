// Package codegen traduit un programme vérifié par sema en bytecode
// (cf. docs/bytecode.md et docs/classes.md).
package codegen

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"

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

	names   []string
	nameIdx map[string]int
	newFn   map[*sema.ClassInfo]int // fonction synthétique « new » de chaque classe concrète

	// État de la fonction en cours.
	depth, maxDepth int
	loops           []*loop
	fnPos           token.Pos
}

// bailout transporte une *Error à travers la génération.
type bailout struct{ err *Error }

// Generate produit le programme bytecode. info doit venir d'un sema.Check
// sans erreur.
//
// Ordre des fonctions : celles de sema (fonctions globales, méthodes,
// constructeurs), puis une fonction « Classe.new » par classe concrète,
// puis <init> (point d'entrée).
func Generate(info *sema.Info) (prog *bytecode.Program, err error) {
	g := &gen{info: info, nameIdx: map[string]int{}, newFn: map[*sema.ClassInfo]int{}}
	defer func() {
		if r := recover(); r != nil {
			b, ok := r.(bailout)
			if !ok {
				panic(r)
			}
			prog, err = nil, b.err
		}
	}()

	next := len(info.Funcs)
	for _, ci := range info.Classes {
		if !ci.Abstract {
			g.newFn[ci] = next
			next++
		}
	}

	prog = &bytecode.Program{NumGlobals: uint16(len(info.Globals))}
	for _, fi := range info.Funcs {
		prog.Funcs = append(prog.Funcs, g.genFunc(fi))
	}
	for _, ci := range info.Classes {
		if !ci.Abstract {
			prog.Funcs = append(prog.Funcs, g.genNew(ci))
		}
	}
	prog.Entry = uint16(len(prog.Funcs))
	prog.Funcs = append(prog.Funcs, g.genInit())

	for _, ci := range info.Classes {
		prog.Classes = append(prog.Classes, g.classEntry(ci))
	}
	if len(g.names) > math.MaxUint16 {
		g.fail(token.Pos{Line: 1, Col: 1}, "trop de noms (max %d)", math.MaxUint16)
	}
	prog.Names = g.names
	prog.Code = g.code
	return prog, nil
}

func (g *gen) fail(pos token.Pos, format string, args ...any) {
	panic(bailout{&Error{Pos: pos, Msg: fmt.Sprintf(format, args...)}})
}

// name renvoie l'index de s dans la table des noms (ajouté au besoin).
func (g *gen) name(s string) int {
	if i, ok := g.nameIdx[s]; ok {
		return i
	}
	if len(s) > 255 {
		s = s[:255]
	}
	g.nameIdx[s] = len(g.names)
	g.names = append(g.names, s)
	return len(g.names) - 1
}

func (g *gen) classEntry(ci *sema.ClassInfo) bytecode.Class {
	c := bytecode.Class{
		NameIdx:   uint16(g.name(ci.Name)),
		Parent:    bytecode.NoParent,
		NumFields: uint8(len(ci.Fields)),
		Abstract:  ci.Abstract,
	}
	if ci.Parent != nil {
		c.Parent = uint16(ci.Parent.Index)
	}
	for _, m := range ci.SortedMethods() {
		c.Methods = append(c.Methods, bytecode.Method{Selector: uint16(g.name(m.Name)), Fn: uint16(m.Func.Index)})
	}
	sort.Slice(c.Methods, func(i, j int) bool { return c.Methods[i].Selector < c.Methods[j].Selector })
	return c
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

func (g *gen) call(fn, nargs int) {
	g.op(bytecode.CALL, 1-nargs)
	g.u16(fn)
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
		NameIdx:   uint16(g.name(name)),
		Offset:    uint32(start),
		NumParams: uint8(params),
		NumLocals: uint8(locals),
		MaxStack:  uint16(g.maxDepth),
	}
}

func (g *gen) genFunc(fi *sema.FuncInfo) bytecode.Func {
	start := g.beginFunc(fi.Pos)
	if fi.ImplicitSuper != nil {
		g.op(bytecode.LOAD, +1) // this
		g.u8(0)
		g.call(fi.ImplicitSuper.Index, 1)
		g.op(bytecode.POP, -1)
	}
	g.stmts(fi.Body.Stmts)
	// Retour implicite (inatteignable si la fonction finit par un return).
	g.pushConst(0)
	g.op(bytecode.RET, -1)
	return g.endFunc(fi.Name, start, fi.NumParams(), fi.NumLocals)
}

// genNew produit « Classe.new(args) » : crée l'objet, initialise ses champs
// (classes parentes d'abord), appelle le constructeur et renvoie l'objet.
func (g *gen) genNew(ci *sema.ClassInfo) bytecode.Func {
	start := g.beginFunc(ci.Decl.Pos())
	n := 0
	if ci.EffCtor != nil {
		n = len(ci.EffCtor.Params)
	}
	obj := n // slot de l'objet, après les paramètres

	g.op(bytecode.ALLOC, +1)
	g.u16(ci.Index)
	g.op(bytecode.STORE, -1)
	g.u8(obj)
	for _, f := range ci.Fields {
		if f.Decl.Value == nil {
			continue // ALLOC met déjà les champs à 0
		}
		g.op(bytecode.LOAD, +1)
		g.u8(obj)
		g.expr(f.Decl.Value)
		g.op(bytecode.SETF, -2)
		g.u8(f.Index)
	}
	if ci.EffCtor != nil {
		g.op(bytecode.LOAD, +1)
		g.u8(obj)
		for i := 0; i < n; i++ {
			g.op(bytecode.LOAD, +1)
			g.u8(i)
		}
		g.call(ci.EffCtor.Index, n+1)
		g.op(bytecode.POP, -1)
	}
	g.op(bytecode.LOAD, +1)
	g.u8(obj)
	g.op(bytecode.RET, -1)
	return g.endFunc(ci.Name+".new", start, n, n+1)
}

// genInit produit <init> : globales, new Main(), main(), HALT.
func (g *gen) genInit() bytecode.Func {
	start := g.beginFunc(token.Pos{Line: 1, Col: 1})
	for i, ls := range g.info.Globals {
		g.expr(ls.Value)
		g.op(bytecode.GSTORE, -1)
		g.u16(i)
	}
	g.call(g.newFn[g.info.MainClass], 0)
	g.call(g.info.MainMethod.Index, 1)
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
		g.assign(s)

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

func (g *gen) assign(s *ast.AssignStmt) {
	switch t := s.Target.(type) {
	case *ast.Ident:
		if s.Op == token.ASSIGN {
			g.expr(s.Value)
		} else {
			g.load(t)
			g.expr(s.Value)
			g.op(compoundOps[s.Op], -1)
		}
		g.store(t)

	case *ast.FieldExpr:
		idx := g.info.Fields[t].Index
		g.loadThis()
		if s.Op == token.ASSIGN {
			g.expr(s.Value)
		} else {
			g.loadThis()
			g.op(bytecode.GETF, 0)
			g.u8(idx)
			g.expr(s.Value)
			g.op(compoundOps[s.Op], -1)
		}
		g.op(bytecode.SETF, -2)
		g.u8(idx)

	default:
		panic(fmt.Sprintf("codegen: affectation de %T", t))
	}
}

func (g *gen) loadThis() {
	g.op(bytecode.LOAD, +1)
	g.u8(0)
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

func (g *gen) args(args []ast.Expr) {
	for _, a := range args {
		g.expr(a)
	}
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
		g.args(e.Args)
		sym := g.info.Calls[e]
		n := len(e.Args)
		if sym.Kind == sema.Native {
			g.op(bytecode.CALL_NATIVE, 1-n)
			g.u16(sym.Index)
			g.u8(n)
		} else {
			g.call(sym.Index, n)
		}

	case *ast.ThisExpr:
		g.loadThis()

	case *ast.FieldExpr:
		g.loadThis()
		g.op(bytecode.GETF, 0)
		g.u8(g.info.Fields[e].Index)

	case *ast.NewExpr:
		g.args(e.Args)
		g.call(g.newFn[g.info.News[e]], len(e.Args))

	case *ast.MethodCall:
		g.expr(e.Recv)
		g.args(e.Args)
		g.op(bytecode.CALLM, -len(e.Args))
		g.u16(g.name(e.Name.Name))
		g.u8(len(e.Args))

	case *ast.SuperCall:
		fn := g.info.SuperCalls[e]
		if fn == nil { // la classe parente n'a pas de constructeur : rien à faire
			g.pushConst(0)
			return
		}
		g.loadThis()
		g.args(e.Args)
		g.call(fn.Index, len(e.Args)+1)

	case *ast.SuperMethodCall:
		g.loadThis()
		g.args(e.Args)
		g.call(g.info.SuperMethods[e].Index, len(e.Args)+1)

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
