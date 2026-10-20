package parser

import (
	"strings"
	"testing"
)

func TestExprPrecedence(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{"1 + 2 * 3", "(+ 1 (* 2 3))"},
		{"(1 + 2) * 3", "(* (+ 1 2) 3)"},
		{"1 - 2 - 3", "(- (- 1 2) 3)"},
		{"a || b && c", "(|| a (&& b c))"},
		{"a == b < c", "(== a (< b c))"},
		{"a | b ^ c & d", "(| a (^ b (& c d)))"},
		{"a & b == c", "(== (& a b) c)"},
		{"1 << 2 + 3", "(<< 1 (+ 2 3))"},
		{"-a * b", "(* (- a) b)"},
		{"!!x", "(! (! x))"},
		{"~0 & 0xFF", "(& (~ 0) 255)"},
		{"f()", "(call f)"},
		{"f(1, g(2), a + b)", "(call f 1 (call g 2) (+ a b))"},
		{"gpio.read(4) == 1", "(== (call gpio.read 4) 1)"},
		{"true && false", "(&& true false)"},
		{"0b1010 + 1_000", "(+ 10 1000)"},
		{"0xFFFFFFFF", "-1"},
		{"x\n", "x"},
	}
	for _, tt := range tests {
		e, err := ParseExpr(tt.src)
		if err != nil {
			t.Errorf("%q : erreur inattendue : %v", tt.src, err)
			continue
		}
		if got := e.String(); got != tt.want {
			t.Errorf("%q\n got: %s\nwant: %s", tt.src, got, tt.want)
		}
	}
}

func TestProgram(t *testing.T) {
	src := `
// Clignotement de la LED intégrée
let led_pin = 2
let count = 0

function toggle(pin, state) {
    gpio.write(pin, state)
    return !state
}

function main() {
    gpio.mode(led_pin, 1)
    let on = 0
    while true {
        on = toggle(led_pin, on)
        count += 1
        if count > 10 {
            break
        } else if count % 2 == 0 {
            continue
        } else {
            delay(500)
        }
    }
    { let x = 1; print(x) }
    return
}
`
	want := strings.Join([]string{
		"(let led_pin 2)",
		"(let count 0)",
		"(function toggle (pin state) (block (call gpio.write pin state) (return (! state))))",
		"(function main () (block" +
			" (call gpio.mode led_pin 1)" +
			" (let on 0)" +
			" (while true (block" +
			" (= on (call toggle led_pin on))" +
			" (+= count 1)" +
			" (if (> count 10) (block (break))" +
			" (if (== (% count 2) 0) (block (continue)) (block (call delay 500))))))" +
			" (block (let x 1) (call print x))" +
			" (return)))",
	}, "\n")

	prog, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("erreur inattendue :\n%v", err)
	}
	if got := prog.String(); got != want {
		t.Errorf("\n got: %s\nwant: %s", got, want)
	}
}

func TestOneLiners(t *testing.T) {
	prog, err := Parse([]byte("function f(a) { return a + 1 }\nfunction main() { print(f(1)) }"))
	if err != nil {
		t.Fatal(err)
	}
	want := "(function f (a) (block (return (+ a 1))))\n(function main () (block (call print (call f 1))))"
	if got := prog.String(); got != want {
		t.Errorf("\n got: %s\nwant: %s", got, want)
	}
}

