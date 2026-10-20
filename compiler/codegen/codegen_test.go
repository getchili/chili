package codegen

import (
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/getchili/chili/compiler/bytecode"
	"github.com/getchili/chili/compiler/natives"
	"github.com/getchili/chili/compiler/refvm"
)

// run compile src, passe par l'encodage binaire (aller-retour), exécute le
// programme et renvoie ce qu'il a affiché avec print, un appel par ligne.
func run(t *testing.T, src string) (string, error) {
	t.Helper()
	prog, err := Compile([]byte(legacy(src)))
	if err != nil {
		t.Fatalf("compilation :\n%v", err)
	}
	decoded, err := bytecode.Decode(prog.Encode())
	if err != nil {
		t.Fatalf("décodage : %v", err)
	}
	if err := bytecode.Verify(decoded, natives.Arity); err != nil {
		t.Fatalf("vérification : %v", err)
	}

	var out strings.Builder
	printID, _, _ := natives.Lookup("print")
	randomID, _, _ := natives.Lookup("random")
	printcID, _, _ := natives.Lookup("printc")
	printlnID, _, _ := natives.Lookup("println")
	vm := refvm.New(decoded, func(id int, args []int32) (int32, error) {
		switch id {
		case printID:
			fmt.Fprintln(&out, args[0])
		case randomID:
			return natives.RandomRange(args[0], args[1], rand.Uint32())
		case printcID:
			if err := natives.CheckChar(args[0]); err != nil {
				return 0, err
			}
			out.WriteString(string(rune(args[0])))
		case printlnID:
			out.WriteString("\n")
		}
		return 0, nil
	})
	vm.MaxSteps = 1_000_000
	err = vm.Run()
	return strings.TrimSpace(out.String()), err
}

