// Package refvm est l'interpréteur de référence du bytecode Chili.
//
// Il définit le comportement attendu de la VM C (cf. docs/bytecode.md) et
// permet d'exécuter un programme sur PC (`chili run`) et de tester le
// générateur de code de bout en bout.
package refvm

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/getchili/chili/compiler/bytecode"
	"github.com/getchili/chili/compiler/natives"
)

// Limites par défaut, proches de ce qu'aura l'ESP32.
const (
	DefaultStackSize = 1024 // valeurs (locales + opérandes, tous cadres confondus)
	DefaultMaxFrames = 64
)

// ---------------------------------------------------------------------------
// Valeurs (cf. docs/bytecode.md §2)

// Kind est la sorte d'une valeur.
type Kind uint8

const (
	Int Kind = iota
	Str
	Arr
	Obj
)

// Article renvoie le nom de la sorte avec son article (« une chaîne »),
// tel qu'il apparaît dans les messages d'erreur.
func (k Kind) Article() string {
	return [...]string{"un entier", "une chaîne", "un tableau", "un objet"}[k]
}

// Value est une valeur Chili. Seuls les champs correspondant à K sont utilisés.
type Value struct {
	K Kind
	I int32   // Int
	S string  // Str
	A *Array  // Arr
	O *Object // Obj
}

type Array struct{ Elems []Value }

type Object struct {
	Class  int
	Fields []Value
}

// IntV et StrV construisent des valeurs (utile aux natives).
func IntV(i int32) Value  { return Value{K: Int, I: i} }
func StrV(s string) Value { return Value{K: Str, S: s} }

// truthy applique la règle de vérité : un entier est vrai s'il est non nul,
// une référence est toujours vraie.
func (v Value) truthy() bool { return v.K != Int || v.I != 0 }

func equal(a, b Value) bool {
	if a.K != b.K {
		return false
	}
	switch a.K {
	case Int:
		return a.I == b.I
	case Str:
		return a.S == b.S
	case Arr:
		return a.A == b.A
	default:
		return a.O == b.O
	}
}

// ---------------------------------------------------------------------------
// VM

// NativeFunc exécute une native de la plateforme (ID id). Les arguments ont
// déjà été vérifiés selon la signature de la native. Les natives str.* sont
// exécutées par la VM elle-même et n'arrivent jamais ici.
type NativeFunc func(id int, args []Value) (Value, error)

// ErrStepLimit est renvoyée quand le nombre maximal d'instructions est atteint.
var ErrStepLimit = errors.New("limite d'instructions atteinte")

// RuntimeError est une erreur d'exécution, avec la fonction et le pc fautifs.
type RuntimeError struct {
	Func string
	PC   int
	Msg  string
}

func (e *RuntimeError) Error() string {
	return fmt.Sprintf("erreur d'exécution dans %s (pc %04x) : %s", e.Func, e.PC, e.Msg)
}

type frame struct {
	fn   int
	pc   int // prochaine instruction
	base int // index du slot 0 dans la pile
}

type VM struct {
	prog    *bytecode.Program
	natives NativeFunc

	Globals   []Value
	stack     []Value
	sp        int
	frames    []frame
	MaxFrames int
	MaxSteps  int // 0 = illimité
}

// New prépare une VM pour prog (déjà vérifié par bytecode.Verify).
func New(prog *bytecode.Program, natives NativeFunc) *VM {
	return &VM{
		prog:      prog,
		natives:   natives,
		Globals:   make([]Value, prog.NumGlobals),
		stack:     make([]Value, DefaultStackSize),
		MaxFrames: DefaultMaxFrames,
	}
}

func (vm *VM) className(c int) string {
	return vm.prog.Name(int(vm.prog.Classes[c].NameIdx))
}

// Text renvoie le texte d'une valeur, tel que l'affiche print
// et tel que l'utilise la concaténation.
func (vm *VM) Text(v Value) string {
	switch v.K {
	case Int:
		return strconv.Itoa(int(v.I))
	case Str:
		return v.S
	case Arr:
		return fmt.Sprintf("<tableau %d>", len(v.A.Elems))
	default:
		return "<" + vm.className(v.O.Class) + ">"
	}
}

func (vm *VM) push(v Value) { vm.stack[vm.sp] = v; vm.sp++ }
func (vm *VM) pop() Value   { vm.sp--; return vm.stack[vm.sp] }
func (vm *VM) top() *Value  { return &vm.stack[vm.sp-1] }

