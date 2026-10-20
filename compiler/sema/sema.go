// Package sema fait l'analyse sémantique d'un programme Chili : résolution
// des noms, portées, arité des appels, classes (héritage, méthodes abstraites,
// constructeurs), point d'entrée Main.main, break/continue.
//
// Le résultat (Info) donne au générateur de code tout ce dont il a besoin :
// index des globales, slots des locales, index des fonctions, ID des natives,
// disposition des champs et tables de méthodes des classes.
package sema

import (
	"fmt"
	"sort"

	"github.com/getchili/chili/compiler/ast"
	"github.com/getchili/chili/compiler/diag"
	"github.com/getchili/chili/compiler/natives"
	"github.com/getchili/chili/compiler/token"
)

// Limites imposées par le format du bytecode (cf. docs/bytecode.md).
const (
	MaxLocals  = 255
	MaxArgs    = 254 // + this pour les méthodes
	MaxGlobals = 65535
	MaxFuncs   = 65535
	MaxFields  = 255
	MaxClasses = 255
)

type SymKind int

const (
	Global SymKind = iota
	Local
	Func
	Native
	Class
)

func (k SymKind) String() string {
	return [...]string{"variable globale", "variable locale", "fonction", "fonction native", "classe"}[k]
}

type Symbol struct {
	Kind  SymKind
	Name  string
	Index int       // globale : index ; locale : slot ; fonction : index ; native : ID ; classe : index
	Arity int       // fonctions et natives
	Pos   token.Pos // position de la déclaration (zéro pour les natives)
}

// FuncInfo décrit une fonction compilée : fonction globale, méthode ou constructeur.
type FuncInfo struct {
	Name      string // « f », « Chien.cri », « Chien.constructor »
	Pos       token.Pos
	Params    []*ast.Ident
	Body      *ast.Block
	Index     int
	NumLocals int        // paramètres compris (et this pour une méthode)
	Class     *ClassInfo // nil pour une fonction globale
	IsCtor    bool
	// ImplicitSuper : constructeur parent (sans paramètre) à appeler au début,
	// quand le constructeur n'appelle pas super(...) lui-même.
	ImplicitSuper *FuncInfo
}

// NumParams compte les paramètres réels, this compris pour une méthode.
func (f *FuncInfo) NumParams() int {
	if f.Class != nil {
		return len(f.Params) + 1
	}
	return len(f.Params)
}

type FieldInfo struct {
	Name  string
	Index int // slot dans l'objet (champs hérités d'abord)
	Decl  *ast.FieldDecl
	Owner *ClassInfo
}

type MethodInfo struct {
	Name     string
	Arity    int // sans this
	Abstract bool
	Func     *FuncInfo // nil si abstraite
	Owner    *ClassInfo
	Pos      token.Pos
}

type ClassInfo struct {
	Decl     *ast.ClassDecl
	Name     string
	Index    int
	Parent   *ClassInfo
	Abstract bool
	Fields   []*FieldInfo // tous, hérités compris ; index = slot
	fieldMap map[string]*FieldInfo
	// Methods est la table complète (héritées + redéfinies), par nom.
	Methods map[string]*MethodInfo
	Ctor    *FuncInfo // constructeur propre (nil si absent)
	EffCtor *FuncInfo // constructeur utilisé par new : propre ou hérité (nil : aucun)
	state   int       // 0 à faire, 1 en cours, 2 fait (détection de cycles)
}

// SortedMethods renvoie les méthodes concrètes, triées par nom.
func (ci *ClassInfo) SortedMethods() []*MethodInfo {
	var ms []*MethodInfo
	for _, m := range ci.Methods {
		if !m.Abstract {
			ms = append(ms, m)
		}
	}
	sort.Slice(ms, func(i, j int) bool { return ms[i].Name < ms[j].Name })
	return ms
}