// programTests sert aussi au test croisé avec la VM C (cvm_test.go).
var programTests = []struct {
	name string
	src  string
	want string // sorties de print séparées par des espaces
}{
	{"arithmétique", `fn main() {
			print(1 + 2 * 3)
			print((1 + 2) * 3)
			print(7 / 2)
			print(-7 / 2)
			print(7 % 3)
			print(-7 % 3)
			print(2 - 5)
			print(-(3))
		}`, "7 9 3 -3 1 -1 -3 -3"},

	{"bits", `fn main() {
			print(6 & 3)
			print(6 | 3)
			print(6 ^ 3)
			print(~0)
			print(1 << 4)
			print(-16 >> 2)
			print(1 << 33)
			print(0xFF00 >> 8)
		}`, "2 7 5 -1 16 -4 2 255"},

	{"comparaisons et logique", `fn main() {
			print(1 < 2)
			print(2 <= 1)
			print(3 == 3)
			print(3 != 3)
			print(!0)
			print(!5)
			print(5 && 7)
			print(0 || 9)
			print(0 && 1)
			print(0 || 0)
		}`, "1 0 1 0 1 0 1 1 0 0"},

	{"court-circuit", `
		fn side(v) {
			print(v)
			return v
		}
		fn main() {
			let a = side(0) && side(1)
			let b = side(2) || side(3)
			let c = side(4) && side(0)
			print(a + b * 10 + c * 100)
		}`, "0 2 4 0 10"},

	{"débordement 32 bits", `fn main() {
			let max = 0x7FFFFFFF
			print(max + 1)
			let min = 0x80000000
			print(min / -1)
			print(min % -1)
			print(100000 * 100000)
		}`, "-2147483648 -2147483648 0 1410065408"},

	{"globales initialisées dans l'ordre", `
		let a = 10
		let b = a * 2
		let c = twice(b)
		fn twice(x) { return x * 2 }
		fn main() {
			print(a)
			print(b)
			print(c)
			a += 1
			bump()
			print(a)
		}
		fn bump() { a *= 3 }`, "10 20 40 33"},

	{"boucles, break, continue", `fn main() {
			let i = 0
			let sum = 0
			while true {
				i += 1
				if i > 10 { break }
				if i % 2 == 0 { continue }
				sum += i
			}
			print(sum)
			print(i)
		}`, "25 11"},

	{"boucles imbriquées", `fn main() {
			let i = 0
			let n = 0
			while i < 4 {
				let j = 0
				while j < 4 {
					j += 1
					if j == 3 { break }
					n += 1
				}
				i += 1
			}
			print(n)
		}`, "8"},

	{"if / else if / else", `
		fn sign(x) {
			if x < 0 {
				return -1
			} else if x == 0 {
				return 0
			} else {
				return 1
			}
		}
		fn main() {
			print(sign(-5))
			print(sign(0))
			print(sign(42))
		}`, "-1 0 1"},

	{"récursion", `
		fn fib(n) {
			if n < 2 { return n }
			return fib(n - 1) + fib(n - 2)
		}
		fn fact(n) {
			if n <= 1 { return 1 }
			return n * fact(n - 1)
		}
		fn main() {
			print(fib(20))
			print(fact(10))
		}`, "6765 3628800"},

	{"masquage et portées", `
		let x = 1
		fn main() {
			print(x)
			let x = x + 10
			print(x)
			{
				let x = x * 2
				print(x)
			}
			print(x)
		}`, "1 11 22 11"},

	{"retour implicite et paramètres", `
		fn nothing() {}
		fn early(a) {
			if a { return }
			return 7
		}
		fn sub(a, b) { return a - b }
		fn main() {
			print(nothing())
			print(early(1))
			print(early(0))
			print(sub(10, 3))
		}`, "0 0 7 7"},

	{"affectations composées", `fn main() {
			let x = 10
			x += 5
			x -= 3
			x *= 4
			x /= 6
			x %= 5
			print(x)
		}`, "3"},

	{"grandes constantes", `fn main() {
			print(127)
			print(128)
			print(-128)
			print(-129)
			print(1_000_000)
			print(0b1010)
		}`, "127 128 -128 -129 1000000 10"},

	{"printc et println", `
class Main {
    function main() {
        printc(72)
        printc(105)
        printc(0x00E9)
        println()
        printc(79)
        printc(75)
        println()
        print(3)
    }
}`, "Hié OK 3"},

	{"random", `fn main() {
			let i = 0
			let ok = 1
			let vu1 = 0
			let vu6 = 0
			while i < 2000 {
				let d = random(1, 6)
				if d < 1 || d > 6 { ok = 0 }
				if d == 1 { vu1 = 1 }
				if d == 6 { vu6 = 1 }
				i += 1
			}
			print(ok)
			print(vu1 + vu6)
			print(random(7, 7))
			print(random(-3, -3))
			let x = random(0x80000000, 0x7FFFFFFF)
			print(1)
		}`, "1 2 7 -3 1"},

	{"classes : polymorphisme et abstract", `
abstract class Forme {
    let cote = 0
    constructor(c) { this.cote = c }
    abstract function aire()
    function decrire() { print(this.aire()) }
}
class Carre extends Forme {
    function aire() { return this.cote * this.cote }
}
class Triangle extends Forme {
    let h = 2
    function aire() { return this.cote * this.h / 2 }
}
class Main {
    function main() {
        let a = new Carre(5)
        let b = new Triangle(6)
        a.decrire()
        b.decrire()
        print(a.aire() + b.aire())
    }
}`, "25 6 31"},

	{"classes : super, constructeurs hérités et implicites", `
class A {
    let x = 1
    let log = 0
    constructor() { this.log = this.log * 10 + 1 }
    function valeur() { return this.x }
}
class B extends A {
    let y = 5
    constructor(v) { super() ; this.y = v ; this.log = this.log * 10 + 2 }
    function valeur() { return super.valeur() + this.y }
    function trace() { return this.log }
}
class C extends B {
    function valeur() { return super.valeur() * 100 }
}
class D extends A {
    constructor() { this.log = this.log * 10 + 3 }
    function trace() { return this.log }
}
class Main {
    function main() {
        let c = new C(7)
        print(c.valeur())
        print(c.trace())
        print(new D().trace())
    }
}`, "800 12 13"},

	{"classes : champs, objets dans des objets, identité", `
class Noeud {
    let valeur
    let suivant = 0
    constructor(v, s) { this.valeur = v ; this.suivant = s }
    function somme() {
        let total = this.valeur
        if this.suivant != 0 { total += this.suivant.somme() }
        return total
    }
}
class Compteur {
    let n = 0
    function incr() { this.n += 1 ; return this }
    function lire() { return this.n }
}
class Main {
    let nom = 42
    function main() {
        let liste = new Noeud(1, new Noeud(2, new Noeud(3, 0)))
        print(liste.somme())
        print(new Compteur().incr().incr().incr().lire())
        let a = new Compteur()
        let b = a
        print(a == b)
        print(a == new Compteur())
        print(this.nom)
    }
}`, "6 3 1 0 42"},

	{"classes : beaucoup d'objets (ramasse-miettes)", `
class Point {
    let x
    let y
    constructor(a, b) { this.x = a ; this.y = b }
    function somme() { return this.x + this.y }
}
class Main {
    function main() {
        let garde = new Point(1000, 2000)
        let total = 0
        let i = 0
        while i < 20000 {
            let p = new Point(i, 1)
            total += p.somme() % 7
            i += 1
        }
        print(total)
        print(garde.somme())
    }
}`, "59998 3000"},
}

