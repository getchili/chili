package codegen

import (
	"errors"
	"fmt"
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
	prog, err := Compile([]byte(src))
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
	vm := refvm.New(decoded, func(id int, args []int32) (int32, error) {
		if id == printID {
			fmt.Fprintln(&out, args[0])
			return 0, nil
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
fn main() {
    gpio.mode(led_pin, 1)
    while true {
        gpio.write(led_pin, 1)
        delay(500)
    }
}`
	prog, err := Compile([]byte(src))
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	bytecode.Disassemble(&b, prog, natives.Name)
	want := `; 1 globale(s), 2 fonction(s), entrée = <init>, 51 octets de code

fn main  ; params=0 locals=0 max_stack=2
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

fn <init>  ; params=0 locals=0 max_stack=1
  0029  PUSH8       2
  002b  GSTORE      0
  002e  CALL        main
  0031  POP
  0032  HALT
`
	if got := b.String(); got != want {
		t.Errorf("désassemblage :\n%s\nattendu :\n%s", got, want)
	}
}

func TestDecodeRejectsCorruption(t *testing.T) {
	prog, err := Compile([]byte("fn main() { print(1) }"))
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