// enter crée le cadre de la fonction fn ; ses paramètres sont déjà au sommet de la pile.
func (vm *VM) enter(fn int) error {
	f := vm.prog.Funcs[fn]
	if len(vm.frames) >= vm.MaxFrames {
		return errors.New("trop d'appels imbriqués (récursion trop profonde ?)")
	}
	base := vm.sp - int(f.NumParams)
	if base+int(f.NumLocals)+int(f.MaxStack) > len(vm.stack) {
		return errors.New("dépassement de pile")
	}
	for i := int(f.NumParams); i < int(f.NumLocals); i++ {
		vm.push(IntV(0))
	}
	vm.frames = append(vm.frames, frame{fn: fn, pc: int(f.Offset), base: base})
	return nil
}

// opSymbols donne l'opérateur du source de chaque opcode, pour les messages.
var opSymbols = map[bytecode.Op]string{
	bytecode.ADD: "+", bytecode.SUB: "-", bytecode.MUL: "*", bytecode.DIV: "/", bytecode.MOD: "%",
	bytecode.AND: "&", bytecode.OR: "|", bytecode.XOR: "^", bytecode.SHL: "<<", bytecode.SHR: ">>",
	bytecode.NEG: "-", bytecode.BNOT: "~",
	bytecode.LT: "<", bytecode.LE: "<=", bytecode.GT: ">", bytecode.GE: ">=",
}