func TestPrograms(t *testing.T) {
	for _, tt := range programTests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := run(t, tt.src)
			if err != nil {
				t.Fatalf("exécution : %v", err)
			}
			got = strings.Join(strings.Fields(got), " ")
			if got != tt.want {
				t.Errorf("\n got: %s\nwant: %s", got, tt.want)
			}
		})
	}
}

func TestRuntimeErrors(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want string
	}{
		{"division par zéro", "fn main() { let z = 0\n print(1 / z) }", "division par zéro"},
		{"modulo par zéro", "fn main() { let z = 0\n print(1 % z) }", "modulo par zéro"},
		{"récursion infinie", "fn f(n) { return f(n + 1) }\nfn main() { f(0) }", "trop d'appels imbriqués"},
		{"random à l'envers", "fn main() { print(random(6, 1)) }", "plus petit que le minimum"},
		{"printc invalide", "fn main() { printc(-1) }", "n'est pas un code de caractère valide"},
		{"méthode sur un nombre", "class A { function f() {} }\nclass Main { function main() { let x = 5\n x.f() } }",
			"appel de f() sur une valeur qui n'est pas un objet"},
		{"méthode absente de la classe réelle", "class A { function f() {} }\nclass B { function g() {} }\nclass Main { function main() { let o = new B()\n o.f() } }",
			"la classe B n'a pas de méthode f"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := run(t, tt.src)
			var re *refvm.RuntimeError
			if !errors.As(err, &re) || !strings.Contains(re.Msg, tt.want) {
				t.Errorf("erreur %v, attendu %q", err, tt.want)
			}
		})
	}
}

func TestInfiniteLoopHitsStepLimit(t *testing.T) {
	_, err := run(t, "fn main() { while true { } }")
	if !errors.Is(err, refvm.ErrStepLimit) {
		t.Errorf("erreur %v, attendu ErrStepLimit", err)
	}
}

