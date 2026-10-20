// Package bytecode définit le format binaire .chlb (cf. docs/bytecode.md) :
// opcodes, encodage, décodage, vérification et désassemblage.
package bytecode

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
)

const (
	Magic      = "CHLB"
	Version    = 1
	HeaderSize = 16
	FuncSize   = 8
)

type Op byte

const (
	HALT  Op = 0x00
	PUSH  Op = 0x01
	PUSH8 Op = 0x02
	POP   Op = 0x03

	LOAD   Op = 0x10
	STORE  Op = 0x11
	GLOAD  Op = 0x12
	GSTORE Op = 0x13

	ADD  Op = 0x20
	SUB  Op = 0x21
	MUL  Op = 0x22
	DIV  Op = 0x23
	MOD  Op = 0x24
	AND  Op = 0x25
	OR   Op = 0x26
	XOR  Op = 0x27
	SHL  Op = 0x28
	SHR  Op = 0x29
	NEG  Op = 0x2A
	NOT  Op = 0x2B
	BNOT Op = 0x2C

	EQ Op = 0x30
	NE Op = 0x31
	LT Op = 0x32
	LE Op = 0x33
	GT Op = 0x34
	GE Op = 0x35

	JMP Op = 0x40
	JZ  Op = 0x41
	JNZ Op = 0x42

	CALL        Op = 0x50
	CALL_NATIVE Op = 0x51
	RET         Op = 0x52
)

// Kind est le type d'un opérande.
type Kind int

const (
	I8 Kind = iota
	I16
	I32
	U8
	U16
)

func (k Kind) Size() int { return [...]int{1, 2, 4, 1, 2}[k] }

type OpInfo struct {
	Name     string
	Operands []Kind
}

var ops = map[Op]OpInfo{
	HALT:  {"HALT", nil},
	PUSH:  {"PUSH", []Kind{I32}},
	PUSH8: {"PUSH8", []Kind{I8}},
	POP:   {"POP", nil},

	LOAD:   {"LOAD", []Kind{U8}},
	STORE:  {"STORE", []Kind{U8}},
	GLOAD:  {"GLOAD", []Kind{U16}},
	GSTORE: {"GSTORE", []Kind{U16}},

	ADD:  {"ADD", nil},
	SUB:  {"SUB", nil},
	MUL:  {"MUL", nil},
	DIV:  {"DIV", nil},
	MOD:  {"MOD", nil},
	AND:  {"AND", nil},
	OR:   {"OR", nil},
	XOR:  {"XOR", nil},
	SHL:  {"SHL", nil},
	SHR:  {"SHR", nil},
	NEG:  {"NEG", nil},
	NOT:  {"NOT", nil},
	BNOT: {"BNOT", nil},

	EQ: {"EQ", nil},
	NE: {"NE", nil},
	LT: {"LT", nil},
	LE: {"LE", nil},
	GT: {"GT", nil},
	GE: {"GE", nil},

	JMP: {"JMP", []Kind{I16}},
	JZ:  {"JZ", []Kind{I16}},
	JNZ: {"JNZ", []Kind{I16}},

	CALL:        {"CALL", []Kind{U16}},
	CALL_NATIVE: {"CALL_NATIVE", []Kind{U16, U8}},
	RET:         {"RET", nil},
}

// Info renvoie la description d'un opcode.
func Info(op Op) (OpInfo, bool) {
	i, ok := ops[op]
	return i, ok
}

func (op Op) String() string {
	if i, ok := ops[op]; ok {
		return i.Name
	}
	return fmt.Sprintf("OP_%02X", byte(op))
}

// Size renvoie la taille totale de l'instruction (opcode + opérandes).
func (op Op) Size() int {
	n := 1
	for _, k := range ops[op].Operands {
		n += k.Size()
	}
	return n
}

// ReadOperand lit un opérande de type k à code[pos:] (bornes à vérifier avant).
func ReadOperand(code []byte, pos int, k Kind) int32 {
	switch k {
	case I8:
		return int32(int8(code[pos]))
	case U8:
		return int32(code[pos])
	case I16:
		return int32(int16(binary.LittleEndian.Uint16(code[pos:])))
	case U16:
		return int32(binary.LittleEndian.Uint16(code[pos:]))
	default:
		return int32(binary.LittleEndian.Uint32(code[pos:]))
	}
}

// ---------------------------------------------------------------------------
// Programme

type Func struct {
	Name      string // non encodé : sert au désassemblage
	Offset    uint32
	NumParams uint8
	NumLocals uint8
	MaxStack  uint16
}

type Program struct {
	NumGlobals uint16
	Funcs      []Func
	Entry      uint16
	Code       []byte
}

// FuncEnd renvoie la fin (exclue) du code de la fonction i.
func (p *Program) FuncEnd(i int) int {
	if i+1 < len(p.Funcs) {
		return int(p.Funcs[i+1].Offset)
	}
	return len(p.Code)
}

// Encode produit le fichier .chlb.
func (p *Program) Encode() []byte {
	var b bytes.Buffer
	le := binary.LittleEndian
	b.WriteString(Magic)
	b.WriteByte(Version)
	b.WriteByte(0)
	b.Write(le.AppendUint16(nil, p.NumGlobals))
	b.Write(le.AppendUint16(nil, uint16(len(p.Funcs))))
	b.Write(le.AppendUint16(nil, p.Entry))
	b.Write(le.AppendUint32(nil, uint32(len(p.Code))))
	for _, f := range p.Funcs {
		b.Write(le.AppendUint32(nil, f.Offset))
		b.WriteByte(f.NumParams)
		b.WriteByte(f.NumLocals)
		b.Write(le.AppendUint16(nil, f.MaxStack))
	}
	b.Write(p.Code)
	b.Write(le.AppendUint32(nil, crc32.ChecksumIEEE(b.Bytes())))
	return b.Bytes()
}

