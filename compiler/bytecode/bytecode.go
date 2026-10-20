// Package bytecode définit le format binaire .chlb (cf. docs/bytecode.md) :
// opcodes, encodage, décodage, vérification et désassemblage.
package bytecode

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"hash/crc32"
	"sort"
)

const (
	Magic      = "CHLB"
	Version    = 3
	HeaderSize = 22
	FuncSize   = 10
	ClassSize  = 8 // en-tête d'une classe, suivi de 4 octets par méthode

	NoParent = 0xFFFF
)

type Op byte

const (
	HALT  Op = 0x00
	PUSH  Op = 0x01
	PUSH8 Op = 0x02
	POP   Op = 0x03
	PUSHS Op = 0x04 // u16 chaîne : → chaîne constante
	DUP2  Op = 0x05 // a b → a b a b

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

	ALLOC Op = 0x60 // u16 classe        : → objet
	GETF  Op = 0x61 // u8 champ          : objet → valeur
	SETF  Op = 0x62 // u8 champ          : objet valeur →
	CALLM Op = 0x63 // u16 nom, u8 argc  : objet args → résultat

	NEWARR Op = 0x64 // n → tableau de n zéros
	MKARR  Op = 0x65 // u16 n : v1 … vn → tableau
	GETI   Op = 0x66 // x i → v
	SETI   Op = 0x67 // t i v →
	LEN    Op = 0x68 // x → n
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
	PUSHS: {"PUSHS", []Kind{U16}},
	DUP2:  {"DUP2", nil},

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

	ALLOC: {"ALLOC", []Kind{U16}},
	GETF:  {"GETF", []Kind{U8}},
	SETF:  {"SETF", []Kind{U8}},
	CALLM: {"CALLM", []Kind{U16, U8}},

	NEWARR: {"NEWARR", nil},
	MKARR:  {"MKARR", []Kind{U16}},
	GETI:   {"GETI", nil},
	SETI:   {"SETI", nil},
	LEN:    {"LEN", nil},
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
	Name      string // texte du nom (Names[NameIdx])
	NameIdx   uint16
	Offset    uint32
	NumParams uint8
	NumLocals uint8
	MaxStack  uint16
}

// Method associe un nom de méthode (index dans Names) à une fonction.
type Method struct {
	Selector uint16
	Fn       uint16
}

type Class struct {
	NameIdx   uint16
	Parent    uint16 // NoParent si aucune
	NumFields uint8  // champs hérités compris
	Abstract  bool
	Methods   []Method // table complète (héritées comprises), triée par Selector
}