func TestErrors(t *testing.T) {
	tests := []struct {
		src  string
		want []string // fragments attendus, un par erreur, dans l'ordre
	}{
		{"x = 1", []string{"1:1: 'class', 'function' ou 'let' attendu au niveau global, trouvé identifiant 'x'"}},
		{"let = 3", []string{"1:5: identifiant attendu, trouvé '='"}},
		{"function main() {\n  let x = \n}", []string{"3:1: expression attendue, trouvé '}'"}},
		{"function main() {\n  1 + 2\n}", []string{"2:3: expression inutilisée"}},
		{"function main() {\n  f() = 3\n}", []string{"2:7: on ne peut affecter qu'une variable"}},
		{"function main() {\n  function g() {}\n}", []string{"2:3: les fonctions et les classes se déclarent au niveau global"}},
		{"function main() {\n  let x = 99999999999\n}", []string{"2:11: entier hors limites"}},
		{"function main() {\n  f(1)(2)\n}", []string{"2:7: seule une fonction nommée peut être appelée"}},
		{"function main() {\n  print(1 2)\n}", []string{"2:11: ')' attendu, trouvé nombre 2"}},
		{"function main() {\n  if x\n  {\n  }\n}", []string{"2:7: '{' attendu, trouvé fin de ligne"}},
		{"function main() {\n  print(1)", []string{"2:11: '}' attendu, trouvé fin de fichier"}},
		{"function main() {\n  let a = 1 $ 2\n}", []string{"2:13: caractère inattendu '$'"}},
		// Récupération : plusieurs erreurs indépendantes sont toutes signalées.
		{"function main() {\n  let = 1\n  let b = )\n  print(3)\n}\nlet",
			[]string{"2:7:", "3:11:", "6:4:"}},
	}
	for _, tt := range tests {
		_, err := Parse([]byte(tt.src))
		if err == nil {
			t.Errorf("%q : erreur attendue", tt.src)
			continue
		}
		errs := err.(ErrorList)
		if len(errs) != len(tt.want) {
			t.Errorf("%q : %d erreur(s), attendu %d :\n%v", tt.src, len(errs), len(tt.want), err)
			continue
		}
		for i, frag := range tt.want {
			if !strings.Contains(errs[i].Error(), frag) {
				t.Errorf("%q : erreur %d = %q, attendu contenant %q", tt.src, i, errs[i].Error(), frag)
			}
		}
	}
}

func TestClasses(t *testing.T) {
	src := `
abstract class Animal {
    let pattes = 4
    let nom
    constructor(n) {
        this.nom = n
    }
    abstract function cri()
    function parle() {
        print(this.cri())
        this.pattes += 1
    }
}

class Chien extends Animal {
    constructor() { super(3) }
    function cri() { return super.cri() }
}

class Main {
    function main() {
        let c = new Chien()
        c.parle()
        new Chien().parle()
    }
}
`
	want := strings.Join([]string{
		"(abstract class Animal (field pattes 4) (field nom)" +
			" (method constructor (n) (block (= this.nom n)))" +
			" (abstract cri ())" +
			" (method parle () (block (call print (callm this cri)) (+= this.pattes 1))))",
		"(class Chien extends Animal (method constructor () (block (super 3)))" +
			" (method cri () (block (return (super.cri)))))",
		"(class Main (method main () (block (let c (new Chien)) (callm c parle) (callm (new Chien) parle))))",
	}, "\n")
	prog, err := Parse([]byte(src))
	if err != nil {
		t.Fatalf("erreur inattendue :\n%v", err)
	}
	if got := prog.String(); got != want {
		t.Errorf("\n got: %s\nwant: %s", got, want)
	}
}

func TestClassErrors(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{"fn main() {}", "1:1: 'fn' n'existe plus : écris 'function'"},
		{"class A {\n  print(1)\n}", "2:3: dans une classe : 'let', 'constructor' ou 'function' attendu"},
		{"abstract class A {\n  abstract function f() {}\n}", "2:25: une méthode abstraite n'a pas de corps"},
		{"class A {\n  constructor() {}\n  constructor() {}\n}", "3:3: la classe A a déjà un constructeur"},
		{"function f(o) {\n  o.x = 1\n}", "2:5: les champs sont privés : écris this.x"},
		{"function f() {\n  class B {}\n}", "2:3: les fonctions et les classes se déclarent au niveau global"},
	}
	for _, tt := range tests {
		_, err := Parse([]byte(tt.src))
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%q : erreur %v, attendu %q", tt.src, err, tt.want)
		}
	}
}