type Info struct {
	// Refs associe chaque identifiant de variable (lecture, affectation ou
	// nom déclaré par let/paramètre) à son symbole.
	Refs map[*ast.Ident]*Symbol
	// Calls associe chaque appel de fonction à la fonction ou native appelée.
	Calls   map[*ast.CallExpr]*Symbol
	Funcs   []*FuncInfo    // fonctions globales, puis méthodes et constructeurs
	Globals []*ast.LetStmt // dans l'ordre du fichier ; index = position ici
	Classes []*ClassInfo   // dans l'ordre du fichier

	Fields       map[*ast.FieldExpr]*FieldInfo
	News         map[*ast.NewExpr]*ClassInfo
	SuperCalls   map[*ast.SuperCall]*FuncInfo // nil : la classe parente n'a pas de constructeur
	SuperMethods map[*ast.SuperMethodCall]*FuncInfo

	MainClass  *ClassInfo
	MainMethod *FuncInfo
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
	info    *Info
	errs    diag.ErrorList
	global  *scope
	classes map[string]*ClassInfo
	methods map[string][]int // nom de méthode -> arités existantes (toutes classes)

	// État de la fonction en cours d'analyse.
	fn        *FuncInfo
	loopDepth int
	superSeen bool // un super(...) a été rencontré dans le constructeur courant
}