func TestBlinkBytecode(t *testing.T) {
	src := `
let led_pin = 2
class Main {
    function main() {
        gpio.mode(led_pin, 1)
        while true {
            gpio.write(led_pin, 1)
            delay(500)
        }
    }
}`
	prog, err := Compile([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	bytecode.Disassemble(&b, prog, natives.Name)
	want := `; 1 globale(s), 3 fonction(s), 1 classe(s), entrée = <init>, 62 octets de code

class Main  ; 0 champ(s)
  main           -> Main.main

function Main.main  ; params=1 locals=1 max_stack=2
  0000  GLOAD       0
  0003  PUSH8       1
  0005  CALL_NATIVE gpio.mode/2
  0009  POP
  000a  PUSH8       1
  000c  JZ          0026
  000f  GLOAD       0
  0012  PUSH8       1
  0014  CALL_NATIVE gpio.write/2
  0018  POP
  0019  PUSH        500
  001e  CALL_NATIVE delay/1
  0022  POP
  0023  JMP         000a
  0026  PUSH8       0
  0028  RET

function Main.new  ; params=0 locals=1 max_stack=1
  0029  ALLOC       Main
  002c  STORE       0
  002e  LOAD        0
  0030  RET

function <init>  ; params=0 locals=0 max_stack=1
  0031  PUSH8       2
  0033  GSTORE      0
  0036  CALL        Main.new
  0039  CALL        Main.main
  003c  POP
  003d  HALT
`
	if got := b.String(); got != want {
		t.Errorf("désassemblage :\n%s\nattendu :\n%s", got, want)
	}
}

func TestDecodeRejectsCorruption(t *testing.T) {
	prog, err := Compile([]byte(legacy("fn main() { print(1) }")))
	if err != nil {
		t.Fatal(err)
	}
	data := prog.Encode()

	bad := append([]byte(nil), data...)
	bad[bytecode.HeaderSize+bytecode.FuncSize] ^= 0xFF
	if _, err := bytecode.Decode(bad); err == nil || !strings.Contains(err.Error(), "CRC") {
		t.Errorf("octet modifié : erreur %v, attendu CRC", err)
	}
	if _, err := bytecode.Decode(data[:len(data)-1]); err == nil {
		t.Error("fichier tronqué accepté")
	}
	bad = append([]byte(nil), data...)
	bad[0] = 'X'
	if _, err := bytecode.Decode(bad); err == nil || !strings.Contains(err.Error(), "magic") {
		t.Errorf("magic : erreur %v", err)
	}
}

func TestVerifyRejectsBadCode(t *testing.T) {
	base := func(code ...byte) *bytecode.Program {
		return &bytecode.Program{
			NumGlobals: 1,
			Funcs:      []bytecode.Func{{Name: "f", NumLocals: 1, MaxStack: 4}},
			Code:       code,
		}
	}
	B := func(op bytecode.Op) byte { return byte(op) }
	tests := []struct {
		name string
		prog *bytecode.Program
		want string
	}{
		{"opcode inconnu", base(0xEE), "opcode inconnu"},
		{"tronqué", base(B(bytecode.PUSH), 1, 2), "tronquée"},
		{"slot", base(B(bytecode.LOAD), 1, B(bytecode.RET)), "slot 1"},
		{"globale", base(B(bytecode.GLOAD), 1, 0, B(bytecode.RET)), "globale 1"},
		{"fonction", base(B(bytecode.CALL), 5, 0, B(bytecode.RET)), "fonction 5"},
		{"native inconnue", base(B(bytecode.CALL_NATIVE), 99, 0, 0, B(bytecode.RET)), "native 99"},
		{"arité native", base(B(bytecode.CALL_NATIVE), 0, 0, 3, B(bytecode.RET)), "attend 1"},
		{"saut hors fonction", base(B(bytecode.JMP), 10, 0), "saut invalide"},
		{"saut au milieu d'une instruction", base(B(bytecode.PUSH8), 0, B(bytecode.JMP), 0xFC, 0xFF), "saut invalide"},
		{"pas de RET final", base(B(bytecode.PUSH8), 0), "doit se terminer"},
	}
	for _, tt := range tests {
		err := bytecode.Verify(tt.prog, natives.Arity)
		if err == nil || !strings.Contains(err.Error(), tt.want) {
			t.Errorf("%s : erreur %v, attendu %q", tt.name, err, tt.want)
		}
	}
}
