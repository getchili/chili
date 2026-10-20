package sema

import (
	"strings"
	"testing"

	"github.com/getchili/chili/compiler/ast"
	"github.com/getchili/chili/compiler/diag"
	"github.com/getchili/chili/compiler/parser"
)

// mainClass est ajouté aux sources de test qui ne portent pas sur le point d'entrée.
const mainClass = "\nclass Main { function main() {} }"

func mustParse(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, err := parser.Parse([]byte(src))
	if err != nil {
		t.Fatalf("erreur de parsing inattendue :\n%v\nsource :\n%s", err, src)
	}
	return prog
}

func funcNamed(info *Info, name string) *FuncInfo {
	for _, f := range info.Funcs {
		if f.Name == name {
			return f
		}
	}
	return nil
}

func TestValidProgram(t *testing.T) {
	src := `
let a = 1
let b = a + 2

class Main {
    function main() {
        let x = f(3)
        { let x = x + 1; print(x) }
        let y = x + b
        gpio.write(2, y)
        while y > 0 {
            y -= 1
            if y == 5 { break } else { continue }
        }
    }
}

// Appelée avant sa définition, et récursive.
function f(n) {
    if n == 0 { return 0 }
    let x = n - 1
    let x2 = x
    return f(x2)
}
`
	prog := mustParse(t, src)
	info, err := Check(prog)
	if err != nil {
		t.Fatalf("erreur inattendue :\n%v", err)
	}

	f := funcNamed(info, "f")
	main := funcNamed(info, "Main.main")
	if f == nil || main == nil || info.MainMethod != main || info.MainClass == nil {
		t.Fatalf("fonctions mal résolues : %+v", info.Funcs)
	}
	if n := main.NumLocals; n != 4 { // this, x, x (bloc), y
		t.Errorf("main : %d locales, attendu 4", n)
	}
	if n := f.NumLocals; n != 3 { // n, x, x2
		t.Errorf("f : %d locales, attendu 3", n)
	}

	// Globales.
	for i, name := range []string{"a", "b"} {
		sym := info.Refs[info.Globals[i].Name]
		if sym == nil || sym.Kind != Global || sym.Index != i || sym.Name != name {
			t.Errorf("globale %d : %+v", i, sym)
		}
	}

	// `{ let x = x + 1 }` : le x de droite est le slot 1, le nouveau x le slot 2.
	body := main.Body.Stmts
	inner := body[1].(*ast.Block).Stmts[0].(*ast.LetStmt)
	if sym := info.Refs[inner.Value.(*ast.BinaryExpr).X.(*ast.Ident)]; sym == nil || sym.Kind != Local || sym.Index != 1 {
		t.Errorf("x lu dans le bloc : %+v", sym)
	}
	if sym := info.Refs[inner.Name]; sym == nil || sym.Kind != Local || sym.Index != 2 {
		t.Errorf("x déclaré dans le bloc : %+v", sym)
	}

	// Appels : fonctions utilisateur et natives.
	calls := map[string]*Symbol{}
	for call, sym := range info.Calls {
		calls[call.Fn.String()] = sym
	}
	want := map[string]struct {
		kind  SymKind
		index int
	}{
		"f":          {Func, f.Index},
		"print":      {Native, 0},
		"gpio.write": {Native, 4},
	}
	for name, w := range want {
		sym := calls[name]
		if sym == nil || sym.Kind != w.kind || sym.Index != w.index {
			t.Errorf("appel %s : %+v, attendu %s %d", name, sym, w.kind, w.index)
		}
	}
}

