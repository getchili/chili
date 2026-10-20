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

	"github.com/getchili/chili/compiler/bytecode"
)

// Limites par défaut, proches de ce qu'aura l'ESP32.
const (
	DefaultStackSize = 1024 // valeurs (locales + opérandes, tous cadres confondus)
	DefaultMaxFrames = 64
)

// NativeFunc exécute la native d'ID id avec ses arguments.
type NativeFunc func(id int, args []int32) (int32, error)

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

	Globals   []int32
	stack     []int32
	sp        int
	frames    []frame
	MaxFrames int
	MaxSteps  int // 0 = illimité

	objects []*object // objets créés (pas de ramasse-miettes : la mémoire du PC suffit)
}

// object est un objet Chili. Sa valeur (handle) vaut handleTag | (index + 1).
type object struct {
	class  int
	fields []int32
}

const handleTag = 0x4F000000

func (vm *VM) objectOf(v int32) (*object, bool) {
	if uint32(v)&0xFF000000 != handleTag {
		return nil, false
	}
	i := int(uint32(v)&0xFFFFFF) - 1
	if i < 0 || i >= len(vm.objects) {
		return nil, false
	}
	return vm.objects[i], true
}

func (vm *VM) className(c int) string {
	return vm.prog.Name(int(vm.prog.Classes[c].NameIdx))
}

// New prépare une VM pour prog (déjà vérifié par bytecode.Verify).
func New(prog *bytecode.Program, natives NativeFunc) *VM {
	return &VM{
		prog:      prog,
		natives:   natives,
		Globals:   make([]int32, prog.NumGlobals),
		stack:     make([]int32, DefaultStackSize),
		MaxFrames: DefaultMaxFrames,
	}
}

func (vm *VM) push(v int32) { vm.stack[vm.sp] = v; vm.sp++ }
func (vm *VM) pop() int32   { vm.sp--; return vm.stack[vm.sp] }

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
		vm.push(0)
	}
	vm.frames = append(vm.frames, frame{fn: fn, pc: int(f.Offset), base: base})
	return nil
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
			vm.push(bytecode.ReadOperand(code, pc+1, bytecode.I32))
		case bytecode.PUSH8:
			vm.push(bytecode.ReadOperand(code, pc+1, bytecode.I8))
		case bytecode.POP:
			vm.sp--

		case bytecode.LOAD:
			vm.push(vm.stack[fr.base+int(code[pc+1])])
		case bytecode.STORE:
			vm.stack[fr.base+int(code[pc+1])] = vm.pop()
		case bytecode.GLOAD:
			vm.push(vm.Globals[bytecode.ReadOperand(code, pc+1, bytecode.U16)])
		case bytecode.GSTORE:
			vm.Globals[bytecode.ReadOperand(code, pc+1, bytecode.U16)] = vm.pop()

		case bytecode.NEG:
			vm.stack[vm.sp-1] = -vm.stack[vm.sp-1]
		case bytecode.NOT:
			vm.stack[vm.sp-1] = b2i(vm.stack[vm.sp-1] == 0)
		case bytecode.BNOT:
			vm.stack[vm.sp-1] = ^vm.stack[vm.sp-1]

		case bytecode.ADD, bytecode.SUB, bytecode.MUL, bytecode.DIV, bytecode.MOD,
			bytecode.AND, bytecode.OR, bytecode.XOR, bytecode.SHL, bytecode.SHR,
			bytecode.EQ, bytecode.NE, bytecode.LT, bytecode.LE, bytecode.GT, bytecode.GE:
			b := vm.pop()
			a := vm.pop()
			r, err := binary(op, a, b)
			if err != nil {
				return fail("%v", err)
			}
			vm.push(r)

		case bytecode.JMP:
			fr.pc += int(bytecode.ReadOperand(code, pc+1, bytecode.I16))
		case bytecode.JZ:
			if vm.pop() == 0 {
				fr.pc += int(bytecode.ReadOperand(code, pc+1, bytecode.I16))
			}
		case bytecode.JNZ:
			if vm.pop() != 0 {
				fr.pc += int(bytecode.ReadOperand(code, pc+1, bytecode.I16))
			}

		case bytecode.CALL:
			if err := vm.enter(int(bytecode.ReadOperand(code, pc+1, bytecode.U16))); err != nil {
				return fail("%v", err)
			}

		case bytecode.CALL_NATIVE:
			id := int(bytecode.ReadOperand(code, pc+1, bytecode.U16))
			argc := int(code[pc+3])
			args := append([]int32(nil), vm.stack[vm.sp-argc:vm.sp]...)
			vm.sp -= argc
			r, err := vm.natives(id, args)
			if err != nil {
				return fail("native %d : %v", id, err)
			}
			vm.push(r)

		case bytecode.ALLOC:
			c := int(bytecode.ReadOperand(code, pc+1, bytecode.U16))
			if len(vm.objects) >= 0xFFFFFE {
				return fail("mémoire des objets pleine")
			}
			vm.objects = append(vm.objects, &object{class: c, fields: make([]int32, vm.prog.Classes[c].NumFields)})
			vm.push(int32(handleTag | uint32(len(vm.objects))))

		case bytecode.GETF, bytecode.SETF:
			idx := int(code[pc+1])
			var val int32
			if op == bytecode.SETF {
				val = vm.pop()
			}
			obj, ok := vm.objectOf(vm.pop())
			if !ok || idx >= len(obj.fields) {
				return fail("accès à un champ sur une valeur qui n'est pas un objet")
			}
			if op == bytecode.SETF {
				obj.fields[idx] = val
			} else {
				vm.push(obj.fields[idx])
			}

		case bytecode.CALLM:
			sel := uint16(bytecode.ReadOperand(code, pc+1, bytecode.U16))
			argc := int(code[pc+3])
			name := vm.prog.Name(int(sel))
			obj, ok := vm.objectOf(vm.stack[vm.sp-argc-1])
			if !ok {
				return fail("appel de %s() sur une valeur qui n'est pas un objet", name)
			}
			methods := vm.prog.Classes[obj.class].Methods
			i := sort.Search(len(methods), func(i int) bool { return methods[i].Selector >= sel })
			if i == len(methods) || methods[i].Selector != sel {
				return fail("la classe %s n'a pas de méthode %s", vm.className(obj.class), name)
			}
			fn := int(methods[i].Fn)
			if int(vm.prog.Funcs[fn].NumParams) != argc+1 {
				return fail("%s.%s attend %d argument(s), %d fourni(s)", vm.className(obj.class), name,
					vm.prog.Funcs[fn].NumParams-1, argc)
			}
			if err := vm.enter(fn); err != nil {
				return fail("%v", err)
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

func b2i(b bool) int32 {
	if b {
		return 1
	}
	return 0
}

// binary applique un opérateur binaire. En Go, MinInt32 / -1 vaut MinInt32
// et MinInt32 % -1 vaut 0, comme l'exige la spec.
func binary(op bytecode.Op, a, b int32) (int32, error) {
	switch op {
	case bytecode.ADD:
		return a + b, nil
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
	case bytecode.SHR:
		return a >> (uint32(b) & 31), nil
	case bytecode.EQ:
		return b2i(a == b), nil
	case bytecode.NE:
		return b2i(a != b), nil
	case bytecode.LT:
		return b2i(a < b), nil
	case bytecode.LE:
		return b2i(a <= b), nil
	case bytecode.GT:
		return b2i(a > b), nil
	default: // GE
		return b2i(a >= b), nil
	}
}