// Decode lit un fichier .chlb, vérifie son intégrité (taille, CRC) puis
// son contenu (Verify). Les noms de fonctions sont f0, f1…
func Decode(data []byte) (*Program, error) {
	le := binary.LittleEndian
	if len(data) < HeaderSize+4 {
		return nil, errors.New("fichier trop court")
	}
	if string(data[:4]) != Magic {
		return nil, errors.New("ce n'est pas un fichier .chlb (magic invalide)")
	}
	if data[4] != Version {
		return nil, fmt.Errorf("version %d non supportée (attendu %d)", data[4], Version)
	}
	numFuncs := int(le.Uint16(data[8:]))
	codeSize := int(le.Uint32(data[12:]))
	total := HeaderSize + numFuncs*FuncSize + codeSize + 4
	if len(data) != total {
		return nil, fmt.Errorf("taille incohérente : %d octets, attendu %d", len(data), total)
	}
	if got, want := crc32.ChecksumIEEE(data[:total-4]), le.Uint32(data[total-4:]); got != want {
		return nil, fmt.Errorf("CRC invalide (%08x, attendu %08x) : fichier corrompu", got, want)
	}

	p := &Program{
		NumGlobals: le.Uint16(data[6:]),
		Entry:      le.Uint16(data[10:]),
	}
	off := HeaderSize
	for i := 0; i < numFuncs; i++ {
		e := data[off : off+FuncSize]
		p.Funcs = append(p.Funcs, Func{
			Name:      fmt.Sprintf("f%d", i),
			Offset:    le.Uint32(e),
			NumParams: e[4],
			NumLocals: e[5],
			MaxStack:  le.Uint16(e[6:]),
		})
		off += FuncSize
	}
	p.Code = append([]byte(nil), data[off:off+codeSize]...)
	if err := Verify(p, nil); err != nil {
		return nil, err
	}
	return p, nil
}

// NativeArity renvoie l'arité d'une native, ou ok = false si l'ID est inconnu.
type NativeArity func(id int) (arity int, ok bool)

// Verify contrôle la cohérence d'un programme (cf. docs/bytecode.md §4).
// Si natives est nil, les ID de natives ne sont pas contrôlés.
func Verify(p *Program, natives NativeArity) error {
	if len(p.Funcs) == 0 {
		return errors.New("aucune fonction")
	}
	if int(p.Entry) >= len(p.Funcs) {
		return fmt.Errorf("fonction d'entrée %d inexistante", p.Entry)
	}
	if p.Funcs[0].Offset != 0 {
		return errors.New("la première fonction doit commencer à 0")
	}
	for i, f := range p.Funcs {
		end := p.FuncEnd(i)
		if int(f.Offset) >= end || end > len(p.Code) {
			return fmt.Errorf("%s : bornes de code invalides", f.Name)
		}
		if f.NumParams > f.NumLocals {
			return fmt.Errorf("%s : plus de paramètres que de locales", f.Name)
		}
		if err := verifyFunc(p, i, natives); err != nil {
			return fmt.Errorf("%s : %w", f.Name, err)
		}
	}
	return nil
}

func verifyFunc(p *Program, fi int, natives NativeArity) error {
	f := p.Funcs[fi]
	start, end := int(f.Offset), p.FuncEnd(fi)

	starts := map[int]bool{} // débuts d'instruction
	type jump struct{ at, target int }
	var jumps []jump
	var last Op

	for pc := start; pc < end; {
		op := Op(p.Code[pc])
		info, ok := ops[op]
		if !ok {
			return fmt.Errorf("opcode inconnu 0x%02X à %d", byte(op), pc)
		}
		if pc+op.Size() > end {
			return fmt.Errorf("instruction %s tronquée à %d", op, pc)
		}
		starts[pc] = true
		var args []int32
		pos := pc + 1
		for _, k := range info.Operands {
			args = append(args, ReadOperand(p.Code, pos, k))
			pos += k.Size()
		}
		switch op {
		case LOAD, STORE:
			if args[0] >= int32(f.NumLocals) {
				return fmt.Errorf("%s : slot %d hors limites à %d", op, args[0], pc)
			}
		case GLOAD, GSTORE:
			if args[0] >= int32(p.NumGlobals) {
				return fmt.Errorf("%s : globale %d hors limites à %d", op, args[0], pc)
			}
		case CALL:
			if int(args[0]) >= len(p.Funcs) {
				return fmt.Errorf("CALL : fonction %d inexistante à %d", args[0], pc)
			}
		case CALL_NATIVE:
			if natives != nil {
				ar, ok := natives(int(args[0]))
				if !ok {
					return fmt.Errorf("CALL_NATIVE : native %d inconnue à %d", args[0], pc)
				}
				if ar != int(args[1]) {
					return fmt.Errorf("CALL_NATIVE : native %d attend %d argument(s), %d fourni(s) à %d", args[0], ar, args[1], pc)
				}
			}
		case JMP, JZ, JNZ:
			jumps = append(jumps, jump{pc, pos + int(args[0])})
		}
		last = op
		pc = pos
	}
	for _, j := range jumps {
		if j.target < start || j.target >= end || !starts[j.target] {
			return fmt.Errorf("saut invalide à %d vers %d", j.at, j.target)
		}
	}
	if last != RET && last != JMP && last != HALT {
		return fmt.Errorf("la fonction doit se terminer par RET, JMP ou HALT (trouvé %s)", last)
	}
	return nil
}