func TestClasses(t *testing.T) {
	src := `
abstract class Animal {
    let pattes = 4
    let nom
    constructor(n) { this.nom = n }
    abstract function cri()
    function parle() { return this.cri() + this.pattes }
}

class Chien extends Animal {
    let age = 1
    constructor() { super(7) }
    function cri() { return 10 }
}

class Chiot extends Chien {
    function cri() { return super.cri() + 1 }
}

class Main {
    function main() {
        let c = new Chiot()
        print(c.parle())
    }
}
`
	info, err := Check(mustParse(t, src))
	if err != nil {
		t.Fatalf("erreur inattendue :\n%v", err)
	}
	classes := map[string]*ClassInfo{}
	for _, c := range info.Classes {
		classes[c.Name] = c
	}
	animal, chien, chiot := classes["Animal"], classes["Chien"], classes["Chiot"]

	// Champs : hérités d'abord.
	var names []string
	for _, f := range chiot.Fields {
		names = append(names, f.Name)
	}
	if got := strings.Join(names, ","); got != "pattes,nom,age" {
		t.Errorf("champs de Chiot : %s", got)
	}
	// Table de méthodes : cri redéfinie, parle héritée.
	if m := chiot.Methods["cri"]; m.Owner != chiot {
		t.Errorf("Chiot.cri vient de %s", m.Owner.Name)
	}
	if m := chiot.Methods["parle"]; m.Owner != animal {
		t.Errorf("Chiot.parle vient de %s", m.Owner.Name)
	}
	if !animal.Methods["cri"].Abstract {
		t.Error("Animal.cri devrait être abstraite")
	}
	// Constructeurs : Chiot hérite de celui de Chien.
	if chiot.EffCtor != chien.Ctor || chien.Ctor == nil {
		t.Errorf("constructeur de Chiot : %+v", chiot.EffCtor)
	}
	if n := chien.Ctor.NumParams(); n != 1 { // this
		t.Errorf("Chien.constructor : %d paramètres réels, attendu 1", n)
	}
}

func TestImplicitSuper(t *testing.T) {
	src := `
class A {
    let x
    constructor() { this.x = 1 }
}
class B extends A {
    constructor() { print(1) }
}` + mainClass
	info, err := Check(mustParse(t, src))
	if err != nil {
		t.Fatal(err)
	}
	b := funcNamed(info, "B.constructor")
	if b.ImplicitSuper == nil || b.ImplicitSuper.Name != "A.constructor" {
		t.Errorf("super() implicite : %+v", b.ImplicitSuper)
	}
}