// Check analyse prog. err est une diag.ErrorList si non nil ; info est
// toujours renvoyé (partiellement rempli en cas d'erreur).
func Check(prog *ast.Program) (*Info, error) {
	c := &checker{
		info: &Info{
			Refs:         map[*ast.Ident]*Symbol{},
			Calls:        map[*ast.CallExpr]*Symbol{},
			Fields:       map[*ast.FieldExpr]*FieldInfo{},
			News:         map[*ast.NewExpr]*ClassInfo{},
			SuperCalls:   map[*ast.SuperCall]*FuncInfo{},
			SuperMethods: map[*ast.SuperMethodCall]*FuncInfo{},
		},
		global:  newScope(nil),
		classes: map[string]*ClassInfo{},
		methods: map[string][]int{},
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

func (c *checker) addFunc(fi *FuncInfo) {
	fi.Index = len(c.info.Funcs)
	c.info.Funcs = append(c.info.Funcs, fi)
}

func (c *checker) checkProgram(prog *ast.Program) {
	// 1. Fonctions globales et classes : visibles partout, même avant leur définition.
	for _, d := range prog.Decls {
		switch d := d.(type) {
		case *ast.FnDecl:
			fi := &FuncInfo{Name: d.Name.Name, Pos: d.Name.Pos(), Params: d.Params, Body: d.Body}
			c.addFunc(fi)
			c.declare(c.global, d.Name, &Symbol{
				Kind: Func, Name: d.Name.Name, Index: fi.Index,
				Arity: len(d.Params), Pos: d.Name.Pos(),
			})
		case *ast.ClassDecl:
			ci := &ClassInfo{Decl: d, Name: d.Name.Name, Index: len(c.info.Classes), Abstract: d.Abstract}
			c.info.Classes = append(c.info.Classes, ci)
			if _, dup := c.classes[ci.Name]; !dup {
				c.classes[ci.Name] = ci
			}
			c.declare(c.global, d.Name, &Symbol{Kind: Class, Name: ci.Name, Index: ci.Index, Pos: d.Name.Pos()})
		}
	}
	if len(c.info.Classes) > MaxClasses {
		c.errorf(token.Pos{Line: 1, Col: 1}, "trop de classes (max %d)", MaxClasses)
	}

	// 2. Classes : parents, champs, méthodes, constructeurs (parents d'abord).
	for _, ci := range c.info.Classes {
		c.resolveClass(ci)
	}
	for _, ci := range c.info.Classes {
		c.checkAbstracts(ci)
	}

	// 3. Les globales, dans l'ordre : un initialiseur ne voit que les
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

	// 4. Initialiseurs de champs (portée globale, pas de this).
	for _, ci := range c.info.Classes {
		for _, f := range ci.Decl.Fields {
			if f.Value != nil {
				c.checkExpr(f.Value, c.global)
			}
		}
	}

	// 5. Les corps de fonctions, méthodes et constructeurs.
	for _, fi := range c.info.Funcs {
		c.checkFunc(fi)
	}
	if len(c.info.Funcs) > MaxFuncs {
		c.errorf(token.Pos{Line: 1, Col: 1}, "trop de fonctions (max %d)", MaxFuncs)
	}

	c.checkMain()
}

// ---------------------------------------------------------------------------
// Classes

func (c *checker) resolveClass(ci *ClassInfo) {
	switch ci.state {
	case 2:
		return
	case 1:
		c.errorf(ci.Decl.Name.Pos(), "héritage circulaire : %s finit par hériter de lui-même", ci.Name)
		return
	}
	ci.state = 1
	d := ci.Decl
	ci.fieldMap = map[string]*FieldInfo{}
	ci.Methods = map[string]*MethodInfo{}

	if d.Parent != nil {
		p, ok := c.classes[d.Parent.Name]
		switch {
		case !ok:
			c.errorf(d.Parent.Pos(), "classe '%s' non déclarée", d.Parent.Name)
		case p == ci:
			c.errorf(d.Parent.Pos(), "une classe ne peut pas hériter d'elle-même")
		default:
			c.resolveClass(p)
			if p.state == 2 {
				ci.Parent = p
			}
		}
	}

	// Champs : hérités d'abord, puis propres.
	if ci.Parent != nil {
		ci.Fields = append(ci.Fields, ci.Parent.Fields...)
		for k, v := range ci.Parent.fieldMap {
			ci.fieldMap[k] = v
		}
	}
	for _, f := range d.Fields {
		if prev, dup := ci.fieldMap[f.Name.Name]; dup {
			if prev.Owner == ci {
				c.errorf(f.Name.Pos(), "le champ '%s' est déjà déclaré dans %s", f.Name.Name, ci.Name)
			} else {
				c.errorf(f.Name.Pos(), "le champ '%s' existe déjà dans la classe parente %s", f.Name.Name, prev.Owner.Name)
			}
			continue
		}
		fi := &FieldInfo{Name: f.Name.Name, Index: len(ci.Fields), Decl: f, Owner: ci}
		ci.Fields = append(ci.Fields, fi)
		ci.fieldMap[fi.Name] = fi
	}
	if len(ci.Fields) > MaxFields {
		c.errorf(d.Name.Pos(), "trop de champs dans %s (max %d)", ci.Name, MaxFields)
	}

	// Méthodes : table du parent, puis redéfinitions et ajouts.
	if ci.Parent != nil {
		for k, v := range ci.Parent.Methods {
			ci.Methods[k] = v
		}
	}
	own := map[string]bool{}
	for _, m := range d.Methods {
		name := m.Name.Name
		if own[name] {
			c.errorf(m.Name.Pos(), "la méthode '%s' est déjà déclarée dans %s", name, ci.Name)
			continue
		}
		own[name] = true
		if len(m.Params) > MaxArgs {
			c.errorf(m.Name.Pos(), "trop de paramètres (max %d)", MaxArgs)
		}
		if m.Abstract && !ci.Abstract {
			c.errorf(m.Pos(), "la méthode abstraite '%s' doit être dans une abstract class (%s ne l'est pas)", name, ci.Name)
		}
		if prev, ok := ci.Methods[name]; ok && len(m.Params) != prev.Arity {
			c.errorf(m.Name.Pos(), "%s.%s redéfinit %s.%s : elle doit avoir %d paramètre(s), pas %d",
				ci.Name, name, prev.Owner.Name, name, prev.Arity, len(m.Params))
		}
		mi := &MethodInfo{Name: name, Arity: len(m.Params), Abstract: m.Abstract, Owner: ci, Pos: m.Name.Pos()}
		if !m.Abstract {
			mi.Func = &FuncInfo{Name: ci.Name + "." + name, Pos: m.Name.Pos(), Params: m.Params, Body: m.Body, Class: ci}
			c.addFunc(mi.Func)
		}
		ci.Methods[name] = mi
		c.methods[name] = append(c.methods[name], len(m.Params))
	}

	// Constructeur.
	if d.Ctor != nil {
		if len(d.Ctor.Params) > MaxArgs {
			c.errorf(d.Ctor.Pos(), "trop de paramètres (max %d)", MaxArgs)
		}
		ci.Ctor = &FuncInfo{Name: ci.Name + ".constructor", Pos: d.Ctor.Pos(), Params: d.Ctor.Params,
			Body: d.Ctor.Body, Class: ci, IsCtor: true}
		c.addFunc(ci.Ctor)
		ci.EffCtor = ci.Ctor
	} else if ci.Parent != nil {
		ci.EffCtor = ci.Parent.EffCtor
	}
	ci.state = 2
}

// checkAbstracts vérifie qu'une classe concrète définit toutes les méthodes abstraites.
func (c *checker) checkAbstracts(ci *ClassInfo) {
	if ci.Abstract || ci.Methods == nil {
		return
	}
	var missing []*MethodInfo
	for _, m := range ci.Methods {
		if m.Abstract && m.Owner != ci { // une abstraite déclarée ici est déjà signalée
			missing = append(missing, m)
		}
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i].Name < missing[j].Name })
	for _, m := range missing {
		c.errorf(ci.Decl.Name.Pos(), "la classe %s doit définir la méthode abstraite %s() héritée de %s (ou être déclarée abstract)",
			ci.Name, m.Name, m.Owner.Name)
	}
}

