package bytecode

import (
	"fmt"
	"io"
	"strings"
)

// Disassemble écrit une version lisible du programme. nativeName, si non
// nil, sert à afficher le nom des natives appelées.
func Disassemble(w io.Writer, p *Program, nativeName func(id int) string) {
	fmt.Fprintf(w, "; %d globale(s), %d fonction(s), entrée = %s, %d octets de code\n",
		p.NumGlobals, len(p.Funcs), p.Funcs[p.Entry].Name, len(p.Code))
	for i, f := range p.Funcs {
		fmt.Fprintf(w, "\nfn %s  ; params=%d locals=%d max_stack=%d\n",
			f.Name, f.NumParams, f.NumLocals, f.MaxStack)
		for pc := int(f.Offset); pc < p.FuncEnd(i); {
			op := Op(p.Code[pc])
			info, ok := ops[op]
			if !ok || pc+op.Size() > p.FuncEnd(i) {
				fmt.Fprintf(w, "  %04x  ?? 0x%02X\n", pc, byte(op))
				pc++
				continue
			}
			var args []int32
			pos := pc + 1
			for _, k := range info.Operands {
				args = append(args, ReadOperand(p.Code, pos, k))
				pos += k.Size()
			}
			line := fmt.Sprintf("  %04x  %-12s%s", pc, op, operands(p, op, args, pos, nativeName))
			fmt.Fprintln(w, strings.TrimRight(line, " "))
			pc = pos
		}
	}
}

func operands(p *Program, op Op, args []int32, next int, nativeName func(int) string) string {
	switch op {
	case JMP, JZ, JNZ:
		return fmt.Sprintf("%04x", next+int(args[0]))
	case CALL:
		if int(args[0]) < len(p.Funcs) {
			return p.Funcs[args[0]].Name
		}
	case CALL_NATIVE:
		if nativeName != nil {
			return fmt.Sprintf("%s/%d", nativeName(int(args[0])), args[1])
		}
		return fmt.Sprintf("#%d/%d", args[0], args[1])
	}
	s := ""
	for i, a := range args {
		if i > 0 {
			s += " "
		}
		s += fmt.Sprint(a)
	}
	return s
}