func TestShadowingAllowed(t *testing.T) {
	src := `
let g = 1
function f() {
    let g = 2
    let a = 1
    { let a = 2 }
    if a { let a = 3 }
    while a { let a = 4; break }
}` + mainClass
	if _, err := Check(mustParse(t, src)); err != nil {
		t.Errorf("erreur inattendue :\n%v", err)
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string // fragments attendus, un par erreur, dans l'ordre
	}{
		{"non déclaré",
			"function f() {\n  print(z)\n}" + mainClass,
			[]string{"2:9: 'z' n'est pas déclaré"}},
		{"affectation non déclarée",
			"function f() {\n  z = 1\n}" + mainClass,
			[]string{"2:3: 'z' n'est pas déclaré"}},
		{"redéclaration même bloc",
			"function f() {\n  let a = 1\n  let a = 2\n}" + mainClass,
			[]string{"3:7: 'a' est déjà déclaré dans cette portée (variable locale, ligne 2)"}},
		{"paramètre redéclaré",
			"function f(a) {\n  let a = 1\n}" + mainClass,
			[]string{"2:7: 'a' est déjà déclaré dans cette portée (variable locale, ligne 1)"}},
		{"paramètres en double",
			"function f(a, a) {}" + mainClass,
			[]string{"1:15: 'a' est déjà déclaré"}},
		{"globale lue avant sa déclaration",
			"let a = b\nlet b = 1" + mainClass,
			[]string{"1:9: 'b' n'est pas déclaré"}},
		{"globale en conflit avec une fonction",
			"let f = 1\nfunction f() {}" + mainClass,
			[]string{"1:5: 'f' est déjà déclaré dans cette portée (fonction, ligne 2)"}},
		{"fonction en double",
			"function f() {}\nfunction f() {}" + mainClass,
			[]string{"2:10: 'f' est déjà déclaré dans cette portée (fonction, ligne 1)"}},
		{"appel d'une variable",
			"function f() {\n  let v = 1\n  v(2)\n}" + mainClass,
			[]string{"3:3: 'v' est une variable locale, pas une fonction"}},
		{"fonction utilisée comme valeur",
			"function f() {\n  let v = f\n}" + mainClass,
			[]string{"2:11: 'f' est une fonction : impossible de l'utiliser comme valeur"}},
		{"affectation d'une native",
			"function f() {\n  print = 1\n}" + mainClass,
			[]string{"2:3: 'print' est une fonction native : impossible de l'affecter"}},
		{"fonction non déclarée",
			"function f() {\n  foo()\n}" + mainClass,
			[]string{"2:3: fonction 'foo' non déclarée"}},
		{"arité fonction",
			"function f(a, b) { return a }\nfunction g() {\n  f(1)\n}" + mainClass,
			[]string{"3:3: f attend 2 argument(s), 1 fourni(s)"}},
		{"arité native",
			"function f() {\n  delay()\n}" + mainClass,
			[]string{"2:3: delay attend 1 argument(s), 0 fourni(s)"}},
		{"arité native qualifiée",
			"function f() {\n  gpio.write(2)\n}" + mainClass,
			[]string{"2:3: gpio.write attend 2 argument(s), 1 fourni(s)"}},
		{"fonction inconnue dans gpio",
			"function f() {\n  gpio.toggle(2)\n}" + mainClass,
			[]string{"2:8: le module 'gpio' n'a pas de fonction 'toggle'"}},
		{"native qualifiée non appelée",
			"function f() {\n  let v = gpio.write\n}" + mainClass,
			[]string{"2:11: 'gpio.write' est une fonction native : il faut l'appeler"}},
		{"nom réservé : native",
			"let delay = 1" + mainClass,
			[]string{"1:5: 'delay' est une fonction native, ce nom est réservé"}},
		{"nom réservé : module",
			"function gpio() {}" + mainClass,
			[]string{"1:10: 'gpio' est un module natif, ce nom est réservé"}},
		{"break hors boucle",
			"function f() {\n  break\n}" + mainClass,
			[]string{"2:3: 'break' en dehors d'une boucle"}},

		// Point d'entrée.
		{"Main absente",
			"function f() {}",
			[]string{"1:1: le programme doit définir la classe Main"}},
		{"Main sans main()",
			"class Main {\n  function start() {}\n}",
			[]string{"1:7: la classe Main doit avoir une méthode main()"}},
		{"main avec paramètres",
			"class Main {\n  function main(a) {}\n}",
			[]string{"2:12: main() ne doit pas avoir de paramètres (1 trouvés)"}},
		{"Main abstraite",
			"abstract class Main {\n  function main() {}\n}",
			[]string{"1:16: la classe Main ne peut pas être abstraite"}},

		// Classes.
		{"parent inconnu",
			"class A extends Z {}" + mainClass,
			[]string{"1:17: classe 'Z' non déclarée"}},
		{"héritage circulaire",
			"class A extends B {}\nclass B extends A {}" + mainClass,
			[]string{"héritage circulaire"}},
		{"champ redéclaré dans la fille",
			"class A { let x }\nclass B extends A { let x }" + mainClass,
			[]string{"2:25: le champ 'x' existe déjà dans la classe parente A"}},
		{"champ inconnu",
			"class A {\n  function f() { return this.y }\n}" + mainClass,
			[]string{"2:30: la classe A n'a pas de champ 'y'"}},
		{"this hors classe",
			"function f() {\n  return this\n}" + mainClass,
			[]string{"2:10: this n'est possible que dans une méthode"}},
		{"méthode abstraite dans classe concrète",
			"class A {\n  abstract function f()\n}" + mainClass,
			[]string{"2:3: la méthode abstraite 'f' doit être dans une abstract class"}},
		{"méthode abstraite non définie",
			"abstract class A {\n  abstract function f()\n}\nclass B extends A {}" + mainClass,
			[]string{"4:7: la classe B doit définir la méthode abstraite f() héritée de A"}},
		{"new d'une classe abstraite",
			"abstract class A {}\nfunction f() {\n  let a = new A()\n}" + mainClass,
			[]string{"3:15: A est abstraite"}},
		{"new : arité du constructeur",
			"class A {\n  constructor(x) {}\n}\nfunction f() {\n  let a = new A()\n}" + mainClass,
			[]string{"5:11: new A attend 1 argument(s) (son constructeur), 0 fourni(s)"}},
		{"redéfinition avec une autre arité",
			"class A {\n  function f(x) {}\n}\nclass B extends A {\n  function f() {}\n}" + mainClass,
			[]string{"5:12: B.f redéfinit A.f : elle doit avoir 1 paramètre(s), pas 0"}},
		{"super() manquant",
			"class A {\n  constructor(x) {}\n}\nclass B extends A {\n  constructor() {}\n}" + mainClass,
			[]string{"5:3: le constructeur de B doit appeler super(...)"}},
		{"super() hors constructeur",
			"class A {}\nclass B extends A {\n  function f() { super() }\n}" + mainClass,
			[]string{"3:18: super(…) ne s'utilise que dans un constructeur"}},
		{"super.m abstraite",
			"abstract class A {\n  abstract function f()\n}\nclass B extends A {\n  function f() { return super.f() }\n}" + mainClass,
			[]string{"5:31: A.f est abstraite"}},
		{"méthode inconnue sur this",
			"class A {\n  function f() { return this.g() }\n}" + mainClass,
			[]string{"2:30: la classe A n'a pas de méthode 'g'"}},
		{"méthode inexistante partout",
			"function f(o) {\n  o.vole()\n}" + mainClass,
			[]string{"2:5: aucune classe n'a de méthode 'vole'"}},
		{"méthode appelée sans this",
			"class A {\n  function f() {}\n  function g() { f() }\n}" + mainClass,
			[]string{"3:18: fonction 'f' non déclarée : pour appeler la méthode, écris this.f(…)"}},
		{"classe utilisée comme valeur",
			"class A {}\nfunction f() {\n  let a = A\n}" + mainClass,
			[]string{"3:11: 'A' est une classe"}},
		{"classe appelée comme une fonction",
			"class A {}\nfunction f() {\n  A()\n}" + mainClass,
			[]string{"3:3: 'A' est une classe : pour créer un objet, écris new A(…)"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Check(mustParse(t, tt.src))
			if err == nil {
				t.Fatalf("erreur attendue")
			}
			errs := err.(diag.ErrorList)
			if len(errs) != len(tt.want) {
				t.Fatalf("%d erreur(s), attendu %d :\n%v", len(errs), len(tt.want), err)
			}
			for i, frag := range tt.want {
				if !strings.Contains(errs[i].Error(), frag) {
					t.Errorf("erreur %d = %q, attendu contenant %q", i, errs[i].Error(), frag)
				}
			}
		})
	}
}