func (c *checker) checkMain() {
	pos := token.Pos{Line: 1, Col: 1}
	mc, ok := c.classes["Main"]
	if !ok {
		c.errorf(pos, "le programme doit définir la classe Main avec une méthode main() : class Main { function main() { … } }")
		return
	}
	c.info.MainClass = mc
	if mc.Abstract {
		c.errorf(mc.Decl.Name.Pos(), "la classe Main ne peut pas être abstraite")
	}
	if mc.EffCtor != nil && len(mc.EffCtor.Params) != 0 {
		c.errorf(mc.Decl.Name.Pos(), "le constructeur de Main ne doit pas avoir de paramètres")
	}
	m, ok := mc.Methods["main"]
	switch {
	case !ok || m.Abstract:
		c.errorf(mc.Decl.Name.Pos(), "la classe Main doit avoir une méthode main()")
	case m.Arity != 0:
		c.errorf(m.Pos, "main() ne doit pas avoir de paramètres (%d trouvés)", m.Arity)
	default:
		c.info.MainMethod = m.Func
	}
}

// ---------------------------------------------------------------------------
// Fonctions

func (c *checker) checkFunc(fi *FuncInfo) {
	c.fn = fi
	c.loopDepth = 0
	c.superSeen = false
	// Les paramètres et le corps partagent la même portée :
	// function f(a) { let a = 1 } est une redéclaration.
	s := newScope(c.global)
	if fi.Class != nil {
		fi.NumLocals = 1 // slot 0 : this
	}
	if len(fi.Params) > MaxArgs {
		c.errorf(fi.Pos, "trop de paramètres (max %d)", MaxArgs)
	}
	for _, p := range fi.Params {
		c.declareLocal(s, p)
	}
	c.checkStmts(fi.Body.Stmts, s)

	if fi.IsCtor && !c.superSeen && fi.Class.Parent != nil {
		if pc := fi.Class.Parent.EffCtor; pc != nil {
			if len(pc.Params) == 0 {
				fi.ImplicitSuper = pc
			} else {
				c.errorf(fi.Pos, "le constructeur de %s doit appeler super(...) : %s attend %d paramètre(s)",
					fi.Class.Name, fi.Class.Parent.Name, len(pc.Params))
			}
		}
	}
	c.fn = nil
}