type Program struct {
	NumGlobals uint16
	Funcs      []Func
	Classes    []Class
	Names      []string // noms des fonctions, classes et méthodes
	Strings    []string // chaînes constantes (PUSHS)
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

// Name renvoie le nom d'index i, ou "#i".
func (p *Program) Name(i int) string {
	if i >= 0 && i < len(p.Names) {
		return p.Names[i]
	}
	return fmt.Sprintf("#%d", i)
}

// Encode produit le fichier .chlb.
func (p *Program) Encode() []byte {
	var b bytes.Buffer
	le := binary.LittleEndian
	u16 := func(v uint16) { b.Write(le.AppendUint16(nil, v)) }
	u32 := func(v uint32) { b.Write(le.AppendUint32(nil, v)) }

	b.WriteString(Magic)
	b.WriteByte(Version)
	b.WriteByte(0)
	u16(p.NumGlobals)
	u16(uint16(len(p.Funcs)))
	u16(p.Entry)
	u32(uint32(len(p.Code)))
	u16(uint16(len(p.Classes)))
	u16(uint16(len(p.Names)))
	u16(uint16(len(p.Strings)))
	for _, f := range p.Funcs {
		u32(f.Offset)
		b.WriteByte(f.NumParams)
		b.WriteByte(f.NumLocals)
		u16(f.MaxStack)
		u16(f.NameIdx)
	}
	for _, c := range p.Classes {
		u16(c.NameIdx)
		u16(c.Parent)
		b.WriteByte(c.NumFields)
		flags := byte(0)
		if c.Abstract {
			flags = 1
		}
		b.WriteByte(flags)
		u16(uint16(len(c.Methods)))
		for _, m := range c.Methods {
			u16(m.Selector)
			u16(m.Fn)
		}
	}
	for _, n := range p.Names {
		b.WriteByte(byte(len(n)))
		b.WriteString(n)
	}
	for _, s := range p.Strings {
		u16(uint16(len(s)))
		b.WriteString(s)
	}
	b.Write(p.Code)
	u32(crc32.ChecksumIEEE(b.Bytes()))
	return b.Bytes()
}

// Decode lit un fichier .chlb, vérifie son intégrité (taille, CRC) puis
// son contenu (Verify, sans contrôle des natives).
func Decode(data []byte) (*Program, error) {
	le := binary.LittleEndian
	if len(data) < HeaderSize+4 {
		return nil, errors.New("fichier trop court")
	}
	if string(data[:4]) != Magic {
		return nil, errors.New("ce n'est pas un fichier .chlb (magic invalide)")
	}
	if data[4] != Version {
		return nil, fmt.Errorf("version de bytecode %d non supportée (attendu %d) : recompile le programme", data[4], Version)
	}
	if got, want := crc32.ChecksumIEEE(data[:len(data)-4]), le.Uint32(data[len(data)-4:]); got != want {
		return nil, fmt.Errorf("CRC invalide (%08x, attendu %08x) : fichier corrompu", got, want)
	}
	body := data[:len(data)-4]

	p := &Program{NumGlobals: le.Uint16(body[6:]), Entry: le.Uint16(body[10:])}
	numFuncs := int(le.Uint16(body[8:]))
	codeSize := int(le.Uint32(body[12:]))
	numClasses := int(le.Uint16(body[16:]))
	numNames := int(le.Uint16(body[18:]))
	numStrings := int(le.Uint16(body[20:]))

	off := HeaderSize
	need := func(n int) error {
		if off+n > len(body) {
			return errors.New("fichier tronqué")
		}
		return nil
	}
	if err := need(numFuncs * FuncSize); err != nil {
		return nil, err
	}
	for i := 0; i < numFuncs; i++ {
		e := body[off:]
		p.Funcs = append(p.Funcs, Func{
			Offset: le.Uint32(e), NumParams: e[4], NumLocals: e[5],
			MaxStack: le.Uint16(e[6:]), NameIdx: le.Uint16(e[8:]),
		})
		off += FuncSize
	}
	for i := 0; i < numClasses; i++ {
		if err := need(ClassSize); err != nil {
			return nil, err
		}
		e := body[off:]
		c := Class{NameIdx: le.Uint16(e), Parent: le.Uint16(e[2:]), NumFields: e[4], Abstract: e[5]&1 != 0}
		n := int(le.Uint16(e[6:]))
		off += ClassSize
		if err := need(4 * n); err != nil {
			return nil, err
		}
		for j := 0; j < n; j++ {
			c.Methods = append(c.Methods, Method{Selector: le.Uint16(body[off:]), Fn: le.Uint16(body[off+2:])})
			off += 4
		}
		p.Classes = append(p.Classes, c)
	}
	for i := 0; i < numNames; i++ {
		if err := need(1); err != nil {
			return nil, err
		}
		n := int(body[off])
		off++
		if err := need(n); err != nil {
			return nil, err
		}
		p.Names = append(p.Names, string(body[off:off+n]))
		off += n
	}
	for i := 0; i < numStrings; i++ {
		if err := need(2); err != nil {
			return nil, err
		}
		n := int(le.Uint16(body[off:]))
		off += 2
		if err := need(n); err != nil {
			return nil, err
		}
		p.Strings = append(p.Strings, string(body[off:off+n]))
		off += n
	}
	if off+codeSize != len(body) {
		return nil, fmt.Errorf("taille incohérente : %d octets, attendu %d", len(data), off+codeSize+4)
	}
	p.Code = append([]byte(nil), body[off:]...)
	for i := range p.Funcs {
		p.Funcs[i].Name = p.Name(int(p.Funcs[i].NameIdx))
	}
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
	for i, n := range p.Names {
		if len(n) > 255 {
			return fmt.Errorf("nom %d trop long", i)
		}
	}
	if len(p.Strings) > 0xFFFF {
		return errors.New("trop de chaînes constantes")
	}
	for i, s := range p.Strings {
		if len(s) > 0xFFFF {
			return fmt.Errorf("chaîne %d trop longue", i)
		}
	}
	for i, c := range p.Classes {
		if int(c.NameIdx) >= len(p.Names) {
			return fmt.Errorf("classe %d : nom invalide", i)
		}
		if c.Parent != NoParent && int(c.Parent) >= len(p.Classes) {
			return fmt.Errorf("classe %s : parent invalide", p.Name(int(c.NameIdx)))
		}
		if !sort.SliceIsSorted(c.Methods, func(a, b int) bool { return c.Methods[a].Selector < c.Methods[b].Selector }) {
			return fmt.Errorf("classe %s : méthodes non triées", p.Name(int(c.NameIdx)))
		}
		for _, m := range c.Methods {
			if int(m.Selector) >= len(p.Names) || int(m.Fn) >= len(p.Funcs) || p.Funcs[m.Fn].NumParams == 0 {
				return fmt.Errorf("classe %s : méthode invalide", p.Name(int(c.NameIdx)))
			}
		}
	}
	for i, f := range p.Funcs {
		end := p.FuncEnd(i)
		if int(f.Offset) >= end || end > len(p.Code) {
			return fmt.Errorf("%s : bornes de code invalides", f.Name)
		}
		if f.NumParams > f.NumLocals {
			return fmt.Errorf("%s : plus de paramètres que de locales", f.Name)
		}
		if int(f.NameIdx) >= len(p.Names) && len(p.Names) > 0 {
			return fmt.Errorf("fonction %d : nom invalide", i)
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
		case ALLOC:
			if int(args[0]) >= len(p.Classes) || p.Classes[args[0]].Abstract {
				return fmt.Errorf("ALLOC : classe %d invalide à %d", args[0], pc)
			}
		case CALLM:
			if int(args[0]) >= len(p.Names) {
				return fmt.Errorf("CALLM : nom %d invalide à %d", args[0], pc)
			}
		case PUSHS:
			if int(args[0]) >= len(p.Strings) {
				return fmt.Errorf("PUSHS : chaîne %d inexistante à %d", args[0], pc)
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
