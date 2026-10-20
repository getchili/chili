package bytecode

import (
	"fmt"
	"io"
	"strings"
)

// Disassemble écrit une version lisible du programme. nativeName, si non
// nil, sert à afficher le nom des natives appelées.
func Disassemble(w io.Writer, p *Program, nativeName func(id int) string) {
	fmt.Fprintf(w, "; %d globale(s), %d fonction(s), %d classe(s), entrée = %s, %d octets de code\n",
		p.NumGlobals, len(p.Funcs), len(p.Classes), p.Funcs[p.Entry].Name, len(p.Code))

	for _, c := range p.Classes {
		head := "class " + p.Name(int(c.NameIdx))
		if c.Abstract {
			head = "abstract " + head
		}
		if c.Parent != NoParent && int(c.Parent) < len(p.Classes) {
			head += " extends " + p.Name(int(p.Classes[c.Parent].NameIdx))
		}
		fmt.Fprintf(w, "\n%s  ; %d champ(s)\n", head, c.NumFields)
		for _, m := range c.Methods {
			fn := "?"
			if int(m.Fn) < len(p.Funcs) {
				fn = p.Funcs[m.Fn].Name
			}
			fmt.Fprintf(w, "  %-14s -> %s\n", p.Name(int(m.Selector)), fn)
		}
	}

	for i, f := range p.Funcs {
		fmt.Fprintf(w, "\nfunction %s  ; params=%d locals=%d max_stack=%d\n",
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
	case ALLOC:
		if int(args[0]) < len(p.Classes) {
			return p.Name(int(p.Classes[args[0]].NameIdx))
		}
	case CALLM:
		return fmt.Sprintf("%s/%d", p.Name(int(args[0])), args[1])
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