// Run exécute le programme depuis sa fonction d'entrée jusqu'à HALT.
func (vm *VM) Run() error {
	if err := vm.enter(int(vm.prog.Entry)); err != nil {
		return err
	}
	code := vm.prog.Code
	steps := 0

	for {
		fr := &vm.frames[len(vm.frames)-1]
		pc := fr.pc
		op := bytecode.Op(code[pc])
		fail := func(format string, args ...any) error {
			return &RuntimeError{Func: vm.prog.Funcs[fr.fn].Name, PC: pc, Msg: fmt.Sprintf(format, args...)}
		}

		if vm.MaxSteps > 0 {
			if steps >= vm.MaxSteps {
				return ErrStepLimit
			}
			steps++
		}

		fr.pc += op.Size()
		switch op {
		case bytecode.HALT:
			return nil

		case bytecode.PUSH:
			vm.push(IntV(bytecode.ReadOperand(code, pc+1, bytecode.I32)))
		case bytecode.PUSH8:
			vm.push(IntV(bytecode.ReadOperand(code, pc+1, bytecode.I8)))
		case bytecode.PUSHS:
			vm.push(StrV(vm.prog.Strings[bytecode.ReadOperand(code, pc+1, bytecode.U16)]))
		case bytecode.POP:
			vm.sp--
		case bytecode.DUP2:
			a, b := vm.stack[vm.sp-2], vm.stack[vm.sp-1]
			vm.push(a)
			vm.push(b)

		case bytecode.LOAD:
			vm.push(vm.stack[fr.base+int(code[pc+1])])
		case bytecode.STORE:
			vm.stack[fr.base+int(code[pc+1])] = vm.pop()
		case bytecode.GLOAD:
			vm.push(vm.Globals[bytecode.ReadOperand(code, pc+1, bytecode.U16)])
		case bytecode.GSTORE:
			vm.Globals[bytecode.ReadOperand(code, pc+1, bytecode.U16)] = vm.pop()

		case bytecode.NEG, bytecode.BNOT:
			t := vm.top()
			if t.K != Int {
				return fail("'%s' impossible sur %s", opSymbols[op], t.K.Article())
			}
			if op == bytecode.NEG {
				t.I = -t.I
			} else {
				t.I = ^t.I
			}
		case bytecode.NOT:
			*vm.top() = IntV(b2i(!vm.top().truthy()))

		case bytecode.EQ, bytecode.NE:
			b := vm.pop()
			a := vm.pop()
			vm.push(IntV(b2i(equal(a, b) == (op == bytecode.EQ))))

		case bytecode.ADD:
			b, a := vm.stack[vm.sp-1], vm.stack[vm.sp-2]
			var r Value
			switch {
			case a.K == Int && b.K == Int:
				r = IntV(a.I + b.I)
			case (a.K == Str || b.K == Str) && (a.K == Int || a.K == Str) && (b.K == Int || b.K == Str):
				r = StrV(vm.Text(a) + vm.Text(b))
			default:
				return fail("'+' impossible entre %s et %s", a.K.Article(), b.K.Article())
			}
			vm.sp -= 2
			vm.push(r)

		case bytecode.LT, bytecode.LE, bytecode.GT, bytecode.GE:
			b := vm.pop()
			a := vm.pop()
			var c int // comparaison : -1, 0, 1
			switch {
			case a.K == Int && b.K == Int:
				c = cmpInt(a.I, b.I)
			case a.K == Str && b.K == Str:
				c = strings.Compare(a.S, b.S)
			default:
				return fail("'%s' impossible entre %s et %s", opSymbols[op], a.K.Article(), b.K.Article())
			}
			var r bool
			switch op {
			case bytecode.LT:
				r = c < 0
			case bytecode.LE:
				r = c <= 0
			case bytecode.GT:
				r = c > 0
			default:
				r = c >= 0
			}
			vm.push(IntV(b2i(r)))

		case bytecode.SUB, bytecode.MUL, bytecode.DIV, bytecode.MOD,
			bytecode.AND, bytecode.OR, bytecode.XOR, bytecode.SHL, bytecode.SHR:
			b := vm.pop()
			a := vm.pop()
			if a.K != Int || b.K != Int {
				return fail("'%s' impossible entre %s et %s", opSymbols[op], a.K.Article(), b.K.Article())
			}
			r, err := binary(op, a.I, b.I)
			if err != nil {
				return fail("%v", err)
			}
			vm.push(IntV(r))

		case bytecode.JMP:
			fr.pc += int(bytecode.ReadOperand(code, pc+1, bytecode.I16))
		case bytecode.JZ:
			if !vm.pop().truthy() {
				fr.pc += int(bytecode.ReadOperand(code, pc+1, bytecode.I16))
			}
		case bytecode.JNZ:
			if vm.pop().truthy() {
				fr.pc += int(bytecode.ReadOperand(code, pc+1, bytecode.I16))
			}

		case bytecode.CALL:
			if err := vm.enter(int(bytecode.ReadOperand(code, pc+1, bytecode.U16))); err != nil {
				return fail("%v", err)
			}

		case bytecode.CALL_NATIVE:
			id := int(bytecode.ReadOperand(code, pc+1, bytecode.U16))
			argc := int(code[pc+3])
			args := append([]Value(nil), vm.stack[vm.sp-argc:vm.sp]...)
			r, err := vm.callNative(id, args)
			if err != nil {
				return fail("%v", err)
			}
			vm.sp -= argc
			vm.push(r)

		case bytecode.ALLOC:
			c := int(bytecode.ReadOperand(code, pc+1, bytecode.U16))
			fields := make([]Value, vm.prog.Classes[c].NumFields)
			vm.push(Value{K: Obj, O: &Object{Class: c, Fields: fields}})

		case bytecode.GETF, bytecode.SETF:
			idx := int(code[pc+1])
			var val Value
			if op == bytecode.SETF {
				val = vm.pop()
			}
			o := vm.pop()
			if o.K != Obj || idx >= len(o.O.Fields) {
				return fail("accès à un champ sur une valeur qui n'est pas un objet")
			}
			if op == bytecode.SETF {
				o.O.Fields[idx] = val
			} else {
				vm.push(o.O.Fields[idx])
			}

		case bytecode.CALLM:
			sel := uint16(bytecode.ReadOperand(code, pc+1, bytecode.U16))
			argc := int(code[pc+3])
			name := vm.prog.Name(int(sel))
			o := vm.stack[vm.sp-argc-1]
			if o.K != Obj {
				return fail("appel de %s() sur une valeur qui n'est pas un objet", name)
			}
			class := o.O.Class
			methods := vm.prog.Classes[class].Methods
			i := sort.Search(len(methods), func(i int) bool { return methods[i].Selector >= sel })
			if i == len(methods) || methods[i].Selector != sel {
				return fail("la classe %s n'a pas de méthode %s", vm.className(class), name)
			}
			fn := int(methods[i].Fn)
			if int(vm.prog.Funcs[fn].NumParams) != argc+1 {
				return fail("%s.%s attend %d argument(s), %d fourni(s)", vm.className(class), name,
					vm.prog.Funcs[fn].NumParams-1, argc)
			}
			if err := vm.enter(fn); err != nil {
				return fail("%v", err)
			}

		case bytecode.NEWARR:
			n := vm.top()
			if n.K != Int {
				return fail("array() attend un entier (reçu : %s)", n.K.Article())
			}
			if n.I < 0 {
				return fail("array(%d) : taille invalide", n.I)
			}
			elems := make([]Value, n.I) // Value{} est l'entier 0
			*n = Value{K: Arr, A: &Array{Elems: elems}}

		case bytecode.MKARR:
			n := int(bytecode.ReadOperand(code, pc+1, bytecode.U16))
			elems := append([]Value(nil), vm.stack[vm.sp-n:vm.sp]...)
			if elems == nil {
				elems = []Value{}
			}
			vm.sp -= n
			vm.push(Value{K: Arr, A: &Array{Elems: elems}})

		case bytecode.GETI:
			i := vm.pop()
			x := vm.pop()
			idx, err := checkIndex(x, i)
			if err != nil {
				return fail("%v", err)
			}
			if x.K == Arr {
				vm.push(x.A.Elems[idx])
			} else {
				vm.push(IntV(int32(x.S[idx])))
			}

		case bytecode.SETI:
			v := vm.pop()
			i := vm.pop()
			t := vm.pop()
			if t.K == Str {
				return fail("les chaînes ne sont pas modifiables")
			}
			idx, err := checkIndex(t, i)
			if err != nil {
				return fail("%v", err)
			}
			t.A.Elems[idx] = v

		case bytecode.LEN:
			x := vm.top()
			switch x.K {
			case Arr:
				*x = IntV(int32(len(x.A.Elems)))
			case Str:
				*x = IntV(int32(len(x.S)))
			default:
				return fail("len() impossible sur %s", x.K.Article())
			}

		case bytecode.RET:
			r := vm.pop()
			vm.sp = fr.base
			vm.frames = vm.frames[:len(vm.frames)-1]
			if len(vm.frames) == 0 {
				return nil
			}
			vm.push(r)

		default:
			return fail("opcode inconnu 0x%02X", byte(op))
		}
	}
}