func TestStringsArraysForSema(t *testing.T) {
	src := `class Main {
    function main() {
        let t = [1, 2]
        for x in t { let y = x }
        for x in 0..len(t) { print(x) }
        let x = 5
    }
}`
	info, err := Check(mustParse(t, src))
	if err != nil {
		t.Fatal(err)
	}
	// this, t, 2 cachées + x, y, 2 cachées + x, puis x : 1 + 1 + 3 + 1 + 3 + 1
	if got := funcNamed(info, "Main.main").NumLocals; got != 10 {
		t.Errorf("NumLocals = %d, attendu 10", got)
	}
	if len(info.ForSlots) != 2 {
		t.Errorf("ForSlots : %d boucles, attendu 2", len(info.ForSlots))
	}

	bad := []struct{ src, want string }{
		{"let len = 1" + mainClass, "'len' est une fonction intégrée, ce nom est réservé"},
		{"function array(n) { }" + mainClass, "'array' est une fonction intégrée"},
		{"let str = 1" + mainClass, "'str' est un module natif"},
		{"class Main { function main() { for i in 0..3 { }\n print(i) } }", "'i' n'est pas déclaré"},
		{"class Main { function main() { for i in 0..i { } } }", "'i' n'est pas déclaré"},
		{"class Main { function main() { print(len(1, 2)) } }", "len attend 1 argument(s), 2 fourni(s)"},
		{"class Main { function main() { print(str.upper(\"a\")) } }", "le module 'str' n'a pas de fonction 'upper'"},
		{"class Main { function main() { let x = len } }", "'len' est une fonction intégrée : impossible de l'utiliser comme valeur"},
		{"class Main { function main() { for i in 0..3 { let i = 2 } } }", ""},
	}
	for _, tt := range bad {
		_, err := Check(mustParse(t, tt.src))
		if tt.want == "" {
			if err != nil {
				t.Errorf("%q : erreur inattendue %v", tt.src, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%q : erreur %v, attendu %q", tt.src, err, tt.want)
		}
	}
}