func (c *checker) declareLocal(s *scope, id *ast.Ident) {
	sym := &Symbol{Kind: Local, Name: id.Name, Index: c.fn.NumLocals, Pos: id.Pos()}
	c.fn.NumLocals++
	if c.fn.NumLocals == MaxLocals+1 {
		c.errorf(id.Pos(), "trop de variables locales dans %s (max %d)", c.fn.Name, MaxLocals)
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
		switch t := st.Target.(type) {
		case *ast.Ident:
			c.checkVar(t, s, "l'affecter")
		case *ast.FieldExpr:
			c.checkField(t)
		}

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
	case sym.Kind == Class:
		c.errorf(id.Pos(), "'%s' est une classe : impossible de %s (pour créer un objet : new %s(…))", id.Name, action, id.Name)
	default:
		c.info.Refs[id] = sym
	}
}

// currentClass renvoie la classe de la méthode en cours, ou signale l'erreur.
func (c *checker) currentClass(pos token.Pos, what string) *ClassInfo {
	if c.fn == nil || c.fn.Class == nil {
		c.errorf(pos, "%s n'est possible que dans une méthode ou un constructeur de classe", what)
		return nil
	}
	return c.fn.Class
}

func (c *checker) checkField(e *ast.FieldExpr) {
	ci := c.currentClass(e.Pos(), "this")
	if ci == nil {
		return
	}
	f, ok := ci.fieldMap[e.Name.Name]
	if !ok {
		if _, isMethod := ci.Methods[e.Name.Name]; isMethod {
			c.errorf(e.Name.Pos(), "'%s' est une méthode de %s : il faut l'appeler, this.%s(…)", e.Name.Name, ci.Name, e.Name.Name)
		} else {
			c.errorf(e.Name.Pos(), "la classe %s n'a pas de champ '%s'", ci.Name, e.Name.Name)
		}
		return
	}
	c.info.Fields[e] = f
}

// ---------------------------------------------------------------------------
// Expressions