// checkIndex vérifie x[i] : x tableau ou chaîne, i entier dans [0, len[.
func checkIndex(x, i Value) (int, error) {
	n := 0
	switch x.K {
	case Arr:
		n = len(x.A.Elems)
	case Str:
		n = len(x.S)
	default:
		return 0, fmt.Errorf("indexation impossible sur %s", x.K.Article())
	}
	if i.K != Int {
		return 0, fmt.Errorf("l'indice doit être un entier (reçu : %s)", i.K.Article())
	}
	if i.I < 0 || int(i.I) >= n {
		return 0, fmt.Errorf("indice %d hors limites (taille %d)", i.I, n)
	}
	return int(i.I), nil
}

// callNative vérifie les arguments selon la signature de la native, puis
// exécute les natives str.* ou passe la main à la plateforme.
func (vm *VM) callNative(id int, args []Value) (Value, error) {
	n := natives.Table[id]
	for i, want := range n.Sig {
		got := args[i].K
		if (want == 'i' && got != Int) || (want == 's' && got != Str) {
			wantName := "un entier"
			if want == 's' {
				wantName = "une chaîne"
			}
			return Value{}, fmt.Errorf("%s : l'argument %d doit être %s (reçu : %s)", n.Name, i+1, wantName, got.Article())
		}
	}
	switch n.Name {
	case "str.sub":
		return StrV(natives.StrSub(args[0].S, args[1].I, args[2].I)), nil
	case "str.find":
		return IntV(natives.StrFind(args[0].S, args[1].S)), nil
	case "str.char":
		if err := natives.CheckChar(n.Name, args[0].I); err != nil {
			return Value{}, err
		}
		return StrV(string(rune(args[0].I))), nil
	case "str.int":
		return IntV(natives.StrInt(args[0].S)), nil
	}
	return vm.natives(id, args)
}

func b2i(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

func cmpInt(a, b int32) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// binary applique un opérateur arithmétique ou bit à bit. En Go,
// MinInt32 / -1 vaut MinInt32 et MinInt32 % -1 vaut 0, comme l'exige la spec.
func binary(op bytecode.Op, a, b int32) (int32, error) {
	switch op {
	case bytecode.SUB:
		return a - b, nil
	case bytecode.MUL:
		return a * b, nil
	case bytecode.DIV:
		if b == 0 {
			return 0, errors.New("division par zéro")
		}
		return a / b, nil
	case bytecode.MOD:
		if b == 0 {
			return 0, errors.New("modulo par zéro")
		}
		return a % b, nil
	case bytecode.AND:
		return a & b, nil
	case bytecode.OR:
		return a | b, nil
	case bytecode.XOR:
		return a ^ b, nil
	case bytecode.SHL:
		return int32(uint32(a) << (uint32(b) & 31)), nil
	default: // SHR
		return a >> (uint32(b) & 31), nil
	}
}
