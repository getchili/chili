// Package sema fait l'analyse sémantique d'un programme Chili : résolution
// des noms, portées, arité des appels, présence de main, break/continue.
//
// Le résultat (Info) donne au générateur de code tout ce dont il a besoin :
// index des globales, slots des locales, index des fonctions, ID des natives.
package sema

import (
	"fmt"

	"github.com/getchili/chili/compiler/ast"
	"github.com/getchili/chili/compiler/diag"
	"github.com/getchili/chili/compiler/natives"
	"github.com/getchili/chili/compiler/token"
)

// Limites imposées par le format du bytecode (cf. docs/spec.md §7).
const (
	MaxLocals  = 255
	MaxArgs    = 255
	MaxGlobals = 65535
	MaxFuncs   = 65535
)

type SymKind int

const (
	Global SymKind = iota
	Local
	Func
	Native
)

func (k SymKind) String() string {
	return [...]string{"variable globale", "variable locale", "fonction", "fonction native"}[k]
}

type Symbol struct {
	Kind  SymKind
	Name  string
	Index int       // globale : index ; locale : slot ; fonction : index ; native : ID
	Arity int       // fonctions et natives
	Pos   token.Pos // position de la déclaration (zéro pour les natives)
}

type FuncInfo struct {
	Decl      *ast.FnDecl
	Index     int
	NumLocals int // paramètres compris
}

type Info struct {
	// Refs associe chaque identifiant de variable (lecture, affectation ou
	// nom déclaré par let/paramètre) à son symbole.
	Refs map[*ast.Ident]*Symbol
	// Calls associe chaque appel à la fonction ou native appelée.
	Calls   map[*ast.CallExpr]*Symbol
	Funcs   []*FuncInfo    // dans l'ordre du fichier
	Globals []*ast.LetStmt // dans l'ordre du fichier ; index = position ici
	Main    *FuncInfo
}

// ---------------------------------------------------------------------------
// Portées

type scope struct {
	parent *scope
	syms   map[string]*Symbol
}

func newScope(parent *scope) *scope {
	return &scope{parent: parent, syms: map[string]*Symbol{}}
}

