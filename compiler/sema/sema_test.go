package sema

import (
	"strings"
	"testing"

	"github.com/getchili/chili/compiler/ast"
	"github.com/getchili/chili/compiler/diag"
	"github.com/getchili/chili/compiler/parser"
)

func mustParse(t *testing.T, src string) *ast.Program {
	t.Helper()
	prog, err := parser.Parse([]byte(src))
	if err != nil {
		t.Fatalf("erreur de parsing inattendue :\n%v\nsource :\n%s", err, src)
	}
	return prog
}

func TestValidProgram(t *testing.T) {
	src := `
let a = 1
let b = a + 2

fn main() {
    let x = f(3)
    { let x = x + 1; print(x) }
    let y = x + b
    gpio.write(2, y)
    while y > 0 {
        y -= 1
        if y == 5 { break } else { continue }
    }
}

// Appelée avant sa définition, et récursive.
fn f(n) {
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

	// Fonctions : index dans l'ordre du fichier, nombre de slots locaux.
	if len(info.Funcs) != 2 {
		t.Fatalf("%d fonctions, attendu 2", len(info.Funcs))
	}
	if info.Main == nil || info.Main.Index != 0 {
		t.Fatalf("main mal résolu : %+v", info.Main)
	}
	if got := info.Funcs[1].Decl.Name.Name; got != "f" {
		t.Errorf("fonction 1 = %s, attendu f", got)
	}
	if n := info.Funcs[0].NumLocals; n != 3 { // x, x (bloc), y
		t.Errorf("main : %d locales, attendu 3", n)
	}
	if n := info.Funcs[1].NumLocals; n != 3 { // n, x, x2
		t.Errorf("f : %d locales, attendu 3", n)
	}

	// Globales.
	if len(info.Globals) != 2 {
		t.Fatalf("%d globales, attendu 2", len(info.Globals))
	}
	for i, name := range []string{"a", "b"} {
		sym := info.Refs[info.Globals[i].Name]
		if sym == nil || sym.Kind != Global || sym.Index != i || sym.Name != name {
			t.Errorf("globale %d : %+v", i, sym)
		}
	}
	// Dans `let b = a + 2`, a désigne la globale 0.
	bInit := info.Globals[1].Value.(*ast.BinaryExpr).X.(*ast.Ident)
	if sym := info.Refs[bInit]; sym == nil || sym.Kind != Global || sym.Index != 0 {
		t.Errorf("a dans l'initialiseur de b : %+v", sym)
	}

	// `{ let x = x + 1 }` : le x de droite est le slot 0, le nouveau x le slot 1.
	body := info.Main.Decl.Body.Stmts
	inner := body[1].(*ast.Block).Stmts[0].(*ast.LetStmt)
	if sym := info.Refs[inner.Value.(*ast.BinaryExpr).X.(*ast.Ident)]; sym == nil || sym.Kind != Local || sym.Index != 0 {
		t.Errorf("x lu dans le bloc : %+v", sym)
	}
	if sym := info.Refs[inner.Name]; sym == nil || sym.Kind != Local || sym.Index != 1 {
		t.Errorf("x déclaré dans le bloc : %+v", sym)
	}
	// `let y = x + b` : x est de nouveau le slot 0, b la globale 1, y le slot 2.
	letY := body[2].(*ast.LetStmt)
	bin := letY.Value.(*ast.BinaryExpr)
	if sym := info.Refs[bin.X.(*ast.Ident)]; sym == nil || sym.Kind != Local || sym.Index != 0 {
		t.Errorf("x dans let y : %+v", sym)
	}
	if sym := info.Refs[bin.Y.(*ast.Ident)]; sym == nil || sym.Kind != Global || sym.Index != 1 {
		t.Errorf("b dans let y : %+v", sym)
	}
	if sym := info.Refs[letY.Name]; sym == nil || sym.Index != 2 {
		t.Errorf("y : %+v", sym)
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
		"f":          {Func, 1},
		"print":      {Native, 0},
		"gpio.write": {Native, 4},
	}
	for name, w := range want {
		sym := calls[name]
		if sym == nil || sym.Kind != w.kind || sym.Index != w.index {
			t.Errorf("appel %s : %+v, attendu %s %d", name, sym, w.kind, w.index)
		}
	}
	if len(calls) != len(want) {
		t.Errorf("appels résolus : %v", calls)
	}
}

func TestShadowingAllowed(t *testing.T) {
	src := `
let g = 1
fn main() {
    let g = 2
    let a = 1
    { let a = 2 }
    if a { let a = 3 }
    while a { let a = 4; break }
}
`
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
			"fn main() {\n  print(z)\n}",
			[]string{"2:9: 'z' n'est pas déclaré"}},
		{"affectation non déclarée",
			"fn main() {\n  z = 1\n}",
			[]string{"2:3: 'z' n'est pas déclaré"}},
		{"redéclaration même bloc",
			"fn main() {\n  let a = 1\n  let a = 2\n}",
			[]string{"3:7: 'a' est déjà déclaré dans cette portée (variable locale, ligne 2)"}},
		{"paramètre redéclaré",
			"fn f(a) {\n  let a = 1\n}\nfn main() {}",
			[]string{"2:7: 'a' est déjà déclaré dans cette portée (variable locale, ligne 1)"}},
		{"paramètres en double",
			"fn f(a, a) {}\nfn main() {}",
			[]string{"1:9: 'a' est déjà déclaré"}},
		{"globale lue avant sa déclaration",
			"let a = b\nlet b = 1\nfn main() {}",
			[]string{"1:9: 'b' n'est pas déclaré"}},
		{"globale en conflit avec une fonction",
			"let f = 1\nfn f() {}\nfn main() {}",
			[]string{"1:5: 'f' est déjà déclaré dans cette portée (fonction, ligne 2)"}},
		{"fonction en double",
			"fn main() {}\nfn main() {}",
			[]string{"2:4: 'main' est déjà déclaré dans cette portée (fonction, ligne 1)"}},
		{"appel d'une variable",
			"fn main() {\n  let v = 1\n  v(2)\n}",
			[]string{"3:3: 'v' est une variable locale, pas une fonction"}},
		{"appel d'une fonction masquée",
			"fn f() {}\nfn main() {\n  let f = 1\n  f()\n}",
			[]string{"4:3: 'f' est une variable locale, pas une fonction"}},
		{"fonction utilisée comme valeur",
			"fn main() {\n  let v = main\n}",
			[]string{"2:11: 'main' est une fonction : impossible de l'utiliser comme valeur"}},
		{"affectation d'une fonction",
			"fn main() {\n  main = 1\n}",
			[]string{"2:3: 'main' est une fonction : impossible de l'affecter"}},
		{"affectation d'une native",
			"fn main() {\n  print = 1\n}",
			[]string{"2:3: 'print' est une fonction native : impossible de l'affecter"}},
		{"fonction non déclarée",
			"fn main() {\n  foo()\n}",
			[]string{"2:3: fonction 'foo' non déclarée"}},
		{"arité fonction",
			"fn f(a, b) { return a }\nfn main() {\n  f(1)\n}",
			[]string{"3:3: f attend 2 argument(s), 1 fourni(s)"}},
		{"arité native",
			"fn main() {\n  delay()\n}",
			[]string{"2:3: delay attend 1 argument(s), 0 fourni(s)"}},
		{"arité native qualifiée",
			"fn main() {\n  gpio.write(2)\n}",
			[]string{"2:3: gpio.write attend 2 argument(s), 1 fourni(s)"}},
		{"module inconnu",
			"fn main() {\n  wifi.connect(1)\n}",
			[]string{"2:3: module 'wifi' inconnu"}},
		{"fonction inconnue dans gpio",
			"fn main() {\n  gpio.toggle(2)\n}",
			[]string{"2:8: le module 'gpio' n'a pas de fonction 'toggle'"}},
		{"native qualifiée non appelée",
			"fn main() {\n  let v = gpio.write\n}",
			[]string{"2:11: 'gpio.write' est une fonction native : il faut l'appeler"}},
		{"nom réservé : native",
			"let delay = 1\nfn main() {}",
			[]string{"1:5: 'delay' est une fonction native, ce nom est réservé"}},
		{"nom réservé : local",
			"fn main() {\n  let print = 1\n}",
			[]string{"2:7: 'print' est une fonction native, ce nom est réservé"}},
		{"nom réservé : module",
			"fn gpio() {}\nfn main() {}",
			[]string{"1:4: 'gpio' est un module natif, ce nom est réservé"}},
		{"break hors boucle",
			"fn main() {\n  break\n}",
			[]string{"2:3: 'break' en dehors d'une boucle"}},
		{"continue hors boucle",
			"fn main() {\n  if true { continue }\n}",
			[]string{"2:13: 'continue' en dehors d'une boucle"}},
		{"main absent",
			"fn f() {}",
			[]string{"1:1: le programme doit définir fn main()"}},
		{"main avec paramètres",
			"fn main(a) {}",
			[]string{"1:4: main ne doit pas avoir de paramètres (1 trouvés)"}},
		{"plusieurs erreurs triées",
			"fn main() {\n  foo()\n  break\n}\nlet a = zz",
			[]string{"2:3:", "3:3:", "5:9:"}},
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