func (c *checker) checkArgs(args []ast.Expr, s *scope, pos token.Pos) {
	for _, a := range args {
		c.checkExpr(a, s)
	}
	if len(args) > MaxArgs {
		c.errorf(pos, "trop d'arguments (max %d)", MaxArgs)
	}
}

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

	case *ast.ThisExpr:
		c.currentClass(e.Pos(), "this")

	case *ast.FieldExpr:
		c.checkField(e)

	case *ast.NewExpr:
		c.checkArgs(e.Args, s, e.Pos())
		ci, ok := c.classes[e.Class.Name]
		if !ok {
			c.errorf(e.Class.Pos(), "classe '%s' non déclarée", e.Class.Name)
			return
		}
		if ci.Abstract {
			c.errorf(e.Class.Pos(), "%s est abstraite : on ne peut pas en créer d'objet, crée plutôt une classe fille", ci.Name)
		}
		want := 0
		if ci.EffCtor != nil {
			want = len(ci.EffCtor.Params)
		}
		if len(e.Args) != want {
			c.errorf(e.Pos(), "new %s attend %d argument(s) (son constructeur), %d fourni(s)", ci.Name, want, len(e.Args))
		}
		c.info.News[e] = ci

	case *ast.MethodCall:
		c.checkExpr(e.Recv, s)
		c.checkArgs(e.Args, s, e.Pos())
		name := e.Name.Name
		if _, isThis := e.Recv.(*ast.ThisExpr); isThis && c.fn != nil && c.fn.Class != nil {
			ci := c.fn.Class
			m, ok := ci.Methods[name]
			if !ok {
				c.errorf(e.Name.Pos(), "la classe %s n'a pas de méthode '%s'", ci.Name, name)
			} else if len(e.Args) != m.Arity {
				c.errorf(e.Name.Pos(), "%s.%s attend %d argument(s), %d fourni(s)", ci.Name, name, m.Arity, len(e.Args))
			}
			return
		}
		arities, ok := c.methods[name]
		if !ok {
			c.errorf(e.Name.Pos(), "aucune classe n'a de méthode '%s'", name)
			return
		}
		for _, a := range arities {
			if a == len(e.Args) {
				return
			}
		}
		c.errorf(e.Name.Pos(), "la méthode '%s' n'accepte pas %d argument(s)", name, len(e.Args))

	case *ast.SuperCall:
		c.checkArgs(e.Args, s, e.Pos())
		if c.fn == nil || !c.fn.IsCtor {
			c.errorf(e.Pos(), "super(…) ne s'utilise que dans un constructeur")
			return
		}
		ci := c.fn.Class
		if ci.Parent == nil {
			c.errorf(e.Pos(), "%s n'hérite d'aucune classe : super(…) est impossible", ci.Name)
			return
		}
		if c.superSeen {
			c.errorf(e.Pos(), "super(…) est déjà appelé dans ce constructeur")
		}
		c.superSeen = true
		pc := ci.Parent.EffCtor
		want := 0
		if pc != nil {
			want = len(pc.Params)
		}
		if len(e.Args) != want {
			c.errorf(e.Pos(), "super(…) : le constructeur de %s attend %d argument(s), %d fourni(s)", ci.Parent.Name, want, len(e.Args))
		}
		c.info.SuperCalls[e] = pc

	case *ast.SuperMethodCall:
		c.checkArgs(e.Args, s, e.Pos())
		ci := c.currentClass(e.Pos(), "super")
		if ci == nil {
			return
		}
		if ci.Parent == nil {
			c.errorf(e.Pos(), "%s n'hérite d'aucune classe : super.%s() est impossible", ci.Name, e.Name.Name)
			return
		}
		m, ok := ci.Parent.Methods[e.Name.Name]
		switch {
		case !ok:
			c.errorf(e.Name.Pos(), "la classe %s n'a pas de méthode '%s'", ci.Parent.Name, e.Name.Name)
		case m.Abstract:
			c.errorf(e.Name.Pos(), "%s.%s est abstraite : super.%s() est impossible", ci.Parent.Name, e.Name.Name, e.Name.Name)
		case len(e.Args) != m.Arity:
			c.errorf(e.Name.Pos(), "%s.%s attend %d argument(s), %d fourni(s)", ci.Parent.Name, e.Name.Name, m.Arity, len(e.Args))
		default:
			c.info.SuperMethods[e] = m.Func
		}

	default:
		panic(fmt.Sprintf("sema: expression inconnue %T", e))
	}
}

func (c *checker) checkCall(call *ast.CallExpr, s *scope) {
	c.checkArgs(call.Args, s, call.Pos())

	var sym *Symbol
	switch fn := call.Fn.(type) {
	case *ast.Ident:
		sym = s.lookup(fn.Name)
		if sym == nil {
			sym = nativeSymbol(fn.Name)
		}
		if sym == nil {
			if c.fn != nil && c.fn.Class != nil {
				if _, isMethod := c.fn.Class.Methods[fn.Name]; isMethod {
					c.errorf(fn.Pos(), "fonction '%s' non déclarée : pour appeler la méthode, écris this.%s(…)", fn.Name, fn.Name)
					return
				}
			}
			c.errorf(fn.Pos(), "fonction '%s' non déclarée", fn.Name)
			return
		}
		if sym.Kind == Class {
			c.errorf(fn.Pos(), "'%s' est une classe : pour créer un objet, écris new %s(…)", fn.Name, fn.Name)
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
			c.errorf(fn.Name.Pos(), "le module '%s' n'a pas de fonction '%s'", fn.Module.Name, fn.Name.Name)
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
