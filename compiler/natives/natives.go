// Package natives est la table des fonctions natives fournies par la VM.
//
// L'ID de chaque native est son index dans Table : il est encodé dans le
// bytecode (CALL_NATIVE) et doit correspondre à la table côté VM C.
// N'ajouter qu'en fin de table pour ne pas casser les programmes compilés.
package natives

import (
	"fmt"
	"strconv"
	"strings"
)

type Native struct {
	Name  string // "delay" ou "gpio.write"
	Arity int
	// Sig donne la sorte attendue de chaque argument (cf. docs/bytecode.md §5) :
	// 'i' entier, 's' chaîne, 'v' n'importe laquelle. len(Sig) == Arity.
	Sig string
}

var Table = []Native{
	{"print", 1, "v"},
	{"delay", 1, "i"},
	{"millis", 0, ""},
	{"gpio.mode", 2, "ii"},
	{"gpio.write", 2, "ii"},
	{"gpio.read", 1, "i"},
	{"random", 2, "ii"},
	{"printc", 1, "i"},
	{"println", 0, ""},
	// Chaînes (v3) : fournies par la VM elle-même, pas par la plateforme.
	{"str.sub", 3, "sii"},
	{"str.find", 2, "ss"},
	{"str.char", 1, "i"},
	{"str.int", 1, "s"},
	// Matériel (firmware 0.4.0).
	{"pwm.write", 2, "ii"},
	{"pwm.tone", 2, "ii"},
	{"servo.write", 2, "ii"},
	{"adc.read", 1, "i"},
	{"adc.mv", 1, "i"},
	{"i2c.begin", 2, "ii"},
	{"i2c.scan", 0, ""},
	{"i2c.write", 2, "iv"},
	{"i2c.read", 2, "ii"},
}

// Limites des natives matériel, communes au firmware et à la simulation PC.
const (
	PWMMax      = 255   // pwm.write : rapport cyclique de 0 à 255
	ToneMax     = 20000 // pwm.tone : fréquence en Hz (0 = silence)
	ServoMax    = 180   // servo.write : angle en degrés
	I2CMaxBytes = 128   // i2c.write / i2c.read : octets par transfert
)

// CheckRange vérifie qu'un argument de native est dans [min, max].
func CheckRange(native, what string, v, min, max int32) error {
	if v < min || v > max {
		return fmt.Errorf("%s : %s %d hors de %d..%d", native, what, v, min, max)
	}
	return nil
}

// CheckTone vérifie une fréquence de pwm.tone : 0 (silence) ou de 20 à ToneMax Hz.
func CheckTone(hz int32) error {
	if hz != 0 && (hz < 20 || hz > ToneMax) {
		return fmt.Errorf("pwm.tone : fréquence %d Hz hors de 20..%d (0 pour couper le son)", hz, ToneMax)
	}
	return nil
}

// ServoPulse renvoie la durée d'impulsion (µs) pour un angle de 0 à 180°.
func ServoPulse(angle int32) int32 {
	return 500 + angle*2000/ServoMax
}

// Builtins sont les fonctions intégrées au langage, compilées en opcodes
// (len → LEN, array → NEWARR). Leurs noms sont réservés comme ceux des natives.
var Builtins = map[string]int{
	"len":   1,
	"array": 1,
}

// Lookup renvoie l'ID et la description d'une native par son nom complet.
func Lookup(name string) (id int, n Native, ok bool) {
	for i, n := range Table {
		if n.Name == name {
			return i, n, true
		}
	}
	return 0, Native{}, false
}

// IsModule indique si au moins une native est préfixée par module + ".".
func IsModule(module string) bool {
	for _, n := range Table {
		if strings.HasPrefix(n.Name, module+".") {
			return true
		}
	}
	return false
}

// Arity renvoie l'arité de la native d'ID id (format attendu par bytecode.Verify).
func Arity(id int) (int, bool) {
	if id < 0 || id >= len(Table) {
		return 0, false
	}
	return Table[id].Arity, true
}

// Name renvoie le nom de la native d'ID id, ou "#id" si elle est inconnue.
func Name(id int) string {
	if id < 0 || id >= len(Table) {
		return "#" + strconv.Itoa(id)
	}
	return Table[id].Name
}

// RandomRange ramène une valeur aléatoire 32 bits r dans [min, max] (bornes
// incluses), sans biais notable. La VM C applique exactement la même formule.
func RandomRange(min, max int32, r uint32) (int32, error) {
	if max < min {
		return 0, fmt.Errorf("random : le maximum (%d) est plus petit que le minimum (%d)", max, min)
	}
	span := uint64(int64(max)-int64(min)) + 1 // de 1 à 2^32
	return int32(int64(min) + int64((uint64(r)*span)>>32)), nil
}

// CheckChar vérifie un code de caractère passé à printc ou str.char.
func CheckChar(name string, code int32) error {
	if code < 1 || code > 0x10FFFF || (code >= 0xD800 && code <= 0xDFFF) {
		return fmt.Errorf("%s : %d n'est pas un code de caractère valide", name, code)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Natives de chaînes (même comportement que chili_vm.c)

// StrSub renvoie les octets [debut, fin[ de s, bornes ramenées dans [0, len(s)].
func StrSub(s string, debut, fin int32) string {
	clamp := func(v int32) int {
		if v < 0 {
			return 0
		}
		if int(v) > len(s) {
			return len(s)
		}
		return int(v)
	}
	a, b := clamp(debut), clamp(fin)
	if a >= b {
		return ""
	}
	return s[a:b]
}

// StrFind renvoie la position de la première occurrence de motif, ou -1.
func StrFind(s, motif string) int32 {
	return int32(strings.Index(s, motif))
}

// StrInt lit un entier décimal : espaces autour permis, signe facultatif,
// repli modulo 2³² au-delà de 32 bits. Renvoie 0 si s n'est pas un nombre.
func StrInt(s string) int32 {
	s = strings.Trim(s, " \t\r\n")
	neg := false
	if s != "" && (s[0] == '-' || s[0] == '+') {
		neg = s[0] == '-'
		s = s[1:]
	}
	if s == "" {
		return 0
	}
	var v uint32
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c < '0' || c > '9' {
			return 0
		}
		v = v*10 + uint32(c-'0')
	}
	if neg {
		v = -v
	}
	return int32(v)
}