func (s *scope) lookup(name string) *Symbol {
	for ; s != nil; s = s.parent {
		if sym, ok := s.syms[name]; ok {
			return sym
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Checker

type checker struct {
	info   *Info
	errs   diag.ErrorList
	global *scope

	// État de la fonction en cours d'analyse.
	fn        *FuncInfo
	loopDepth int
}

// Check analyse prog. err est une diag.ErrorList si non nil ; info est
// toujours renvoyé (partiellement rempli en cas d'erreur).
func Check(prog *ast.Program) (*Info, error) {
	c := &checker{
		info: &Info{
			Refs:  map[*ast.Ident]*Symbol{},
			Calls: map[*ast.CallExpr]*Symbol{},
		},
		global: newScope(nil),
	}
	c.checkProgram(prog)
	if len(c.errs) > 0 {
		c.errs.Sort()
		return c.info, c.errs
	}
	return c.info, nil
}

func (c *checker) errorf(pos token.Pos, format string, args ...any) {
	c.errs = append(c.errs, diag.Error{Pos: pos, Msg: fmt.Sprintf(format, args...)})
}

// declare ajoute sym dans s, en signalant redéclarations et noms réservés.
func (c *checker) declare(s *scope, id *ast.Ident, sym *Symbol) {
	if _, _, ok := natives.Lookup(id.Name); ok {
		c.errorf(id.Pos(), "'%s' est une fonction native, ce nom est réservé", id.Name)
		return
	}
	if natives.IsModule(id.Name) {
		c.errorf(id.Pos(), "'%s' est un module natif, ce nom est réservé", id.Name)
		return
	}
	if prev, ok := s.syms[id.Name]; ok {
		c.errorf(id.Pos(), "'%s' est déjà déclaré dans cette portée (%s, ligne %d)",
			id.Name, prev.Kind, prev.Pos.Line)
		return
	}
	s.syms[id.Name] = sym
}

func (c *checker) checkProgram(prog *ast.Program) {
	// 1. Les fonctions sont visibles partout, même avant leur définition.
	for _, d := range prog.Decls {
		fd, ok := d.(*ast.FnDecl)
		if !ok {
			continue
		}
		fi := &FuncInfo{Decl: fd, Index: len(c.info.Funcs)}
		c.info.Funcs = append(c.info.Funcs, fi)
		c.declare(c.global, fd.Name, &Symbol{
			Kind: Func, Name: fd.Name.Name, Index: fi.Index,
			Arity: len(fd.Params), Pos: fd.Name.Pos(),
		})
		if fd.Name.Name == "main" {
			c.info.Main = fi
		}
	}
	if len(c.info.Funcs) > MaxFuncs {
		c.errorf(token.Pos{Line: 1, Col: 1}, "trop de fonctions (max %d)", MaxFuncs)
	}

	// 2. Les globales, dans l'ordre : un initialiseur ne voit que les
	//    globales déclarées avant lui.
	for _, d := range prog.Decls {
		ls, ok := d.(*ast.LetStmt)
		if !ok {
			continue
		}
		c.checkExpr(ls.Value, c.global)
		sym := &Symbol{Kind: Global, Name: ls.Name.Name, Index: len(c.info.Globals), Pos: ls.Name.Pos()}
		c.info.Globals = append(c.info.Globals, ls)
		c.declare(c.global, ls.Name, sym)
		c.info.Refs[ls.Name] = sym
		if len(c.info.Globals) == MaxGlobals+1 {
			c.errorf(ls.Pos(), "trop de variables globales (max %d)", MaxGlobals)
		}
	}

	// 3. Les corps de fonctions.
	for _, fi := range c.info.Funcs {
		c.checkFunc(fi)
	}

	if c.info.Main == nil {
		c.errorf(token.Pos{Line: 1, Col: 1}, "le programme doit définir fn main()")
	} else if n := len(c.info.Main.Decl.Params); n != 0 {
		c.errorf(c.info.Main.Decl.Name.Pos(), "main ne doit pas avoir de paramètres (%d trouvés)", n)
	}
}

func (c *checker) checkFunc(fi *FuncInfo) {
	c.fn = fi
	c.loopDepth = 0
	// Les paramètres et le corps partagent la même portée :
	// fn f(a) { let a = 1 } est une redéclaration.
	s := newScope(c.global)
	if len(fi.Decl.Params) > MaxArgs {
		c.errorf(fi.Decl.Name.Pos(), "trop de paramètres (max %d)", MaxArgs)
	}
	for _, p := range fi.Decl.Params {
		c.declareLocal(s, p)
	}
	c.checkStmts(fi.Decl.Body.Stmts, s)
	c.fn = nil
}

func (c *checker) declareLocal(s *scope, id *ast.Ident) {
	sym := &Symbol{Kind: Local, Name: id.Name, Index: c.fn.NumLocals, Pos: id.Pos()}
	c.fn.NumLocals++
	if c.fn.NumLocals == MaxLocals+1 {
		c.errorf(id.Pos(), "trop de variables locales dans %s (max %d)", c.fn.Decl.Name.Name, MaxLocals)
	}
	c.declare(s, id, sym)
	c.info.Refs[id] = sym
}

// ---------------------------------------------------------------------------
// Instructions

func (c *checker) checkStmts(stmts []ast.Stmt, s *scope) {
	for _, st := range stmts {
		c.checkStmt(st, s)
	}
}

func (c *checker) checkStmt(st ast.Stmt, s *scope) {
	switch st := st.(type) {
	case *ast.LetStmt:
		// L'initialiseur est résolu avant la déclaration :
		// let x = x + 1 lit le x de la portée englobante.
		c.checkExpr(st.Value, s)
		c.declareLocal(s, st.Name)

	case *ast.AssignStmt:
		c.checkExpr(st.Value, s)
		c.checkVar(st.Target, s, "l'affecter")

	case *ast.ExprStmt:
		c.checkExpr(st.X, s)

	case *ast.Block:
		c.checkStmts(st.Stmts, newScope(s))

	case *ast.IfStmt:
		c.checkExpr(st.Cond, s)
		c.checkStmts(st.Then.Stmts, newScope(s))
		if st.Else != nil {
			c.checkStmt(st.Else, s)
		}

	case *ast.WhileStmt:
		c.checkExpr(st.Cond, s)
		c.loopDepth++
		c.checkStmts(st.Body.Stmts, newScope(s))
		c.loopDepth--

	case *ast.ReturnStmt:
		if st.Value != nil {
			c.checkExpr(st.Value, s)
		}

	case *ast.BranchStmt:
		if c.loopDepth == 0 {
			c.errorf(st.Pos(), "'%s' en dehors d'une boucle", st.Tok)
		}

	default:
		panic(fmt.Sprintf("sema: instruction inconnue %T", st))
	}
}

// checkVar résout un identifiant qui doit désigner une variable.
func (c *checker) checkVar(id *ast.Ident, s *scope, action string) {
	sym := s.lookup(id.Name)
	switch {
	case sym == nil:
		if _, _, ok := natives.Lookup(id.Name); ok {
			c.errorf(id.Pos(), "'%s' est une fonction native : impossible de %s", id.Name, action)
		} else {
			c.errorf(id.Pos(), "'%s' n'est pas déclaré", id.Name)
		}
	case sym.Kind == Func:
		c.errorf(id.Pos(), "'%s' est une fonction : impossible de %s", id.Name, action)
	default:
		c.info.Refs[id] = sym
	}
}

// ---------------------------------------------------------------------------
// Expressions

func (c *checker) checkExpr(e ast.Expr, s *scope) {
	switch e := e.(type) {
	case *ast.IntLit, *ast.BoolLit:

	case *ast.Ident:
		c.checkVar(e, s, "l'utiliser comme valeur")

	case *ast.QualifiedName:
		c.errorf(e.Pos(), "'%s' est une fonction native : il faut l'appeler", e)

	case *ast.UnaryExpr:
		c.checkExpr(e.X, s)

	case *ast.BinaryExpr:
		c.checkExpr(e.X, s)
		c.checkExpr(e.Y, s)

	case *ast.CallExpr:
		c.checkCall(e, s)

	default:
		panic(fmt.Sprintf("sema: expression inconnue %T", e))
	}
}

func (c *checker) checkCall(call *ast.CallExpr, s *scope) {
	for _, a := range call.Args {
		c.checkExpr(a, s)
	}
	if len(call.Args) > MaxArgs {
		c.errorf(call.Pos(), "trop d'arguments (max %d)", MaxArgs)
	}

	var sym *Symbol
	switch fn := call.Fn.(type) {
	case *ast.Ident:
		sym = s.lookup(fn.Name)
		if sym == nil {
			sym = nativeSymbol(fn.Name)
		}
		if sym == nil {
			c.errorf(fn.Pos(), "fonction '%s' non déclarée", fn.Name)
			return
		}
		if sym.Kind != Func && sym.Kind != Native {
			c.errorf(fn.Pos(), "'%s' est une %s, pas une fonction", fn.Name, sym.Kind)
			return
		}

	case *ast.QualifiedName:
		name := fn.String()
		sym = nativeSymbol(name)
		if sym == nil {
			if natives.IsModule(fn.Module.Name) {
				c.errorf(fn.Name.Pos(), "le module '%s' n'a pas de fonction '%s'", fn.Module.Name, fn.Name.Name)
			} else {
				c.errorf(fn.Pos(), "module '%s' inconnu", fn.Module.Name)
			}
			return
		}

	default:
		panic(fmt.Sprintf("sema: appel sur %T", fn))
	}

	if len(call.Args) != sym.Arity {
		c.errorf(call.Pos(), "%s attend %d argument(s), %d fourni(s)", call.Fn, sym.Arity, len(call.Args))
	}
	c.info.Calls[call] = sym
}

func nativeSymbol(name string) *Symbol {
	id, n, ok := natives.Lookup(name)
	if !ok {
		return nil
	}
	return &Symbol{Kind: Native, Name: name, Index: id, Arity: n.Arity}
}
